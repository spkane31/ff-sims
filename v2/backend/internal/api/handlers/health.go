package handlers

import (
	"backend/pkg/version"
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthCheck returns application health and version information
func HealthCheck(c *gin.Context) {
	response := gin.H{
		"GitSHA":    version.GitSHA,
		"BuildTime": version.BuildTime,
		"status":    "healthy",
	}
	
	// Add debug info if values are empty
	if version.GitSHA == "" {
		response["GitSHA"] = "not-set-during-build"
	}
	if version.BuildTime == "" {
		response["BuildTime"] = "not-set-during-build"  
	}
	
	c.JSON(http.StatusOK, response)
}
