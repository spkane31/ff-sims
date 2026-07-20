package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend/internal/database"
	"backend/internal/models"
)

// SleeperStatsSnapshot is one hourly all-time-count snapshot, mirroring
// models.SleeperLifetimeCount. TransactionsTotal stays a pointer (omitted
// from the JSON when nil) so callers can distinguish "no archive DB was
// configured for this snapshot" from a real zero, matching the nullability
// of the underlying column; TradeCount/DraftCount default to 0 for the same
// nil case since that distinction isn't needed for those two callers today.
type SleeperStatsSnapshot struct {
	SnapshotAt time.Time `json:"snapshot_at"`

	UsersTotal    int64 `json:"users_total"`
	UsersExpanded int64 `json:"users_expanded"`
	UsersPending  int64 `json:"users_pending"`
	UsersSkipped  int64 `json:"users_skipped"`

	LeaguesTotal    int64 `json:"leagues_total"`
	LeaguesExpanded int64 `json:"leagues_expanded"`
	LeaguesPending  int64 `json:"leagues_pending"`
	LeaguesSkipped  int64 `json:"leagues_skipped"`

	TransactionsTotal *int64 `json:"transactions_total,omitempty"`
	TradeCount        int64  `json:"trade_count"`
	DraftCount        int64  `json:"draft_count"`
}

// SleeperStatsResponse is the response for GET /api/v1/sleeper/stats.
type SleeperStatsResponse struct {
	Snapshots []SleeperStatsSnapshot `json:"snapshots"`
}

// defaultStatsLimit/maxStatsLimit bound GetSleeperStats' limit query param:
// enough by default for a callable "just the latest" (limit=1) or a modest
// chart without a param, but capped so an unbounded limit can't pull the
// whole table's history in one request.
const (
	defaultStatsLimit = 100
	maxStatsLimit     = 1000
)

// TradeSidePlayer is a single player in one side of a trade. Value is the
// model's valuation as of the trade date (nil when the model has no snapshot
// for the player by then).
type TradeSidePlayer struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Position string   `json:"position"`
	Value    *float64 `json:"value,omitempty"`
}

// TradeSide groups the assets received by one roster in a trade. TotalValue
// sums the valued players on the side (picks are not valued); nil when no
// player on the side has a valuation.
type TradeSide struct {
	RosterID   int               `json:"roster_id"`
	Players    []TradeSidePlayer `json:"players"`
	Picks      []string          `json:"picks"`
	TotalValue *float64          `json:"total_value"`
}

// SleeperTradeItem is a single row in the trades list.
type SleeperTradeItem struct {
	ID         string      `json:"id"`
	LeagueID   string      `json:"league_id"`
	LeagueName string      `json:"league_name"`
	Season     string      `json:"season"`
	Scoring    string      `json:"scoring"`
	Superflex  bool        `json:"superflex"`
	LeagueSize string      `json:"league_size"`
	Status     string      `json:"status"`
	Sides      []TradeSide `json:"sides"`
	CreatedAt  int64       `json:"created_at"`
}

// SleeperTradesResponse is the paginated response for GET /api/v1/sleeper/trades.
type SleeperTradesResponse struct {
	Trades     []SleeperTradeItem `json:"trades"`
	Total      int64              `json:"total"`
	Page       int                `json:"page"`
	Limit      int                `json:"limit"`
	TotalPages int                `json:"total_pages"`
}

// tradePick is the shape of one entry in the draft_picks JSON array on a Sleeper transaction.
type tradePick struct {
	Season  string `json:"season"`
	Round   int    `json:"round"`
	OwnerID int    `json:"owner_id"` // roster receiving the pick
}

// knownValuationSegments mirrors SEGMENTS in analysis/src/config.py — the
// league formats the valuation model runs on. Trades from leagues outside
// these segments get no values.
var knownValuationSegments = map[string]struct{}{
	"ppr-sf-12": {},
	"ppr-sf-10": {},
	"ppr-sf-8":  {},
}

// formatScoring maps a league's PPR setting to a display label, matching the
// buckets used by the admin segment-distribution table.
func formatScoring(ppr *float64) string {
	if ppr == nil {
		return "Other"
	}
	switch *ppr {
	case 1:
		return "PPR"
	case 0.5:
		return "0.5 PPR"
	case 0:
		return "Standard"
	default:
		return "Other"
	}
}

// formatLeagueSize maps a league's roster count to a display label, matching
// the buckets used by the admin segment-distribution table.
func formatLeagueSize(totalRosters int) string {
	switch {
	case totalRosters == 8, totalRosters == 10, totalRosters == 12:
		return strconv.Itoa(totalRosters)
	case totalRosters >= 14:
		return "14+"
	default:
		return "Other"
	}
}

