package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetPlayers returns all players with optional filtering
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"ping": "pong",
	})
}
