package activities

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"go.temporal.io/sdk/temporal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/models"
	"backend/internal/sleeper"
)

// DiscoveryLogTag is stamped on every discovery-related log line (activity
// and workflow) as a "tag" field, so a single grep on the shared worker
// journal — which also carries draft-sync, transaction-sync, and ESPN
// worker log lines — pulls out exactly the discovery pipeline's output:
//
//	journalctl -u ff-sims-worker | grep discovery_trace
const DiscoveryLogTag = "discovery_trace"

// firstScannedSeason is the earliest NFL season scraped per user discovery run.
// Older seasons are excluded — their data is complete and not worth re-scanning.
const firstScannedSeason = 2025

// Seasons returns the NFL seasons to scan during user discovery: firstScannedSeason
// through the current calendar year. Computed at call time (rather than a fixed
// list) so next season's leagues are picked up automatically without a yearly
// code change.
func Seasons() []string {
	currentYear := time.Now().Year()
	seasons := make([]string, 0, currentYear-firstScannedSeason+1)
	for y := firstScannedSeason; y <= currentYear; y++ {
		seasons = append(seasons, strconv.Itoa(y))
	}
	return seasons
}

// DiscoveryActivities holds dependencies for user/league graph expansion activities.
type DiscoveryActivities struct {
	DB      *gorm.DB
	Sleeper *sleeper.Client
}

// claimStaleUsersSQL atomically claims up to batchSize stale users for
// discovery (same pattern as the league sync paths). FOR UPDATE SKIP LOCKED
// lets concurrent claimers partition the queue without double-claiming, and
// the 120-minute expiry re-queues users claimed by a worker that died
// mid-batch. 120 minutes (not 20) because the cron path
// (internal/discoverycron) imposes no per-item timeout shorter than that; a
// shorter TTL risked a still-in-flight user being reclaimed and processed a
// second time concurrently. Because ticks claim rather than re-select, a
// stuck cohort can never head-of-line-block the queue. The tradeoff: a
// worker that dies mid-batch now leaves its claimed users unclaimable for up
// to 120 minutes (not 20) before they become re-queueable, a real if minor
// cost given crashes are rare.
const claimStaleUsersSQL = `
UPDATE sleeper_users SET claimed_at = now()
WHERE sleeper_user_id IN (
    SELECT sleeper_user_id FROM sleeper_users
    WHERE skipped_at IS NULL
      AND (claimed_at IS NULL OR claimed_at < now() - interval '120 minutes')
    ORDER BY last_fetched_at ASC NULLS FIRST
    LIMIT ?
    FOR UPDATE SKIP LOCKED
)
RETURNING sleeper_user_id`