// segmentKeyForLeague maps a league's settings to its valuation segment key,
// or "" when no segment covers that format.
func segmentKeyForLeague(ppr *float64, isSuperflex *bool, totalRosters int, leagueType string) string {
	if ppr == nil || *ppr != 1.0 || isSuperflex == nil || !*isSuperflex || leagueType != "redraft" {
		return ""
	}
	key := fmt.Sprintf("ppr-sf-%d", totalRosters)
	if _, ok := knownValuationSegments[key]; !ok {
		return ""
	}
	return key
}

// valuationSnap is one dated model valuation for a player.
type valuationSnap struct {
	SleeperPlayerID string    `gorm:"column:sleeper_player_id"`
	ValuationDate   time.Time `gorm:"column:valuation_date"`
	Value           float64   `gorm:"column:value"`
}

// loadValuationHistory fetches all of one segment's valuation snapshots up to
// upTo for the given players, grouped per player and sorted by date ascending.
func loadValuationHistory(segment string, playerIDs []string, upTo time.Time) map[string][]valuationSnap {
	history := map[string][]valuationSnap{}
	if segment == "" || len(playerIDs) == 0 {
		return history
	}
	var snaps []valuationSnap
	database.DB.Table("player_valuations").
		Select("sleeper_player_id, valuation_date, value").
		Where("segment = ? AND sleeper_player_id IN ? AND valuation_date <= ?",
			segment, playerIDs, upTo).
		Order("sleeper_player_id, valuation_date ASC").
		Scan(&snaps)
	for _, s := range snaps {
		history[s.SleeperPlayerID] = append(history[s.SleeperPlayerID], s)
	}
	return history
}

// valueAsOf returns the latest snapshot value at or before ts. snaps must be
// sorted by date ascending.
func valueAsOf(snaps []valuationSnap, ts time.Time) (float64, bool) {
	for i := len(snaps) - 1; i >= 0; i-- {
		if !snaps[i].ValuationDate.After(ts) {
			return snaps[i].Value, true
		}
	}
	return 0, false
}

// applySideValues annotates each player with its model value at trade time
// (from values, keyed by player_id) and sets each side's TotalValue. A side's
// TotalValue stays nil when none of its players have a valuation.
func applySideValues(sides []TradeSide, values map[string]float64) {
	for i := range sides {
		var total float64
		var valued bool
		for j := range sides[i].Players {
			v, ok := values[sides[i].Players[j].ID]
			if !ok {
				continue
			}
			val := v
			sides[i].Players[j].Value = &val
			total += v
			valued = true
		}
		if valued {
			t := total
			sides[i].TotalValue = &t
		}
	}
}

// buildTradeSides groups adds (player_id → roster_id) and draft picks into
// per-roster sides. Sides are sorted by roster_id; players within each side
// are sorted by name.
func buildTradeSides(adds map[string]int, players map[string]TradeSidePlayer, rawPicks []byte) []TradeSide {
	sideMap := map[int]*TradeSide{}

	ensureSide := func(rosterID int) *TradeSide {
		if _, ok := sideMap[rosterID]; !ok {
			sideMap[rosterID] = &TradeSide{RosterID: rosterID, Players: []TradeSidePlayer{}, Picks: []string{}}
		}
		return sideMap[rosterID]
	}

	for playerID, rosterID := range adds {
		p, ok := players[playerID]
		if !ok {
			p = TradeSidePlayer{ID: playerID, Name: playerID}
		}
		s := ensureSide(rosterID)
		s.Players = append(s.Players, p)
	}

	if len(rawPicks) > 0 {
		var picks []tradePick
		if err := json.Unmarshal(rawPicks, &picks); err == nil {
			for _, pk := range picks {
				if pk.OwnerID == 0 {
					continue
				}
				label := fmt.Sprintf("%s Round %d pick", pk.Season, pk.Round)
				s := ensureSide(pk.OwnerID)
				s.Picks = append(s.Picks, label)
			}
		}
	}

	rosterIDs := make([]int, 0, len(sideMap))
	for id := range sideMap {
		rosterIDs = append(rosterIDs, id)
	}
	sort.Ints(rosterIDs)
	sides := make([]TradeSide, len(rosterIDs))
	for i, rid := range rosterIDs {
		s := sideMap[rid]
		sort.Slice(s.Players, func(a, b int) bool { return s.Players[a].Name < s.Players[b].Name })
		sort.Strings(s.Picks)
		sides[i] = *s
	}
	return sides
}

