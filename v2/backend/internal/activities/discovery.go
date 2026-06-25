package activities

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"go.temporal.io/sdk/temporal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/models"
	"backend/internal/sleeper"
)

// Seasons is the list of NFL seasons scraped per user discovery run.
var Seasons = []string{"2022", "2023", "2024", "2025"}

// DiscoveryActivities holds dependencies for user/league graph expansion activities.
type DiscoveryActivities struct {
	DB      *gorm.DB
	Sleeper *sleeper.Client
}

// GetStaleUsers returns up to batchSize user IDs ordered by last_fetched_at ASC NULLS FIRST,
// excluding users that have been permanently skipped (404).
func (a *DiscoveryActivities) GetStaleUsers(ctx context.Context, params GetStaleUsersParams) ([]string, error) {
	var users []models.SleeperUser
	err := a.DB.WithContext(ctx).
		Where("skipped_at IS NULL").
		Order("CASE WHEN last_fetched_at IS NULL THEN 0 ELSE 1 END, last_fetched_at ASC").
		Limit(params.BatchSize).
		Find(&users).Error
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(users))
	for i, u := range users {
		ids[i] = u.SleeperUserID
	}
	return ids, nil
}

// FetchUserLeagues fetches all leagues for userID across all configured seasons,
// upserts them into sleeper_leagues and sleeper_league_users, and returns the discovered league IDs.
// Returns a non-retryable NOT_FOUND error if the user no longer exists in Sleeper.
func (a *DiscoveryActivities) FetchUserLeagues(ctx context.Context, params FetchUserLeaguesParams) ([]string, error) {
	var leagueIDs []string
	for _, season := range Seasons {
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

// MarkUserFetched sets last_fetched_at=now() on the given user.
func (a *DiscoveryActivities) MarkUserFetched(ctx context.Context, params MarkUserFetchedParams) error {
	now := time.Now().UTC()
	return a.DB.WithContext(ctx).
		Model(&models.SleeperUser{}).
		Where("sleeper_user_id = ?", params.UserID).
		Update("last_fetched_at", now).Error
}

// MarkUserSkipped sets skipped_at=now() so the user is excluded from future batches.
func (a *DiscoveryActivities) MarkUserSkipped(ctx context.Context, params MarkUserSkippedParams) error {
	now := time.Now().UTC()
	return a.DB.WithContext(ctx).
		Model(&models.SleeperUser{}).
		Where("sleeper_user_id = ?", params.UserID).
		Update("skipped_at", now).Error
}

// FetchLeagueDetails fetches league metadata from Sleeper and stamps last_fetched_at.
// Called during user discovery so league metadata is populated before draft/transaction sync.
func (a *DiscoveryActivities) FetchLeagueDetails(ctx context.Context, params FetchLeagueDetailsParams) error {
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
			"scoring_settings": scoringJSON,
			"roster_positions": rosterJSON,
			"last_fetched_at":  now,
		}).Error
}
