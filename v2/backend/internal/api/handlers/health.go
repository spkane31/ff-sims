package handlers

import (
	"backend/pkg/version"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetPlayers returns all players with optional filtering
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"GitSHA":    version.GitSHA,
		"BuildTime": version.BuildTime,
	})
}
