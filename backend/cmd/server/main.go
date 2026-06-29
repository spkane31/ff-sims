package main

import (
	"backend/internal/api"
	"backend/internal/config"
	"backend/internal/database"
	"backend/pkg/version"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// matchRouteSegments recursively walks the Next.js pages output directory,
// matching URL segments against both static and dynamic ([param]) entries.
func matchRouteSegments(dir string, segments []string) (string, bool) {
	if len(segments) == 0 {
		f := filepath.Join(dir, "index.html")
		if _, err := os.Stat(f); err == nil {
			return f, true
		}
		return "", false
	}

	seg := segments[0]
	rest := segments[1:]

	// Static: exact segment as .html file (only valid for the last segment)
	if len(rest) == 0 {
		f := filepath.Join(dir, seg+".html")
		if _, err := os.Stat(f); err == nil {
			return f, true
		}
	}

	// Static: exact segment as subdirectory
	subDir := filepath.Join(dir, seg)
	if info, err := os.Stat(subDir); err == nil && info.IsDir() {
		if f, ok := matchRouteSegments(subDir, rest); ok {
			return f, true
		}
	}

	// Dynamic: scan for [param] entries in current directory
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "[") {
			continue
		}
		// Dynamic file match for last segment: [param].html
		if len(rest) == 0 && !e.IsDir() && strings.HasSuffix(name, "].html") {
			return filepath.Join(dir, name), true
		}
		// Dynamic directory match: recurse
		if e.IsDir() {
			if f, ok := matchRouteSegments(filepath.Join(dir, name), rest); ok {
				return f, true
			}
		}
	}

	return "", false
}

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

	// Static file serving for Next.js build assets
	r.Static("/_next/static", "/app/frontend/.next/static")

	// Serve public files with higher priority
	r.StaticFS("/public", http.Dir("/app/frontend/public"))

	// Individual static file routes for common public files
	r.StaticFile("/favicon.ico", "/app/frontend/public/favicon.ico")
	r.StaticFile("/robots.txt", "/app/frontend/public/robots.txt")

	// Handle all non-API routes with Next.js server-side rendered pages or fallback
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// Skip API routes
		if strings.HasPrefix(path, "/api/") {
			c.Status(http.StatusNotFound)
			return
		}

		// Skip _next static assets (already handled above)
		if strings.HasPrefix(path, "/_next/") {
			c.Status(http.StatusNotFound)
			return
		}

		// Try to serve static files from public directory first
		publicFile := filepath.Join("/app/frontend/public", path)
		if _, err := os.Stat(publicFile); err == nil && !isDirectory(publicFile) {
			c.File(publicFile)
			return
		}

		// For Next.js pages, try to serve pre-built HTML
		htmlFile := "/app/frontend/.next/server/pages" + path + ".html"
		if path == "/" {
			htmlFile = "/app/frontend/.next/server/pages/index.html"
		}

		// If HTML file exists, serve it
		if _, err := os.Stat(htmlFile); err == nil {
			c.File(htmlFile)
			return
		}

		// For dynamic Next.js routes (e.g. /league/1 -> /league/[leagueId].html)
		pagesDir := "/app/frontend/.next/server/pages"
		pathParts := strings.Split(strings.Trim(path, "/"), "/")
		if f, ok := matchRouteSegments(pagesDir, pathParts); ok {
			c.File(f)
			return
		}

		// Final fallback to index.html for client-side routing
		indexFile := "/app/frontend/.next/server/pages/index.html"
		if _, err := os.Stat(indexFile); err == nil {
			c.File(indexFile)
		} else {
			c.Status(http.StatusNotFound)
		}
	})

	log.Printf("Server starting on :8080, version: %s, build time: %s", version.GitSHA, version.BuildTime)
	log.Fatal(r.Run(":8080"))
}
