package middleware

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

const LeagueIDKey = "league_id"

// LeagueContextMiddleware extracts league ID from URL param and adds to context
func LeagueContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		leagueIDStr := c.Param("leagueId")
		leagueID, err := strconv.ParseUint(leagueIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid league ID"})
			c.Abort()
			return
		}
		c.Set(LeagueIDKey, uint(leagueID))
		c.Next()
	}
}

// DefaultLeagueMiddleware sets default league ID for legacy routes
func DefaultLeagueMiddleware(defaultID uint) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if league_id query param exists, otherwise use default
		leagueIDStr := c.Query("league_id")
		if leagueIDStr != "" {
			leagueID, err := strconv.ParseUint(leagueIDStr, 10, 32)
			if err == nil {
				c.Set(LeagueIDKey, uint(leagueID))
				c.Next()
				return
			}
		}
		c.Set(LeagueIDKey, defaultID)
		c.Next()
	}
}

// GetLeagueID extracts league ID from context
func GetLeagueID(c *gin.Context) uint {
	if val, exists := c.Get(LeagueIDKey); exists {
		return val.(uint)
	}
	return 345674 // Fallback to default
}
