package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// parseLeagueID extracts :leagueId from the path, writes 400 and returns false on failure.
func parseLeagueID(c *gin.Context) (uint, bool) {
	val, err := strconv.ParseUint(c.Param("leagueId"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid leagueId"})
		return 0, false
	}
	return uint(val), true
}
