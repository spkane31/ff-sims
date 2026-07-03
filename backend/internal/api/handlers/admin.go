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
	Scoring    string `json:"scoring"`
	Superflex  bool   `json:"superflex"`
	LeagueSize string `json:"league_size"`
	Leagues    int64  `json:"leagues"`
}

// AdminSegmentsResponse reports how fetched Sleeper leagues distribute across
// format segments, used to decide which segments are worth adding to the
// player-valuation model.
type AdminSegmentsResponse struct {
	TotalLeagues int64             `json:"total_leagues"`
	Segments     []AdminSegmentRow `json:"segments"`
}

// GetAdminSegments buckets all fetched, non-skipped Sleeper leagues by scoring
// type (PPR / 0.5 PPR / Standard), superflex, and league size (8 / 10 / 12 /
// 14+), returning per-bucket counts sorted largest first.
func GetAdminSegments(c *gin.Context) {
	const q = `
		SELECT
			CASE
				WHEN ppr = 1 THEN 'PPR'
				WHEN ppr = 0.5 THEN '0.5 PPR'
				WHEN ppr = 0 THEN 'Standard'
				ELSE 'Other'
			END AS scoring,
			COALESCE(is_superflex, FALSE) AS superflex,
			CASE
				WHEN total_rosters = 8 THEN '8'
				WHEN total_rosters = 10 THEN '10'
				WHEN total_rosters = 12 THEN '12'
				WHEN total_rosters >= 14 THEN '14+'
				ELSE 'Other'
			END AS league_size,
			COUNT(*) AS leagues
		FROM sleeper_leagues
		WHERE skipped_at IS NULL AND last_fetched_at IS NOT NULL
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
	}
	c.JSON(http.StatusOK, resp)
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
