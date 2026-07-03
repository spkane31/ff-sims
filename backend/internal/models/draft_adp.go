package models

import "time"

// DraftADP is one player's average-draft-position rollup for a single
// (segment, season) — upserted daily by the ADP rollup Temporal worker from
// completed snake/linear redraft Sleeper drafts.
type DraftADP struct {
	Segment         string    `gorm:"primaryKey;column:segment"`
	Season          string    `gorm:"primaryKey;column:season"`
	SleeperPlayerID string    `gorm:"primaryKey;column:sleeper_player_id"`
	AvgPickNo       float64   `gorm:"column:avg_pick_no"`
	PickCount       int       `gorm:"column:pick_count"`
	MinPickNo       int       `gorm:"column:min_pick_no"`
	MaxPickNo       int       `gorm:"column:max_pick_no"`
	UpdatedAt       time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (DraftADP) TableName() string { return "draft_adp" }

// ADPLeagueSizes are the league_size filter/bucket values ADP is computed
// for. "14+" buckets every league with total_rosters >= 14.
var ADPLeagueSizes = []string{"8", "10", "12", "14+"}

// ADPScoringFormats are the scoring_format filter/bucket values ADP is
// computed for, matching sleeper_leagues.ppr: 0, 0.5, 1.
var ADPScoringFormats = []string{"standard", "half_ppr", "ppr"}

// ADPSegment is one (league_size, scoring_format, superflex) combination.
type ADPSegment struct {
	LeagueSize    string
	ScoringFormat string
	Superflex     bool
}

// Key returns the segment's storage/lookup key, e.g. "12-ppr-sf" or "10-half_ppr-1qb".
func (s ADPSegment) Key() string {
	return ADPSegmentKey(s.LeagueSize, s.ScoringFormat, s.Superflex)
}

// ADPSegmentKey builds a segment key from bucketed filter values.
func ADPSegmentKey(leagueSize, scoringFormat string, superflex bool) string {
	sf := "1qb"
	if superflex {
		sf = "sf"
	}
	return leagueSize + "-" + scoringFormat + "-" + sf
}

// AllADPSegments enumerates every ADP segment: the full cross product of
// ADPLeagueSizes x ADPScoringFormats x {superflex, 1qb} (24 segments).
func AllADPSegments() []ADPSegment {
	segments := make([]ADPSegment, 0, len(ADPLeagueSizes)*len(ADPScoringFormats)*2)
	for _, size := range ADPLeagueSizes {
		for _, scoring := range ADPScoringFormats {
			for _, superflex := range []bool{true, false} {
				segments = append(segments, ADPSegment{
					LeagueSize:    size,
					ScoringFormat: scoring,
					Superflex:     superflex,
				})
			}
		}
	}
	return segments
}
