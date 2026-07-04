package activities

import (
	"context"
	"math"
	"sort"
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

type pickRow struct {
	SleeperPlayerID string `gorm:"column:sleeper_player_id"`
	PickNo          int    `gorm:"column:pick_no"`
}

// percentileCont returns the p-th percentile (0 <= p <= 1) of sorted using
// linear interpolation between closest ranks — the same algorithm Postgres's
// PERCENTILE_CONT implements. sorted must already be sorted ascending and
// non-empty.
func percentileCont(sorted []int, p float64) float64 {
	n := len(sorted)
	if n == 1 {
		return float64(sorted[0])
	}
	rank := p * float64(n-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return float64(sorted[lo])
	}
	frac := rank - float64(lo)
	return float64(sorted[lo]) + frac*(float64(sorted[hi])-float64(sorted[lo]))
}

// ComputeSegmentSeasonADP computes ADP for every player picked in qualifying
// drafts matching params.Segment and params.Season, then upserts one
// draft_adp row per player. The 20-draft minimum sample size is enforced at
// API read time, not here — every player who appears at least once is
// upserted.
//
// Stats (avg/min/max/count/95% CI) are aggregated in Go rather than SQL: the
// 95% CI needs an ordered-set aggregate (Postgres's PERCENTILE_CONT), which
// has no SQLite equivalent, and this activity's test suite runs against an
// in-memory SQLite DB. Computing in Go with the same linear-interpolation
// formula PERCENTILE_CONT uses keeps results identical while staying
// portable and testable.
func (a *ADPRollupActivities) ComputeSegmentSeasonADP(ctx context.Context, params ComputeSegmentSeasonADPParams) error {
	db := a.DB.WithContext(ctx).
		Table("sleeper_draft_picks p").
		Select("p.sleeper_player_id, p.pick_no").
		Joins("JOIN sleeper_drafts d ON d.sleeper_draft_id = p.sleeper_draft_id").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = d.sleeper_league_id").
		Where("d.status = ? AND d.type IN ? AND l.league_type = ? AND d.season = ?",
			"complete", qualifyingDraftTypes, "redraft", params.Season).
		Where("p.sleeper_player_id != ''")
	db = applySegmentPredicate(db, params.Segment)

	var picks []pickRow
	if err := db.Scan(&picks).Error; err != nil {
		return err
	}
	if len(picks) == 0 {
		return nil
	}

	byPlayer := make(map[string][]int, len(picks))
	for _, p := range picks {
		byPlayer[p.SleeperPlayerID] = append(byPlayer[p.SleeperPlayerID], p.PickNo)
	}

	segmentKey := params.Segment.Key()
	records := make([]models.DraftADP, 0, len(byPlayer))
	for playerID, pickNos := range byPlayer {
		sort.Ints(pickNos)
		sum := 0
		for _, v := range pickNos {
			sum += v
		}
		records = append(records, models.DraftADP{
			Segment:         segmentKey,
			Season:          params.Season,
			SleeperPlayerID: playerID,
			AvgPickNo:       float64(sum) / float64(len(pickNos)),
			PickCount:       len(pickNos),
			MinPickNo:       pickNos[0],
			MaxPickNo:       pickNos[len(pickNos)-1],
			CILowPickNo:     percentileCont(pickNos, 0.025),
			CIHighPickNo:    percentileCont(pickNos, 0.975),
		})
	}

	// One batched upsert instead of one round-trip per player: with a large
	// qualifying draft pool (hundreds of distinct players), a per-row loop
	// could exceed the activity's StartToCloseTimeout partway through,
	// leaving only whichever players were reached upserted for that
	// segment/season, with no rollback. A single batched statement is both
	// atomic and one round trip instead of hundreds.
	return a.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "segment"}, {Name: "season"}, {Name: "sleeper_player_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"avg_pick_no", "pick_count", "min_pick_no", "max_pick_no", "ci_low_pick_no", "ci_high_pick_no", "updated_at",
		}),
	}).CreateInBatches(&records, 500).Error
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
