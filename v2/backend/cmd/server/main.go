package main

import (
	"backend/internal/api"
	"backend/internal/config"
	"backend/internal/database"
	"backend/pkg/version"
	"log"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}
	if err := database.Initialize(cfg); err != nil {
		log.Printf("Warning: Could not initialize database: %v", err)
		log.Printf("Server will continue without database connection")
	}

	// Create a gin router with default middleware
	r := gin.Default()
	// Configure CORS - most permissive settings
	config := cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD", "PATCH"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"*"},
		AllowCredentials: false,     // Set to false when AllowAllOrigins is true
		MaxAge:           12 * 3600, // 12 hours
	}
	r.Use(cors.New(config))

	api.SetupRouter(r)

	// No static file serving - nginx handles all routing

	log.Printf("Server starting on :8080, version: %s, build time: %s", version.GitSHA, version.BuildTime)
	log.Fatal(r.Run(":8080"))
}
