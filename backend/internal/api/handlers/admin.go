package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"backend/internal/database"
	"backend/internal/models"
	"backend/internal/transactionage"
)

// AdminTransactionFetchAgeHistorySnapshot is one hourly current-season
// transaction-fetch age distribution for the admin history chart. The age
// ranges are mutually exclusive, so the values stack to the full distribution.
type AdminTransactionFetchAgeHistorySnapshot struct {
	SnapshotAt                     time.Time `json:"snapshot_at"`
	NeverFetched                   int64     `json:"never_fetched"`
	FetchedWithinFourHours         int64     `json:"fetched_within_four_hours"`
	FetchedFourToEightHours        int64     `json:"fetched_four_to_eight_hours"`
	FetchedEightToTwelveHours      int64     `json:"fetched_eight_to_twelve_hours"`
	FetchedTwelveToSixteenHours    int64     `json:"fetched_twelve_to_sixteen_hours"`
	FetchedSixteenToTwentyHours    int64     `json:"fetched_sixteen_to_twenty_hours"`
	FetchedTwentyToTwentyFourHours int64     `json:"fetched_twenty_to_twenty_four_hours"`
	FetchedTwentyFourOrMoreHours   int64     `json:"fetched_twenty_four_or_more_hours"`
}

// AdminTransactionFetchAgeHistoryResponse is the response for the admin's
// hourly transaction-fetch age history chart.
type AdminTransactionFetchAgeHistoryResponse struct {
	Season    string                                    `json:"season"`
	Snapshots []AdminTransactionFetchAgeHistorySnapshot `json:"snapshots"`
}

const (
	defaultTransactionFetchAgeHistoryLimit = 168
	maxTransactionFetchAgeHistoryLimit     = 1000
)

// AdminSegmentRow is one league-format bucket: scoring type x superflex x size.
type AdminSegmentRow struct {
	Scoring      string `json:"scoring"`
	Superflex    bool   `json:"superflex"`
	LeagueSize   string `json:"league_size"`
	Leagues      int64  `json:"leagues"`
	Transactions int64  `json:"transactions"`
}

// AdminSegmentsResponse reports how fetched Sleeper leagues distribute across
// format segments, used to decide which segments are worth adding to the
// player-valuation model.
type AdminSegmentsResponse struct {
	TotalLeagues      int64             `json:"total_leagues"`
	TotalTransactions int64             `json:"total_transactions"`
	Segments          []AdminSegmentRow `json:"segments"`
}

// GetAdminSegments buckets all fetched, non-skipped Sleeper leagues by scoring
// type (PPR / 0.5 PPR / Standard), superflex, and league size (8 / 10 / 12 /
// 14+), returning per-bucket league and transaction counts sorted largest
// first by league count.
func GetAdminSegments(c *gin.Context) {
	const q = `
		SELECT
			CASE
				WHEN l.ppr = 1 THEN 'PPR'
				WHEN l.ppr = 0.5 THEN '0.5 PPR'
				WHEN l.ppr = 0 THEN 'Standard'
				ELSE 'Other'
			END AS scoring,
			COALESCE(l.is_superflex, FALSE) AS superflex,
			CASE
				WHEN l.total_rosters = 8 THEN '8'
				WHEN l.total_rosters = 10 THEN '10'
				WHEN l.total_rosters = 12 THEN '12'
				WHEN l.total_rosters >= 14 THEN '14+'
				ELSE 'Other'
			END AS league_size,
			COUNT(DISTINCT l.sleeper_league_id) AS leagues,
			COUNT(t.sleeper_transaction_id) AS transactions
		FROM sleeper_leagues l
		LEFT JOIN sleeper_transactions t ON t.sleeper_league_id = l.sleeper_league_id
		WHERE l.skipped_at IS NULL AND l.last_fetched_at IS NOT NULL
		GROUP BY scoring, superflex, league_size
		ORDER BY leagues DESC, scoring, superflex, league_size`

	rows := []AdminSegmentRow{}
	if err := database.DB.Raw(q).Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := AdminSegmentsResponse{Segments: rows}
	for _, r := range rows {
		resp.TotalLeagues += r.Leagues
		resp.TotalTransactions += r.Transactions
	}
	c.JSON(http.StatusOK, resp)
}

// AdminTableSizeRow is one table's on-disk size (including its indexes) and
// estimated row count.
type AdminTableSizeRow struct {
	TableName   string `json:"table_name"`
	SizeBytes   int64  `json:"size_bytes"`
	RowEstimate int64  `json:"row_estimate"`
}

// AdminDatabaseSizeResponse reports the total Postgres database size and a
// per-table breakdown, used to spot which tables are driving storage growth.
type AdminDatabaseSizeResponse struct {
	TotalBytes int64               `json:"total_bytes"`
	Tables     []AdminTableSizeRow `json:"tables"`
}

