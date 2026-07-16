package discoverycron

import (
	"context"
	"time"

	"gorm.io/gorm"

	"backend/internal/activities"
	"backend/internal/models"
)

// ProcessUser fetches userID's leagues across configured seasons and upserts
// them (activities.FetchUserLeagues already does the league-row + junction-
// row upserts), then stamps the user done. Unlike the Temporal path's
// discoverOneUser, this does not fetch league members/details inline — that
// is ProcessLeague's job now, claimed independently, so a league shared by
// many users is fetched once total instead of once per member.
func ProcessUser(ctx context.Context, da *activities.DiscoveryActivities, userID string) error {
	_, err := da.FetchUserLeagues(ctx, activities.FetchUserLeaguesParams{UserID: userID})
	if err != nil {
		if activities.IsNotFoundAppError(err) {
			return da.DB.WithContext(ctx).
				Model(&models.SleeperUser{}).
				Where("sleeper_user_id = ?", userID).
				Updates(map[string]interface{}{
					"skipped_at": time.Now().UTC(),
					"claimed_at": nil,
				}).Error
		}
		return err
	}

	return da.DB.WithContext(ctx).
		Model(&models.SleeperUser{}).
		Where("sleeper_user_id = ?", userID).
		Updates(map[string]interface{}{
			"last_fetched_at": time.Now().UTC(),
			"claimed_at":      nil,
		}).Error
}

// ProcessLeague fetches leagueID's members and details and writes both in a
// single DB transaction, then clears discovery_claimed_at. Wrapping both
// fetches in one transaction means a details-fetch failure (e.g. Sleeper
// returns an error after members already upserted successfully) leaves no
// partial state — either both land or neither does, and the claim stays in
// place for a later retry either way.
func ProcessLeague(ctx context.Context, da *activities.DiscoveryActivities, leagueID string) error {
	return da.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txDA := &activities.DiscoveryActivities{DB: tx, Sleeper: da.Sleeper}
		if err := txDA.FetchLeagueMembers(ctx, activities.FetchLeagueMembersParams{LeagueID: leagueID}); err != nil {
			return err
		}
		if err := txDA.FetchLeagueDetails(ctx, activities.FetchLeagueDetailsParams{LeagueID: leagueID}); err != nil {
			return err
		}
		return tx.Model(&models.SleeperLeague{}).
			Where("sleeper_league_id = ?", leagueID).
			Update("discovery_claimed_at", nil).Error
	})
}