// GetSleeperStats returns a series of hourly all-time-count snapshots —
// users/leagues discovery-state breakdowns plus trades/drafts/transactions
// totals — most recent first, read from sleeper_lifetime_counts (see
// internal/statscron, the cmd/cron job that snapshots it) rather than
// COUNT(*) against sleeper_transactions/sleeper_drafts directly: those cloud
// tables are trimmed to a hot window by the scavenger's purge phase (and,
// for drafts, mostly bypassed entirely at ingest once the archive DB is
// configured — see syncOneLeagueDrafts in internal/activities/data_fetch.go),
// so a live COUNT there would undercount. Supports limit (default 100, max
// 1000) and skip (default 0) query params, so the home page can ask for just
// the latest point (limit=1) and /admin's growth-over-time charts can page
// through history. A snapshot taken before an archive DB was configured
// leaves TradeCount/DraftCount at zero and omits transactions_total entirely
// for that point, rather than erroring on the nil columns.
func GetSleeperStats(c *gin.Context) {
	limit := defaultStatsLimit
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		limit = min(v, maxStatsLimit)
	}
	skip := 0
	if v, err := strconv.Atoi(c.Query("skip")); err == nil && v >= 0 {
		skip = v
	}

	var rows []models.SleeperLifetimeCount
	if err := database.DB.Order("snapshot_at DESC").Limit(limit).Offset(skip).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, map[string]string{"msg": err.Error()})
		return
	}

	snapshots := make([]SleeperStatsSnapshot, len(rows))
	for i, r := range rows {
		snapshots[i] = SleeperStatsSnapshot{
			SnapshotAt: r.SnapshotAt,

			UsersTotal:    r.UsersTotal,
			UsersExpanded: r.UsersExpanded,
			UsersPending:  r.UsersPending,
			UsersSkipped:  r.UsersSkipped,

			LeaguesTotal:    r.LeaguesTotal,
			LeaguesExpanded: r.LeaguesExpanded,
			LeaguesPending:  r.LeaguesPending,
			LeaguesSkipped:  r.LeaguesSkipped,

			TransactionsTotal: r.TransactionsTotal,
		}
		if r.TradesCompleted != nil {
			snapshots[i].TradeCount = *r.TradesCompleted
		}
		if r.DraftsCompleted != nil {
			snapshots[i].DraftCount = *r.DraftsCompleted
		}
	}

	c.JSON(http.StatusOK, SleeperStatsResponse{Snapshots: snapshots})
}

// applyLeagueFilters appends league-level filter conditions to a GORM query.
// leagueAlias is the SQL alias used for sleeper_leagues (e.g. "l").
func applyLeagueFilters(db *gorm.DB, c *gin.Context, leagueAlias string) *gorm.DB {
	if v := c.Query("league_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			db = db.Where(leagueAlias+".total_rosters = ?", n)
		}
	}
	if v := c.Query("scoring_format"); v != "" {
		switch v {
		case "standard":
			db = db.Where(leagueAlias+".ppr = ?", 0)
		case "half_ppr":
			db = db.Where(leagueAlias+".ppr = ?", 0.5)
		case "ppr":
			db = db.Where(leagueAlias+".ppr = ?", 1)
		}
	}
	if v := c.Query("draft_type"); v != "" {
		db = db.Where(leagueAlias+".draft_type = ?", v)
	}
	if v := c.Query("league_type"); v != "" {
		db = db.Where(leagueAlias+".league_type = ?", v)
	}
	if v := c.Query("superflex"); v != "" {
		db = db.Where(leagueAlias+".is_superflex = ?", v == "true")
	}
	return db
}

// hasLeagueFilters reports whether the request includes any league-level filters.
// Used to decide whether the COUNT query needs a JOIN to sleeper_leagues.
func hasLeagueFilters(c *gin.Context) bool {
	return c.Query("league_size") != "" ||
		c.Query("scoring_format") != "" ||
		c.Query("draft_type") != "" ||
		c.Query("league_type") != "" ||
		c.Query("superflex") != ""
}

