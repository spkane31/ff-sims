package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"backend/internal/database"
	"backend/internal/models"
)

// AdminBacklogResponse reports the Sleeper transaction-sync backlog for the
// current season, used to size Temporal worker throughput.
type AdminBacklogResponse struct {
	Season                      string     `json:"season"`
	TotalLeagues                int64      `json:"total_leagues"`
	NeverFetchedCount           int64      `json:"never_fetched_count"`
	OldestTransactionsFetchedAt *time.Time `json:"oldest_transactions_fetched_at"`
}

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

// GetAdminBacklog returns how many leagues in the current season (the max
// value of sleeper_leagues.season) have never had transactions fetched, and
// the oldest last_transactions_fetched_at among the ones that have.
func GetAdminBacklog(c *gin.Context) {
	var season string
	if err := database.DB.Model(&models.SleeperLeague{}).
		Select("COALESCE(MAX(season), '')").
		Scan(&season).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var resp AdminBacklogResponse
	resp.Season = season

	if err := database.DB.Model(&models.SleeperLeague{}).
		Where("season = ? AND skipped_at IS NULL", season).
		Count(&resp.TotalLeagues).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := database.DB.Model(&models.SleeperLeague{}).
		Where("season = ? AND skipped_at IS NULL AND last_transactions_fetched_at IS NULL", season).
		Count(&resp.NeverFetchedCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var oldestLeague models.SleeperLeague
	err := database.DB.
		Where("season = ? AND skipped_at IS NULL AND last_transactions_fetched_at IS NOT NULL", season).
		Order("last_transactions_fetched_at ASC").
		Limit(1).
		Take(&oldestLeague).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err == nil {
		resp.OldestTransactionsFetchedAt = oldestLeague.LastTransactionsFetchedAt
	}

	c.JSON(http.StatusOK, resp)
}
