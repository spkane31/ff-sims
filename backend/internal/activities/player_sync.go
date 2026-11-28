package activities

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/models"
	"backend/internal/playerstatus"
	"backend/internal/sleeper"
)

// bulkStatusStaleAfter is the longest expected gap between daily bulk status
// captures before the gap is reported as missing coverage.
const bulkStatusStaleAfter = 36 * time.Hour

// PlayerSyncActivities holds dependencies for the daily full player database sync.
type PlayerSyncActivities struct {
	DB      *gorm.DB
	Sleeper *sleeper.Client
}

// FetchAndUpsertAllPlayers fetches the full Sleeper player database (~5MB) and bulk-upserts
// into sleeper_players. Heartbeats every 100 records so Temporal can detect worker crashes.
// It also snapshots each fantasy player's NFL team and injury status from the
// same response, so status collection never adds a second bulk fetch.
func (a *PlayerSyncActivities) FetchAndUpsertAllPlayers(ctx context.Context) (PlayerSyncResult, error) {
	players, err := a.Sleeper.GetAllPlayers(ctx, "nfl")
	if err != nil {
		return PlayerSyncResult{}, err
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
				return PlayerSyncResult{PlayersUpserted: processed}, err
			}
			processed += len(batch)
			activity.RecordHeartbeat(ctx, processed)
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		if err := a.upsertBatch(ctx, batch); err != nil {
			return PlayerSyncResult{PlayersUpserted: processed}, err
		}
		processed += len(batch)
	}

	status := a.snapshotStatus(ctx, players, now)
	return PlayerSyncResult{PlayersUpserted: processed, Status: status}, nil
}

// snapshotStatus writes the availability snapshot for this bulk response. A
// failure is reported in the result instead of failing the activity: the
// player upsert above is already durable, and a Temporal retry would refetch
// Sleeper's bulk feed, which Sleeper asks clients to call at most once a day.
func (a *PlayerSyncActivities) snapshotStatus(ctx context.Context, players map[string]sleeper.Player, observedAt time.Time) playerstatus.Report {
	logger := activityLogger(ctx)
	c := playerstatus.Capture{
		ID:         fmt.Sprintf("%s:%d", playerstatus.SourcePlayersBulk, observedAt.UnixNano()),
		Source:     playerstatus.SourcePlayersBulk,
		ObservedAt: observedAt,
	}
	var warnings []string
	if state, err := a.Sleeper.GetNFLState(ctx); err != nil {
		warnings = append(warnings, "observed season/week unknown: "+err.Error())
	} else {
		c.Season, c.Week = state.Season, state.Week
		if c.Season == "" {
			warnings = append(warnings, "observed season/week unknown: NFL state returned no season")
		}
	}

	report, err := playerstatus.Write(ctx, a.DB, c, playerstatus.FromBulkPlayers(players), nil, bulkStatusStaleAfter)
	report.Warnings = append(warnings, report.Warnings...)
	if err != nil {
		report.WriteError = err.Error()
		logger.Error("player status snapshot write failed",
			"captureID", c.ID, "source", c.Source, "error", err)
		return report
	}

	keyvals := []interface{}{
		"captureID", report.CaptureID,
		"source", report.Source,
		"season", c.Season,
		"week", c.Week,
		"observations", report.Observations,
		"applicabilityUnknown", report.ApplicabilityUnknown,
		"conflictingDuplicates", report.ConflictingDuplicates,
		"stale", report.Stale,
		"warnings", report.Warnings,
	}
	if report.PreviousCaptureAt != nil {
		keyvals = append(keyvals, "previousCaptureAge", observedAt.Sub(*report.PreviousCaptureAt).String())
	}
	if report.Stale || report.Observations == 0 || len(report.Warnings) > 0 {
		logger.Warn("player status snapshot has coverage gaps", keyvals...)
	} else {
		logger.Info("player status snapshot written", keyvals...)
	}
	return report
}

// activityLogger returns Temporal's activity logger, or the default slog
// logger when called outside an activity (direct unit tests).
func activityLogger(ctx context.Context) log.Logger {
	if activity.IsActivity(ctx) {
		return activity.GetLogger(ctx)
	}
	return slog.Default()
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
