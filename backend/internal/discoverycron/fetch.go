package discoverycron

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

// UserDiscoveryResult is FetchUserLeagues's result for one claimed user.
// FlushUserDiscovery is the batch-write counterpart that persists it.
type UserDiscoveryResult struct {
	UserID      string
	Leagues     []models.SleeperLeague
	Memberships []models.SleeperLeagueUser
	// NotFound is true when Sleeper no longer has this user — a successful,
	// batchable outcome (it still needs skipped_at written), not an error.
	NotFound bool
}

// FetchUserLeagues fetches all of userID's leagues across all configured
// seasons and returns the discovered league + membership rows without
// writing anything.
func FetchUserLeagues(ctx context.Context, sleeperClient *sleeper.Client, userID string) (UserDiscoveryResult, error) {
	res := UserDiscoveryResult{UserID: userID}
	for _, season := range Seasons() {
		leagues, err := sleeperClient.GetUserLeagues(ctx, userID, "nfl", season)
		if err != nil {
			var nfe *sleeper.NotFoundError
			if errors.As(err, &nfe) {
				return UserDiscoveryResult{UserID: userID, NotFound: true}, nil
			}
			return UserDiscoveryResult{}, err
		}
		for _, l := range leagues {
			res.Leagues = append(res.Leagues, models.SleeperLeague{
				SleeperLeagueID: l.LeagueID,
				Name:            l.Name,
				Season:          l.Season,
				Sport:           l.Sport,
				Status:          l.Status,
				TotalRosters:    l.TotalRosters,
			})
			res.Memberships = append(res.Memberships, models.SleeperLeagueUser{
				SleeperLeagueID: l.LeagueID,
				SleeperUserID:   userID,
			})
		}
	}
	return res, nil
}

// FlushUserDiscovery is FetchUserLeagues's batch-write counterpart: one bulk
// insert for all of the batch's league rows (DoNothing conflict), one for
// membership rows, one bulk update stamping last_fetched_at and clearing
// claimed_at for every non-NotFound user, and one bulk update stamping
// skipped_at and clearing claimed_at for every NotFound user.
func FlushUserDiscovery(ctx context.Context, tx *gorm.DB, batch []UserDiscoveryResult) error {
	var leagues []models.SleeperLeague
	var memberships []models.SleeperLeagueUser
	var okIDs, notFoundIDs []string
	for _, r := range batch {
		leagues = append(leagues, r.Leagues...)
		memberships = append(memberships, r.Memberships...)
		if r.NotFound {
			notFoundIDs = append(notFoundIDs, r.UserID)
		} else {
			okIDs = append(okIDs, r.UserID)
		}
	}

	if len(leagues) > 0 {
		if err := tx.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(leagues, 500).Error; err != nil {
			return err
		}
	}
	if len(memberships) > 0 {
		if err := tx.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(memberships, 500).Error; err != nil {
			return err
		}
	}

	now := time.Now().UTC()
	if len(okIDs) > 0 {
		if err := tx.WithContext(ctx).Model(&models.SleeperUser{}).
			Where("sleeper_user_id IN ?", okIDs).
			Updates(map[string]interface{}{"last_fetched_at": now, "claimed_at": nil}).Error; err != nil {
			return err
		}
	}
	if len(notFoundIDs) > 0 {
		if err := tx.WithContext(ctx).Model(&models.SleeperUser{}).
			Where("sleeper_user_id IN ?", notFoundIDs).
			Updates(map[string]interface{}{"skipped_at": now, "claimed_at": nil}).Error; err != nil {
			return err
		}
	}
	return nil
}

// LeagueDetailsUpdate carries FetchLeague's league-metadata fields, ready to
// write. A nil *LeagueDetailsUpdate on LeagueDiscoveryResult means the
// league was already fully synced (see leagueFullySynced) — no detail
// update is needed, only the claim needs clearing.
type LeagueDetailsUpdate struct {
	Name            string
	Status          string
	TotalRosters    int
	PPR             float64
	TEPremium       float64
	IsSuperflex     bool
	LeagueType      string
	ScoringSettings json.RawMessage
	RosterPositions json.RawMessage
}