// GetSleeperTrades returns a paginated list of Sleeper trades ordered by recency,
// with each trade's adds grouped by roster into named sides.
// Supports query filters: league_size (int), scoring_format (standard|half_ppr|ppr), draft_type (snake|auction|linear), league_type (redraft|keeper|dynasty), superflex (bool), exclude_picks (bool).
func GetSleeperTrades(c *gin.Context) {
	page, limit := parsePagination(c)
	offset := (page - 1) * limit
	excludePicks := c.Query("exclude_picks") == "true" || c.Query("exclude_picks") == "1"

	type tradeRow struct {
		SleeperTransactionID string          `gorm:"column:sleeper_transaction_id"`
		SleeperLeagueID      string          `gorm:"column:sleeper_league_id"`
		LeagueName           string          `gorm:"column:league_name"`
		Season               string          `gorm:"column:season"`
		Status               string          `gorm:"column:status"`
		Adds                 json.RawMessage `gorm:"column:adds"`
		DraftPicks           json.RawMessage `gorm:"column:draft_picks"`
		CreatedAtSleeper     int64           `gorm:"column:created_at_sleeper"`
		PPR                  *float64        `gorm:"column:ppr"`
		IsSuperflex          *bool           `gorm:"column:is_superflex"`
		TotalRosters         int             `gorm:"column:total_rosters"`
		LeagueType           string          `gorm:"column:league_type"`
	}

	var rows []tradeRow
	var total int64

	db := database.DB.Table("sleeper_transactions t").
		Select("t.sleeper_transaction_id, t.sleeper_league_id, l.name as league_name, l.season, t.status, t.adds, t.draft_picks, t.created_at_sleeper, l.ppr, l.is_superflex, l.total_rosters, l.league_type").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = t.sleeper_league_id").
		Where("t.type = ? AND t.status = ?", "trade", "complete")
	db = applyLeagueFilters(db, c, "l")
	if excludePicks {
		db = db.Where("t.draft_picks IS NULL OR jsonb_array_length(t.draft_picks) = 0")
	}

	// When no league-level filters are active, count directly on sleeper_transactions
	// to avoid a full join across 10M+ rows. The partial index
	// idx_sleeper_transactions_trade_complete makes this an index-only scan.
	if hasLeagueFilters(c) {
		db.Count(&total)
	} else {
		countDB := database.DB.Model(&models.SleeperTransaction{}).
			Where("type = ? AND status = ?", "trade", "complete")
		if excludePicks {
			countDB = countDB.Where("draft_picks IS NULL OR jsonb_array_length(draft_picks) = 0")
		}
		countDB.Count(&total)
	}
	db.Order("t.created_at_sleeper DESC").Limit(limit).Offset(offset).Scan(&rows)

	// Decode adds and collect all unique player IDs on this page.
	addsPerRow := make([]map[string]int, len(rows))
	playerIDSet := map[string]struct{}{}
	for i, r := range rows {
		var adds map[string]int
		if len(r.Adds) > 0 {
			_ = json.Unmarshal(r.Adds, &adds)
		}
		addsPerRow[i] = adds
		for pid := range adds {
			playerIDSet[pid] = struct{}{}
		}
	}

	// Batch-fetch player names for all players on this page.
	playerLookup := map[string]TradeSidePlayer{}
	if len(playerIDSet) > 0 {
		ids := make([]string, 0, len(playerIDSet))
		for id := range playerIDSet {
			ids = append(ids, id)
		}
		var players []models.SleeperPlayer
		database.DB.Where("sleeper_player_id IN ?", ids).Find(&players)
		for _, p := range players {
			playerLookup[p.SleeperPlayerID] = TradeSidePlayer{
				ID:       p.SleeperPlayerID,
				Name:     p.FullName,
				Position: p.Position,
			}
		}
	}

	// Batch-load valuation history for this page's players — one query per
	// valuation segment present on the page — then resolve each player's value
	// as of its trade's date. Trades from leagues outside the model's segments
	// get no values.
	var maxCreated int64
	segmentPerRow := make([]string, len(rows))
	playersBySegment := map[string]map[string]struct{}{}
	for i, r := range rows {
		if r.CreatedAtSleeper > maxCreated {
			maxCreated = r.CreatedAtSleeper
		}
		seg := segmentKeyForLeague(r.PPR, r.IsSuperflex, r.TotalRosters, r.LeagueType)
		segmentPerRow[i] = seg
		if seg == "" {
			continue
		}
		if playersBySegment[seg] == nil {
			playersBySegment[seg] = map[string]struct{}{}
		}
		for pid := range addsPerRow[i] {
			playersBySegment[seg][pid] = struct{}{}
		}
	}
	historyBySegment := map[string]map[string][]valuationSnap{}
	for seg, idSet := range playersBySegment {
		ids := make([]string, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		historyBySegment[seg] = loadValuationHistory(seg, ids, time.UnixMilli(maxCreated).UTC())
	}

	items := make([]SleeperTradeItem, len(rows))
	for i, r := range rows {
		sides := buildTradeSides(addsPerRow[i], playerLookup, r.DraftPicks)
		if seg := segmentPerRow[i]; seg != "" {
			tradeTime := time.UnixMilli(r.CreatedAtSleeper).UTC()
			values := map[string]float64{}
			for pid := range addsPerRow[i] {
				if v, ok := valueAsOf(historyBySegment[seg][pid], tradeTime); ok {
					values[pid] = v
				}
			}
			applySideValues(sides, values)
		}
		items[i] = SleeperTradeItem{
			ID:         r.SleeperTransactionID,
			LeagueID:   r.SleeperLeagueID,
			LeagueName: r.LeagueName,
			Season:     r.Season,
			Scoring:    formatScoring(r.PPR),
			Superflex:  r.IsSuperflex != nil && *r.IsSuperflex,
			LeagueSize: formatLeagueSize(r.TotalRosters),
			Status:     r.Status,
			Sides:      sides,
			CreatedAt:  r.CreatedAtSleeper,
		}
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	c.JSON(http.StatusOK, SleeperTradesResponse{
		Trades:     items,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	})
}

// SleeperTransactionItem is a single row in the transactions list.
type SleeperTransactionItem struct {
	ID          string `json:"id"`
	LeagueID    string `json:"league_id"`
	LeagueName  string `json:"league_name"`
	Season      string `json:"season"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	CreatedAt   int64  `json:"created_at"`
	PlayerCount int    `json:"player_count"`
}

// SleeperTransactionsResponse is the paginated response for GET /api/v1/sleeper/transactions.
type SleeperTransactionsResponse struct {
	Transactions []SleeperTransactionItem `json:"transactions"`
	Total        int64                    `json:"total"`
	Page         int                      `json:"page"`
	Limit        int                      `json:"limit"`
	TotalPages   int                      `json:"total_pages"`
}

// GetSleeperTransactions returns a paginated list of all Sleeper transactions.
// Supports query filters: type (trade|waiver|free_agent), league_size, scoring_format, draft_type, league_type (redraft|keeper|dynasty).
func GetSleeperTransactions(c *gin.Context) {
	page, limit := parsePagination(c)
	offset := (page - 1) * limit

	type txRow struct {
		SleeperTransactionID string          `gorm:"column:sleeper_transaction_id"`
		SleeperLeagueID      string          `gorm:"column:sleeper_league_id"`
		LeagueName           string          `gorm:"column:league_name"`
		Season               string          `gorm:"column:season"`
		Type                 string          `gorm:"column:type"`
		Status               string          `gorm:"column:status"`
		CreatedAtSleeper     int64           `gorm:"column:created_at_sleeper"`
		Adds                 json.RawMessage `gorm:"column:adds"`
	}

	var rows []txRow
	var total int64

	txType := c.Query("type")

	db := database.DB.Table("sleeper_transactions t").
		Select("t.sleeper_transaction_id, t.sleeper_league_id, l.name as league_name, l.season, t.type, t.status, t.created_at_sleeper, t.adds").
		Joins("JOIN sleeper_leagues l ON l.sleeper_league_id = t.sleeper_league_id").
		Where("t.status = ?", "complete")

	if txType != "" {
		db = db.Where("t.type = ?", txType)
	}
	db = applyLeagueFilters(db, c, "l")

	if hasLeagueFilters(c) {
		db.Count(&total)
	} else {
		countDB := database.DB.Model(&models.SleeperTransaction{}).Where("status = ?", "complete")
		if txType != "" {
			countDB = countDB.Where("type = ?", txType)
		}
		countDB.Count(&total)
	}
	db.Order("t.created_at_sleeper DESC").Limit(limit).Offset(offset).Scan(&rows)

	items := make([]SleeperTransactionItem, len(rows))
	for i, r := range rows {
		var adds map[string]int
		if len(r.Adds) > 0 {
			_ = json.Unmarshal(r.Adds, &adds)
		}
		items[i] = SleeperTransactionItem{
			ID:          r.SleeperTransactionID,
			LeagueID:    r.SleeperLeagueID,
			LeagueName:  r.LeagueName,
			Season:      r.Season,
			Type:        r.Type,
			Status:      r.Status,
			CreatedAt:   r.CreatedAtSleeper,
			PlayerCount: len(adds),
		}
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	c.JSON(http.StatusOK, SleeperTransactionsResponse{
		Transactions: items,
		Total:        total,
		Page:         page,
		Limit:        limit,
		TotalPages:   totalPages,
	})
}

func parsePagination(c *gin.Context) (page, limit int) {
	page = 1
	limit = 25
	if p, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && p > 0 {
		page = p
	}
	if l, err := strconv.Atoi(c.DefaultQuery("limit", "25")); err == nil && l > 0 && l <= 100 {
		limit = l
	}
	return
}
