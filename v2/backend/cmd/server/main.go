package main

import (
	"backend/internal/api"
	"backend/internal/config"
	"backend/internal/database"
	"backend/pkg/version"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

//go:embed static/*
var staticFiles embed.FS

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}
	if err := database.Initialize(cfg); err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}

	// Create a gin router with default middleware
	r := gin.Default()
	r.Use(cors.Default())

	api.SetupRouter(r)

	// Serve static files
	staticFS := getStaticFS()
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// If it's an API route, let it 404 naturally
		if strings.HasPrefix(path, "/api/") {
			c.Status(http.StatusNotFound)
			return
		}

		// Try serving static files
		http.FileServer(staticFS).ServeHTTP(c.Writer, c.Request)
	})

	log.Printf("Server starting on :8080, version: %s, build time: %s", version.GitSHA, version.BuildTime)
	log.Fatal(r.Run(":8080"))
}

// Get file system for static files
func getStaticFS() http.FileSystem {
	// When using embed.FS (if files are embedded)
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		// Fallback to regular file system (when files are copied)
		log.Printf("Error creating sub FS: %v, falling back to regular file system", err)
		return http.Dir("./static")
	}

	// Log files in the embedded filesystem for debugging
	fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		log.Printf("Embedded file: %s, isDir: %t\n", path, d.IsDir())
		return nil
	})

	return http.FS(sub)
}
