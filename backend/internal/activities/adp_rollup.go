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
// Read is the archive DB (full draft/pick history — see the T5 scavenger);
// Write is cloud, where the small derived draft_adp rollup lives.
type ADPRollupActivities struct {
	Read  *gorm.DB
	Write *gorm.DB
}

// ListADPSeasons returns the distinct seasons with at least one qualifying
// (complete, snake/linear, redraft) draft, so the dispatcher doesn't need a
// hardcoded season list.
func (a *ADPRollupActivities) ListADPSeasons(ctx context.Context) ([]string, error) {
	var seasons []string
	err := a.Read.WithContext(ctx).
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
	CILowPickNo     float64 `gorm:"column:ci_low_pick_no"`
	CIHighPickNo    float64 `gorm:"column:ci_high_pick_no"`
}

// baseADPSelect computes avg/count/min/max with ordinary aggregate functions,
// supported identically by every SQL dialect this project runs against.
const baseADPSelect = "p.sleeper_player_id, AVG(p.pick_no) AS avg_pick_no, COUNT(*) AS pick_count, MIN(p.pick_no) AS min_pick_no, MAX(p.pick_no) AS max_pick_no"

// postgresPercentileSelect adds the 95% CI via Postgres's native ordered-set
// aggregate. This computes the percentile inside Postgres from the indexed
// join, in the same single grouped query as the other stats — no per-pick
// rows are ever pulled into the application, which matters at production
// scale (thousands of qualifying drafts per segment/season).
const postgresPercentileSelect = ", PERCENTILE_CONT(0.025) WITHIN GROUP (ORDER BY p.pick_no) AS ci_low_pick_no, PERCENTILE_CONT(0.975) WITHIN GROUP (ORDER BY p.pick_no) AS ci_high_pick_no"

// adpSelectClause returns the Select expression for ComputeSegmentSeasonADP's
// aggregate query. PERCENTILE_CONT/WITHIN GROUP is Postgres-only syntax with
// no SQLite equivalent, and this activity's test suite runs against an
// in-memory SQLite DB (see newTestDB), so the percentile expressions are
// only appended for the "postgres" dialect. Under any other dialect (i.e.
// only ever SQLite, and only ever in tests) ci_low_pick_no/ci_high_pick_no
// are left at their zero value — the same default the 017 migration backfills
// existing rows with — and are never asserted on by the test suite.
func adpSelectClause(dialect string) string {
	if dialect == "postgres" {
		return baseADPSelect + postgresPercentileSelect
	}
	return baseADPSelect
}

// ComputeSegmentSeasonADP computes ADP for every player picked in qualifying
// drafts matching params.Segment and params.Season, then upserts one
// draft_adp row per player. The 20-draft minimum sample size is enforced at
// API read time, not here — every player who appears at least once is
// upserted.
func (a *ADPRollupActivities) ComputeSegmentSeasonADP(ctx context.Context, params ComputeSegmentSeasonADPParams) error {
	db := a.Read.WithContext(ctx).
		Table("sleeper_draft_picks p").
		Select(adpSelectClause(a.Read.Dialector.Name())).
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
	if len(rows) == 0 {
		return nil
	}

	segmentKey := params.Segment.Key()
	records := make([]models.DraftADP, len(rows))
	for i, r := range rows {
		records[i] = models.DraftADP{
			Segment:         segmentKey,
			Season:          params.Season,
			SleeperPlayerID: r.SleeperPlayerID,
			AvgPickNo:       r.AvgPickNo,
			PickCount:       r.PickCount,
			MinPickNo:       r.MinPickNo,
			MaxPickNo:       r.MaxPickNo,
			CILowPickNo:     r.CILowPickNo,
			CIHighPickNo:    r.CIHighPickNo,
		}
	}

	// One batched upsert instead of one round-trip per player: with a large
	// qualifying draft pool (hundreds of distinct players), a per-row loop
	// could exceed the activity's StartToCloseTimeout partway through,
	// leaving only whichever players were reached — in whatever order
	// Postgres happened to return the GROUP BY in — upserted for that
	// segment/season, with no rollback. A single batched statement is both
	// atomic and one round trip instead of hundreds.
	return a.Write.WithContext(ctx).Clauses(clause.OnConflict{
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
