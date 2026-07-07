package activities

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/temporal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/helpers"
	"backend/internal/models"
	"backend/internal/sleeper"
)

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

// GetDiscoveryConfig returns the discovery dispatcher tuning knobs from env,
// clamped to at least 1 so a bad value can't stall dispatch or break the
// claim query's LIMIT.
func (a *DiscoveryActivities) GetDiscoveryConfig(ctx context.Context) (DiscoveryConfig, error) {
	return DiscoveryConfig{
		ParallelBatches: max(helpers.GetEnv("DISCOVERY_PARALLEL_BATCHES", 2), 1),
		BatchSize:       max(helpers.GetEnv("DISCOVERY_BATCH_SIZE", 50), 1),
		Concurrency:     max(helpers.GetEnv("DISCOVERY_USER_CONCURRENCY", 8), 1),
	}, nil
}

// claimStaleUsersSQL atomically claims up to batchSize stale users for
// discovery (same pattern as the league sync paths). FOR UPDATE SKIP LOCKED
// lets concurrent claimers partition the queue without double-claiming, and
// the 20-minute expiry re-queues users claimed by a worker that died
// mid-batch. Because ticks claim rather than re-select, a stuck cohort can
// never head-of-line-block the queue the way the old workflow-ID-collision
// dedupe did.
const claimStaleUsersSQL = `
UPDATE sleeper_users SET claimed_at = now()
WHERE sleeper_user_id IN (
    SELECT sleeper_user_id FROM sleeper_users
    WHERE skipped_at IS NULL
      AND (claimed_at IS NULL OR claimed_at < now() - interval '20 minutes')
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

// DiscoverUsersBatch runs discovery for a claimed batch of users with bounded
// concurrency, stamping each user done as it completes. Per-user failures are
// counted, not propagated: a failed user keeps its claim and re-enters the
// queue when the claim expires. The activity heartbeats as users complete so
// a dead worker is detected via HeartbeatTimeout.
func (a *DiscoveryActivities) DiscoverUsersBatch(ctx context.Context, params DiscoverUsersBatchParams) (SyncBatchResult, error) {
	logger := activity.GetLogger(ctx)
	res := SyncBatchResult{}

	// Re-scope to users still claimed: on an activity retry, users stamped by
	// the previous attempt have claimed_at cleared and must not re-run.
	var stillClaimed []string
	if err := a.DB.WithContext(ctx).Model(&models.SleeperUser{}).
		Where("sleeper_user_id IN ? AND claimed_at IS NOT NULL", params.UserIDs).
		Pluck("sleeper_user_id", &stillClaimed).Error; err != nil {
		return res, err
	}
	if len(stillClaimed) == 0 {
		return res, nil
	}

	concurrency := max(1, params.Concurrency)
	type userResult struct {
		userID string
		err    error
	}
	sem := make(chan struct{}, concurrency)
	results := make(chan userResult, len(stillClaimed))
	var wg sync.WaitGroup
	for _, id := range stillClaimed {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			results <- userResult{userID: id, err: a.discoverOneUser(ctx, logger, id)}
		})
	}
	go func() { wg.Wait(); close(results) }()

	done := 0
	for r := range results {
		done++
		if r.err != nil {
			res.Failed++
			logger.Warn("user discovery failed", "userID", r.userID, "error", r.err)
		} else {
			res.Processed++
		}
		if done%5 == 0 {
			activity.RecordHeartbeat(ctx, done)
		}
	}
	return res, nil
}

// discoverOneUser fetches a user's leagues across configured seasons, then for
// each league upserts its members (with NULL last_fetched_at so they enter the
// discovery queue) and its details, and finally stamps the user done (clearing
// the claim). Per-league failures are logged and skipped, matching the old
// UserDiscoveryWorkflow's warn-and-continue behavior. A 404 on the user marks
// them permanently skipped.
func (a *DiscoveryActivities) discoverOneUser(ctx context.Context, logger log.Logger, userID string) error {
	leagueIDs, err := a.FetchUserLeagues(ctx, FetchUserLeaguesParams{UserID: userID})
	if err != nil {
		if isNotFoundAppError(err) {
			return a.DB.WithContext(ctx).
				Model(&models.SleeperUser{}).
				Where("sleeper_user_id = ?", userID).
				Updates(map[string]interface{}{
					"skipped_at": time.Now().UTC(),
					"claimed_at": nil,
				}).Error
		}
		return err
	}

	for _, lid := range leagueIDs {
		if err := a.FetchLeagueMembers(ctx, FetchLeagueMembersParams{LeagueID: lid}); err != nil {
			logger.Warn("FetchLeagueMembers failed, continuing", "leagueID", lid, "error", err)
		}
		if err := a.FetchLeagueDetails(ctx, FetchLeagueDetailsParams{LeagueID: lid}); err != nil {
			logger.Warn("FetchLeagueDetails failed, continuing", "leagueID", lid, "error", err)
		}
	}

	return a.DB.WithContext(ctx).
		Model(&models.SleeperUser{}).
		Where("sleeper_user_id = ?", userID).
		Updates(map[string]interface{}{
			"last_fetched_at": time.Now().UTC(),
			"claimed_at":      nil,
		}).Error
}

// isNotFoundAppError reports whether err is the NOT_FOUND application error
// produced by the fetch helpers when a Sleeper entity no longer exists.
func isNotFoundAppError(err error) bool {
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

// FetchLeagueDetails fetches league metadata from Sleeper and stamps last_fetched_at.
// Called during user discovery so league metadata is populated before draft/transaction sync.
// Skips the API call for leagues already marked complete — their metadata is immutable.
func (a *DiscoveryActivities) FetchLeagueDetails(ctx context.Context, params FetchLeagueDetailsParams) error {
	var existing models.SleeperLeague
	if err := a.DB.WithContext(ctx).
		Where("sleeper_league_id = ?", params.LeagueID).
		First(&existing).Error; err == nil {
		if existing.Status == "complete" && existing.LastFetchedAt != nil {
			return nil
		}
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