// ClaimStaleUsers claims up to BatchSize users for discovery, never-fetched
// first then oldest. Postgres-only (SKIP LOCKED).
func (a *DiscoveryActivities) ClaimStaleUsers(ctx context.Context, params ClaimStaleUsersParams) ([]string, error) {
	var ids []string
	if err := a.DB.WithContext(ctx).Raw(claimStaleUsersSQL, params.BatchSize).Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// IsNotFoundAppError reports whether err is the NOT_FOUND application error
// produced by the fetch helpers when a Sleeper entity no longer exists.
func IsNotFoundAppError(err error) bool {
	var appErr *temporal.ApplicationError
	return errors.As(err, &appErr) && appErr.Type() == "NOT_FOUND"
}

// FetchUserLeagues fetches all leagues for userID across all configured seasons,
// upserts them into sleeper_leagues and sleeper_league_users, and returns the discovered league IDs.
// Returns a non-retryable NOT_FOUND error if the user no longer exists in Sleeper.
func (a *DiscoveryActivities) FetchUserLeagues(ctx context.Context, params FetchUserLeaguesParams) ([]string, error) {
	var leagueIDs []string
	for _, season := range Seasons() {
		leagues, err := a.Sleeper.GetUserLeagues(ctx, params.UserID, "nfl", season)
		if err != nil {
			var nfe *sleeper.NotFoundError
			if errors.As(err, &nfe) {
				return nil, temporal.NewNonRetryableApplicationError(
					"user not found: "+params.UserID, "NOT_FOUND", err,
				)
			}
			return nil, err
		}
		for _, l := range leagues {
			row := models.SleeperLeague{
				SleeperLeagueID: l.LeagueID,
				Name:            l.Name,
				Season:          l.Season,
				Sport:           l.Sport,
				Status:          l.Status,
				TotalRosters:    l.TotalRosters,
			}
			if err := a.DB.WithContext(ctx).
				Clauses(clause.OnConflict{DoNothing: true}).
				Create(&row).Error; err != nil {
				return nil, err
			}
			junc := models.SleeperLeagueUser{
				SleeperLeagueID: l.LeagueID,
				SleeperUserID:   params.UserID,
			}
			if err := a.DB.WithContext(ctx).
				Clauses(clause.OnConflict{DoNothing: true}).
				Create(&junc).Error; err != nil {
				return nil, err
			}
			leagueIDs = append(leagueIDs, l.LeagueID)
		}
	}
	return leagueIDs, nil
}

// FetchLeagueMembers fetches all members of leagueID and upserts them as new sleeper_users
// with last_fetched_at=NULL so they are picked up by future dispatcher runs.
func (a *DiscoveryActivities) FetchLeagueMembers(ctx context.Context, params FetchLeagueMembersParams) error {
	users, err := a.Sleeper.GetLeagueUsers(ctx, params.LeagueID)
	if err != nil {
		var nfe *sleeper.NotFoundError
		if errors.As(err, &nfe) {
			return temporal.NewNonRetryableApplicationError(
				"league not found: "+params.LeagueID, "NOT_FOUND", err,
			)
		}
		return err
	}
	for _, u := range users {
		row := models.SleeperUser{
			SleeperUserID: u.UserID,
			Username:      u.Username,
			DisplayName:   u.DisplayName,
			Avatar:        u.Avatar,
		}
		if err := a.DB.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// sleeperLeagueType converts the integer type from Sleeper's league settings to a string.
// Sleeper encodes: 0=redraft, 1=keeper, 2=dynasty.
func sleeperLeagueType(t int) string {
	switch t {
	case 1:
		return "keeper"
	case 2:
		return "dynasty"
	default:
		return "redraft"
	}
}

// leagueFullySynced reports whether leagueID is marked complete with details
// already fetched — a completed league's metadata and membership are both
// immutable, so neither needs to be re-fetched on future discovery passes.
func (a *DiscoveryActivities) leagueFullySynced(ctx context.Context, leagueID string) bool {
	var existing models.SleeperLeague
	if err := a.DB.WithContext(ctx).
		Where("sleeper_league_id = ?", leagueID).
		First(&existing).Error; err != nil {
		return false
	}
	return existing.Status == "complete" && existing.LastFetchedAt != nil
}

// FetchLeagueDetails fetches league metadata from Sleeper and stamps last_fetched_at.
// Called during user discovery so league metadata is populated before draft/transaction sync.
// Skips the API call for leagues already marked complete — their metadata is immutable.
func (a *DiscoveryActivities) FetchLeagueDetails(ctx context.Context, params FetchLeagueDetailsParams) error {
	if a.leagueFullySynced(ctx, params.LeagueID) {
		return nil
	}

	league, err := a.Sleeper.GetLeague(ctx, params.LeagueID)
	if err != nil {
		var nfe *sleeper.NotFoundError
		if errors.As(err, &nfe) {
			return temporal.NewNonRetryableApplicationError(
				"league not found: "+params.LeagueID, "NOT_FOUND", err,
			)
		}
		return err
	}

	scoringJSON, _ := json.Marshal(league.ScoringSettings)
	rosterJSON, _ := json.Marshal(league.RosterPositions)

	ppr := league.ScoringSettings["rec"]
	tePremium := league.ScoringSettings["bonus_rec_te"]
	isSuperflex := false
	for _, pos := range league.RosterPositions {
		if pos == "SUPER_FLEX" {
			isSuperflex = true
			break
		}
	}

	leagueType := sleeperLeagueType(league.Settings.Type)

	now := time.Now().UTC()
	return a.DB.WithContext(ctx).
		Model(&models.SleeperLeague{}).
		Where("sleeper_league_id = ?", params.LeagueID).
		Updates(map[string]interface{}{
			"name":             league.Name,
			"status":           league.Status,
			"total_rosters":    league.TotalRosters,
			"ppr":              ppr,
			"te_premium":       tePremium,
			"is_superflex":     isSuperflex,
			"league_type":      leagueType,
			"scoring_settings": scoringJSON,
			"roster_positions": rosterJSON,
			"last_fetched_at":  now,
		}).Error
}