// LeagueDiscoveryResult is FetchLeague's result for one claimed league.
// FlushLeagueDiscovery is the batch-write counterpart that persists it.
type LeagueDiscoveryResult struct {
	LeagueID string
	Members  []models.SleeperUser
	Details  *LeagueDetailsUpdate
}

// leagueFullySynced reports whether leagueID is marked complete with details
// already fetched — a completed league's metadata and membership are both
// immutable, so neither needs to be re-fetched on future discovery passes.
func leagueFullySynced(ctx context.Context, db *gorm.DB, leagueID string) bool {
	var existing models.SleeperLeague
	if err := db.WithContext(ctx).
		Where("sleeper_league_id = ?", leagueID).
		First(&existing).Error; err != nil {
		return false
	}
	return existing.Status == "complete" && existing.LastFetchedAt != nil
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

// FetchLeague fetches leagueID's members and, unless the league is already
// fully synced, its metadata — without writing anything.
func FetchLeague(ctx context.Context, db *gorm.DB, sleeperClient *sleeper.Client, leagueID string) (LeagueDiscoveryResult, error) {
	res := LeagueDiscoveryResult{LeagueID: leagueID}

	users, err := sleeperClient.GetLeagueUsers(ctx, leagueID)
	if err != nil {
		return LeagueDiscoveryResult{}, err
	}
	for _, u := range users {
		res.Members = append(res.Members, models.SleeperUser{
			SleeperUserID: u.UserID,
			Username:      u.Username,
			DisplayName:   u.DisplayName,
			Avatar:        u.Avatar,
		})
	}

	if leagueFullySynced(ctx, db, leagueID) {
		return res, nil
	}

	league, err := sleeperClient.GetLeague(ctx, leagueID)
	if err != nil {
		return LeagueDiscoveryResult{}, err
	}

	scoringJSON, _ := json.Marshal(league.ScoringSettings)
	rosterJSON, _ := json.Marshal(league.RosterPositions)
	isSuperflex := false
	for _, pos := range league.RosterPositions {
		if pos == "SUPER_FLEX" {
			isSuperflex = true
			break
		}
	}
	res.Details = &LeagueDetailsUpdate{
		Name:            league.Name,
		Status:          league.Status,
		TotalRosters:    league.TotalRosters,
		PPR:             league.ScoringSettings["rec"],
		TEPremium:       league.ScoringSettings["bonus_rec_te"],
		IsSuperflex:     isSuperflex,
		LeagueType:      sleeperLeagueType(league.Settings.Type),
		ScoringSettings: scoringJSON,
		RosterPositions: rosterJSON,
	}
	return res, nil
}

// FlushLeagueDiscovery is FetchLeague's batch-write counterpart: one bulk
// insert for all of the batch's member rows, one bulk update clearing
// discovery_claimed_at for every league in the batch (regardless of whether
// Details is nil — a fully-synced league still needs its claim cleared),
// then a per-league Updates call only for leagues with a non-nil Details
// (those field values genuinely vary per league, unlike the claim-clear).
func FlushLeagueDiscovery(ctx context.Context, tx *gorm.DB, batch []LeagueDiscoveryResult) error {
	var members []models.SleeperUser
	leagueIDs := make([]string, len(batch))
	for i, r := range batch {
		leagueIDs[i] = r.LeagueID
		members = append(members, r.Members...)
	}

	if len(members) > 0 {
		if err := tx.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(members, 500).Error; err != nil {
			return err
		}
	}

	if err := tx.WithContext(ctx).
		Model(&models.SleeperLeague{}).
		Where("sleeper_league_id IN ?", leagueIDs).
		Update("discovery_claimed_at", nil).Error; err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, r := range batch {
		if r.Details == nil {
			continue
		}
		d := r.Details
		if err := tx.WithContext(ctx).
			Model(&models.SleeperLeague{}).
			Where("sleeper_league_id = ?", r.LeagueID).
			Updates(map[string]interface{}{
				"name":             d.Name,
				"status":           d.Status,
				"total_rosters":    d.TotalRosters,
				"ppr":              d.PPR,
				"te_premium":       d.TEPremium,
				"is_superflex":     d.IsSuperflex,
				"league_type":      d.LeagueType,
				"scoring_settings": d.ScoringSettings,
				"roster_positions": d.RosterPositions,
				"last_fetched_at":  now,
			}).Error; err != nil {
			return err
		}
	}
	return nil
}
