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
