package activities

import (
	"context"
	"time"

	"go.temporal.io/sdk/activity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/models"
	"backend/internal/sleeper"
)

// PlayerSyncActivities holds dependencies for the daily full player database sync.
type PlayerSyncActivities struct {
	DB      *gorm.DB
	Sleeper *sleeper.Client
}

// FetchAndUpsertAllPlayers fetches the full Sleeper player database (~5MB) and bulk-upserts
// into sleeper_players. Heartbeats every 100 records so Temporal can detect worker crashes.
func (a *PlayerSyncActivities) FetchAndUpsertAllPlayers(ctx context.Context) error {
	players, err := a.Sleeper.GetAllPlayers(ctx, "nfl")
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	batch := make([]models.SleeperPlayer, 0, 100)
	processed := 0

	for id, p := range players {
		batch = append(batch, models.SleeperPlayer{
			SleeperPlayerID: id,
			EspnID:          string(p.EspnID),
			YahooID:         string(p.YahooID),
			FullName:        p.FullName,
			Position:        p.Position,
			NflTeam:         p.Team,
			Age:             p.Age,
			YearsExp:        p.YearsExp,
			LastFetchedAt:   &now,
		})
		if len(batch) >= 100 {
			if err := a.upsertBatch(ctx, batch); err != nil {
				return err
			}
			processed += len(batch)
			activity.RecordHeartbeat(ctx, processed)
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		return a.upsertBatch(ctx, batch)
	}
	return nil
}

func (a *PlayerSyncActivities) upsertBatch(ctx context.Context, batch []models.SleeperPlayer) error {
	return a.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "sleeper_player_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"espn_id", "yahoo_id", "full_name", "position", "nfl_team",
			"age", "years_exp", "last_fetched_at",
		}),
	}).Create(&batch).Error
}
