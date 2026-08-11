// Package valuation resolves a league's model-valuation segment and computes
// per-side trade totals from player_valuations snapshots. It is used by
// transactioncron to persist sleeper_transactions.trade_values at sync time
// — see docs/superpowers/specs/2026-08-10-trade-valuation-totals-design.md.
package valuation

import (
	"encoding/json"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// FreshnessWindow is how stale a snapshot may be, relative to the timestamp
// it's being valued as-of, before it's treated as absent rather than used.
const FreshnessWindow = 24 * time.Hour

// KnownSegments mirrors SEGMENTS in analysis/src/config.py — the league
// formats the valuation model runs on. Only ppr-sf-10 has a systemd job
// actually producing snapshots today; the other two are supported here so
// adding their replay job later doesn't require a code change.
var KnownSegments = map[string]struct{}{
	"ppr-sf-12": {},
	"ppr-sf-10": {},
	"ppr-sf-8":  {},
}

// Snapshot is one dated model valuation for a player, read from
// player_valuations (written by analysis/main.py, never by Go in
// production — this model exists for reads and test fixtures only).
type Snapshot struct {
	Segment         string    `gorm:"column:segment"`
	SleeperPlayerID string    `gorm:"column:sleeper_player_id"`
	ValuationDate   time.Time `gorm:"column:valuation_date"`
	Value           float64   `gorm:"column:value"`
}

func (Snapshot) TableName() string { return "player_valuations" }

// SegmentKeyForLeague maps a league's settings to its valuation segment key,
// or "" when no segment covers that format.
func SegmentKeyForLeague(ppr *float64, isSuperflex *bool, totalRosters int, leagueType string) string {
	if ppr == nil || *ppr != 1.0 || isSuperflex == nil || !*isSuperflex || leagueType != "redraft" {
		return ""
	}
	key := "ppr-sf-" + strconv.Itoa(totalRosters)
	if _, ok := KnownSegments[key]; !ok {
		return ""
	}
	return key
}

// LoadSnapshotHistory fetches one segment's valuation snapshots dated within
// [from, upTo] for the given players, grouped per player and sorted by date
// ascending. Rows older than `from` are provably unusable by ValueAsOf's
// FreshnessWindow check (they can never resolve for any ts >= from), so
// bounding both ends keeps this from loading a whole segment's history on
// every call.
func LoadSnapshotHistory(db *gorm.DB, segment string, playerIDs []string, from, upTo time.Time) map[string][]Snapshot {
	history := map[string][]Snapshot{}
	if segment == "" || len(playerIDs) == 0 {
		return history
	}
	var snaps []Snapshot
	db.Table("player_valuations").
		Select("sleeper_player_id, valuation_date, value").
		Where("segment = ? AND sleeper_player_id IN ? AND valuation_date >= ? AND valuation_date <= ?", segment, playerIDs, from, upTo).
		Order("sleeper_player_id, valuation_date ASC").
		Scan(&snaps)
	for _, s := range snaps {
		history[s.SleeperPlayerID] = append(history[s.SleeperPlayerID], s)
	}
	return history
}

// ValueAsOf returns the latest snapshot value at or before ts, provided it's
// within maxAge of ts — a snapshot older than that is treated the same as no
// snapshot at all. snaps must be sorted by date ascending.
func ValueAsOf(snaps []Snapshot, ts time.Time, maxAge time.Duration) (float64, bool) {
	for i := len(snaps) - 1; i >= 0; i-- {
		if !snaps[i].ValuationDate.After(ts) {
			if ts.Sub(snaps[i].ValuationDate) > maxAge {
				return 0, false
			}
			return snaps[i].Value, true
		}
	}
	return 0, false
}

// GroupPlayersByRoster inverts a trade's `adds` map (player_id -> roster_id,
// the shape of sleeper_transactions.adds) into roster_id -> player IDs.
// Draft picks are never included — there is no pick-valuation model, and
// picks live in a separate column (draft_picks) this function never sees.
func GroupPlayersByRoster(adds map[string]int) map[int][]string {
	rosters := map[int][]string{}
	for playerID, rosterID := range adds {
		rosters[rosterID] = append(rosters[rosterID], playerID)
	}
	return rosters
}

// ComputeTradeValues sums each roster's players into a total, requiring
// every player on a roster to resolve via ValueAsOf before that roster's
// total is included in values — a roster with any unvalued player, or with
// no players at all (a picks-only side), is simply omitted from values.
//
// complete reports whether every non-empty roster resolved (vacuously true
// when there are none, e.g. a picks-only trade) — independent of whether
// values itself is nil. This distinction is what lets a caller keep retrying
// a trade with some resolved sides and some still-pending ones: values can
// be non-nil (one side already has a total worth showing) while complete is
// still false (another side's player hasn't been valued yet), so the caller
// knows to try again later rather than treating the trade as settled.
func ComputeTradeValues(rosterPlayers map[int][]string, tradeTime time.Time, history map[string][]Snapshot) (values json.RawMessage, complete bool) {
	totals := map[string]float64{}
	nonEmptyRosters := 0
	for rosterID, playerIDs := range rosterPlayers {
		if len(playerIDs) == 0 {
			continue
		}
		nonEmptyRosters++
		var total float64
		resolved := true
		for _, pid := range playerIDs {
			v, ok := ValueAsOf(history[pid], tradeTime, FreshnessWindow)
			if !ok {
				resolved = false
				break
			}
			total += v
		}
		if resolved {
			totals[strconv.Itoa(rosterID)] = total
		}
	}
	complete = len(totals) == nonEmptyRosters
	if len(totals) == 0 {
		return nil, complete
	}
	raw, err := json.Marshal(totals)
	if err != nil {
		return nil, false
	}
	return raw, complete
}
