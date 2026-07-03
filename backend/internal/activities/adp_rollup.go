package activities

import (
	"context"
	"strconv"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/internal/models"
)

// qualifyingDraftTypes are the Sleeper draft types comparable to a snake
// pick order. Auction pick_no reflects nomination order, not draft slot
// value, so auction drafts are excluded from ADP.
var qualifyingDraftTypes = []string{"snake", "linear"}

// ADPRollupActivities holds dependencies for the daily ADP rollup worker.
type ADPRollupActivities struct {
	DB *gorm.DB
}

// ListADPSeasons returns the distinct seasons with at least one qualifying
// (complete, snake/linear, redraft) draft, so the dispatcher doesn't need a
// hardcoded season list.
func (a *ADPRollupActivities) ListADPSeasons(ctx context.Context) ([]string, error) {
	var seasons []string
	err := a.DB.WithContext(ctx).
		Table("sleeper_drafts d").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id").
		Where("d.status = ? AND d.type IN ? AND l.league_type = ?", "complete", qualifyingDraftTypes, "redraft").
		Distinct("d.season").
		Pluck("d.season", &seasons).Error
	return seasons, err
}

type adpRow struct {
	SleeperPlayerID string  `gorm:"column:sleeper_player_id"`
	AvgPickNo       float64 `gorm:"column:avg_pick_no"`
	PickCount       int     `gorm:"column:pick_count"`
	MinPickNo       int     `gorm:"column:min_pick_no"`
	MaxPickNo       int     `gorm:"column:max_pick_no"`
}

// ComputeSegmentSeasonADP computes ADP for every player picked in qualifying
// drafts matching params.Segment and params.Season, then upserts one
// draft_adp row per player. The 20-draft minimum sample size is enforced at
// API read time, not here — every player who appears at least once is
// upserted.
func (a *ADPRollupActivities) ComputeSegmentSeasonADP(ctx context.Context, params ComputeSegmentSeasonADPParams) error {
	db := a.DB.WithContext(ctx).
		Table("sleeper_draft_picks p").
		Select("p.sleeper_player_id, AVG(p.pick_no) AS avg_pick_no, COUNT(*) AS pick_count, MIN(p.pick_no) AS min_pick_no, MAX(p.pick_no) AS max_pick_no").
		Joins("JOIN sleeper_drafts d ON d.sleeper_draft_id = p.sleeper_draft_id").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id").
		Where("d.status = ? AND d.type IN ? AND l.league_type = ? AND d.season = ?",
			"complete", qualifyingDraftTypes, "redraft", params.Season).
		Where("p.sleeper_player_id != ''")
	db = applySegmentPredicate(db, params.Segment)

	var rows []adpRow
	if err := db.Group("p.sleeper_player_id").Scan(&rows).Error; err != nil {
		return err
	}

	segmentKey := params.Segment.Key()
	for _, r := range rows {
		record := models.DraftADP{
			Segment:         segmentKey,
			Season:          params.Season,
			SleeperPlayerID: r.SleeperPlayerID,
			AvgPickNo:       r.AvgPickNo,
			PickCount:       r.PickCount,
			MinPickNo:       r.MinPickNo,
			MaxPickNo:       r.MaxPickNo,
		}
		if err := a.DB.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "segment"}, {Name: "season"}, {Name: "sleeper_player_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"avg_pick_no", "pick_count", "min_pick_no", "max_pick_no", "updated_at",
			}),
		}).Create(&record).Error; err != nil {
			return err
		}
	}
	return nil
}

// applySegmentPredicate appends WHERE conditions for one ADP segment's
// league_size/scoring_format/superflex bucket onto a query already joined to
// sleeper_leagues as "l".
func applySegmentPredicate(db *gorm.DB, seg models.ADPSegment) *gorm.DB {
	if seg.LeagueSize == "14+" {
		db = db.Where("l.total_rosters >= ?", 14)
	} else if n, err := strconv.Atoi(seg.LeagueSize); err == nil {
		db = db.Where("l.total_rosters = ?", n)
	}
	switch seg.ScoringFormat {
	case "standard":
		db = db.Where("l.ppr = ?", 0)
	case "half_ppr":
		db = db.Where("l.ppr = ?", 0.5)
	case "ppr":
		db = db.Where("l.ppr = ?", 1)
	}
	return db.Where("l.is_superflex = ?", seg.Superflex)
}