// GetAdminDatabaseSize reports the total on-disk size of the current
// Postgres database and a per-table breakdown (table + index bytes, sorted
// largest first) for the public schema.
func GetAdminDatabaseSize(c *gin.Context) {
	var totalBytes int64
	if err := database.DB.Raw(`SELECT pg_database_size(current_database())`).
		Scan(&totalBytes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	const q = `
		SELECT
			relname AS table_name,
			pg_total_relation_size(relid) AS size_bytes,
			n_live_tup AS row_estimate
		FROM pg_catalog.pg_stat_user_tables
		WHERE schemaname = 'public'
		ORDER BY size_bytes DESC`

	tables := []AdminTableSizeRow{}
	if err := database.DB.Raw(q).Scan(&tables).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, AdminDatabaseSizeResponse{TotalBytes: totalBytes, Tables: tables})
}

// AdminDiscoveryCounts is a total/expanded/pending/skipped breakdown for one
// entity type (sleeper_users, or sleeper_leagues within one season).
type AdminDiscoveryCounts struct {
	Total    int64 `json:"total"`
	Expanded int64 `json:"expanded"`
	Pending  int64 `json:"pending"`
	Skipped  int64 `json:"skipped"`
}

// AdminDiscoveryLeagueSeasonRow is the league discovery breakdown for one season.
type AdminDiscoveryLeagueSeasonRow struct {
	Season string `json:"season"`
	AdminDiscoveryCounts
}

// AdminDiscoveryFrontierResponse reports how much of the league/user discovery
// graph is known but not yet expanded, used to gauge remaining discovery work.
type AdminDiscoveryFrontierResponse struct {
	Users           AdminDiscoveryCounts            `json:"users"`
	LeaguesBySeason []AdminDiscoveryLeagueSeasonRow `json:"leagues_by_season"`
}

// GetAdminDiscoveryFrontier reports how many Sleeper users and leagues are
// known (discovered) but not yet expanded (last_fetched_at IS NULL) by the
// recursive discovery workflow, i.e. the size of the discovery frontier still
// left to fetch, plus how many have been expanded or permanently skipped.
func GetAdminDiscoveryFrontier(c *gin.Context) {
	var users AdminDiscoveryCounts
	const userQ = `
		SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE last_fetched_at IS NOT NULL) AS expanded,
			COUNT(*) FILTER (WHERE last_fetched_at IS NULL AND skipped_at IS NULL) AS pending,
			COUNT(*) FILTER (WHERE skipped_at IS NOT NULL) AS skipped
		FROM sleeper_users`
	if err := database.DB.Raw(userQ).Scan(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	const leagueQ = `
		SELECT
			season,
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE last_fetched_at IS NOT NULL) AS expanded,
			COUNT(*) FILTER (WHERE last_fetched_at IS NULL AND skipped_at IS NULL) AS pending,
			COUNT(*) FILTER (WHERE skipped_at IS NOT NULL) AS skipped
		FROM sleeper_leagues
		GROUP BY season
		ORDER BY season DESC`
	rows := []AdminDiscoveryLeagueSeasonRow{}
	if err := database.DB.Raw(leagueQ).Scan(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, AdminDiscoveryFrontierResponse{Users: users, LeaguesBySeason: rows})
}

// GetAdminTransactionFetchAgeHistory returns recent hourly fetch-age bucket
// snapshots for the current season, newest first. limit and skip operate on
// snapshot hours rather than individual bucket rows.
func GetAdminTransactionFetchAgeHistory(c *gin.Context) {
	limit := defaultTransactionFetchAgeHistoryLimit
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 {
		limit = min(v, maxTransactionFetchAgeHistoryLimit)
	}
	skip := 0
	if v, err := strconv.Atoi(c.Query("skip")); err == nil && v >= 0 {
		skip = v
	}

	season, err := transactionage.CurrentSeason(c.Request.Context(), database.DB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := AdminTransactionFetchAgeHistoryResponse{Season: season, Snapshots: []AdminTransactionFetchAgeHistorySnapshot{}}
	if season == "" {
		c.JSON(http.StatusOK, resp)
		return
	}

	var rows []models.SleeperTransactionFetchAgeSnapshot
	if err := database.DB.WithContext(c.Request.Context()).
		Where("season = ?", season).
		Order("snapshot_at DESC").
		Limit(limit).
		Offset(skip).
		Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(rows) == 0 {
		c.JSON(http.StatusOK, resp)
		return
	}

	resp.Snapshots = make([]AdminTransactionFetchAgeHistorySnapshot, len(rows))
	for i, row := range rows {
		resp.Snapshots[i] = AdminTransactionFetchAgeHistorySnapshot{
			SnapshotAt:                     row.SnapshotAt,
			NeverFetched:                   row.NeverFetched,
			FetchedWithinFourHours:         row.FetchedWithinFourHours,
			FetchedFourToEightHours:        row.FetchedFourToEightHours,
			FetchedEightToTwelveHours:      row.FetchedEightToTwelveHours,
			FetchedTwelveToSixteenHours:    row.FetchedTwelveToSixteenHours,
			FetchedSixteenToTwentyHours:    row.FetchedSixteenToTwentyHours,
			FetchedTwentyToTwentyFourHours: row.FetchedTwentyToTwentyFourHours,
			FetchedTwentyFourOrMoreHours:   row.FetchedTwentyFourOrMoreHours,
		}
	}

	c.JSON(http.StatusOK, resp)
}
