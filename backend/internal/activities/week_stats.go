package activities

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/models"
	"backend/internal/sleeper"
)

// WeekStatsActivities holds dependencies for the weekly NFL player stats scraper.
type WeekStatsActivities struct {
	DB      *gorm.DB
	Sleeper *sleeper.Client
}

// fantasyPositions are the positions kept from Sleeper's weekly stats response;
// everything else (IDP, etc.) is filtered out.
var fantasyPositions = []string{"QB", "RB", "WR", "TE", "K", "DEF"}

type weekStatPoints struct {
	PtsPPR     *float64 `json:"pts_ppr"`
	PtsHalfPPR *float64 `json:"pts_half_ppr"`
	PtsStd     *float64 `json:"pts_std"`
}

// FetchWeekStats fetches one week of Sleeper stats, filters to fantasy-relevant
// positions, upserts sleeper_player_week_stats (overwriting on refetch so in-season
// corrections land), and stamps sleeper_week_stat_fetches — including whether the
// week is finalized per Sleeper's current NFL state.
func (a *WeekStatsActivities) FetchWeekStats(ctx context.Context, params FetchWeekStatsParams) error {
	raw, err := a.Sleeper.GetWeekStats(ctx, params.Season, params.Week)
	if err != nil {
		var nfe *sleeper.NotFoundError
		if !errors.As(err, &nfe) {
			return err
		}
		raw = nil // no stats published for this week yet
	}

	if len(raw) > 0 {
		var players []models.SleeperPlayer
		if err := a.DB.WithContext(ctx).
			Where("position IN ?", fantasyPositions).
			Find(&players).Error; err != nil {
			return err
		}
		fantasyIDs := make(map[string]struct{}, len(players))
		for _, p := range players {
			fantasyIDs[p.SleeperPlayerID] = struct{}{}
		}

		for id, statBytes := range raw {
			if _, ok := fantasyIDs[id]; !ok {
				continue
			}
			var pts weekStatPoints
			if err := json.Unmarshal(statBytes, &pts); err != nil {
				return err
			}
			row := models.SleeperPlayerWeekStat{
				Season:          params.Season,
				Week:            params.Week,
				SleeperPlayerID: id,
				PtsPPR:          pts.PtsPPR,
				PtsHalfPPR:      pts.PtsHalfPPR,
				PtsStd:          pts.PtsStd,
				Stats:           json.RawMessage(statBytes),
			}
			if err := a.DB.WithContext(ctx).Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "season"}, {Name: "week"}, {Name: "sleeper_player_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"pts_ppr", "pts_half_ppr", "pts_std", "stats", "updated_at",
				}),
			}).Create(&row).Error; err != nil {
				return err
			}
		}
	}

	state, err := a.Sleeper.GetNFLState(ctx)
	if err != nil {
		return err
	}
	finalized := params.Season < state.Season || (params.Season == state.Season && params.Week < state.Week)

	now := time.Now().UTC()
	fetchRow := models.SleeperWeekStatFetch{
		Season:        params.Season,
		Week:          params.Week,
		LastFetchedAt: &now,
		Finalized:     finalized,
	}
	return a.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "season"}, {Name: "week"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_fetched_at", "finalized"}),
	}).Create(&fetchRow).Error
}

// GetFinalizedWeeks returns the weeks already marked finalized for season, so
// SyncWeekStats can skip re-fetching them.
func (a *WeekStatsActivities) GetFinalizedWeeks(ctx context.Context, params GetFinalizedWeeksParams) ([]int, error) {
	var rows []models.SleeperWeekStatFetch
	if err := a.DB.WithContext(ctx).
		Where("season = ? AND finalized = ?", params.Season, true).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	weeks := make([]int, len(rows))
	for i, r := range rows {
		weeks[i] = r.Week
	}
	return weeks, nil
}

// GetCurrentSeason returns the current NFL season per Sleeper's state endpoint,
// used by the schedule dispatcher to sync the in-progress season automatically.
func (a *WeekStatsActivities) GetCurrentSeason(ctx context.Context) (string, error) {
	state, err := a.Sleeper.GetNFLState(ctx)
	if err != nil {
		return "", err
	}
	return state.Season, nil
}
