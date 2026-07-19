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

	subDir := filepath.Join(dir, seg)
	if info, err := os.Stat(subDir); err == nil && info.IsDir() {
		if f, ok := matchRouteSegments(subDir, rest); ok {
			return f, true
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "[") {
			continue
		}
		if len(rest) == 0 && !e.IsDir() && strings.HasSuffix(name, "].html") {
			return filepath.Join(dir, name), true
		}
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

	r := gin.Default()
	config := cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD", "PATCH"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"*"},
		AllowCredentials: false, // Set to false when AllowAllOrigins is true
		MaxAge:           12 * 3600,
	}
	r.Use(cors.New(config))

	api.SetupRouter(r)

	r.Static("/_next/static", "/app/frontend/.next/static")

	r.StaticFS("/public", http.Dir("/app/frontend/public"))

	r.StaticFile("/favicon.ico", "/app/frontend/public/favicon.ico")
	r.StaticFile("/robots.txt", "/app/frontend/public/robots.txt")

	// Handle all non-API routes with Next.js server-side rendered pages or fallback
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		if strings.HasPrefix(path, "/api/") {
			c.Status(http.StatusNotFound)
			return
		}

		// Skip _next static assets (already handled above)
		if strings.HasPrefix(path, "/_next/") {
			c.Status(http.StatusNotFound)
			return
		}

		publicFile := filepath.Join("/app/frontend/public", path)
		if _, err := os.Stat(publicFile); err == nil && !isDirectory(publicFile) {
			c.File(publicFile)
			return
		}

		htmlFile := "/app/frontend/.next/server/pages" + path + ".html"
		if path == "/" {
			htmlFile = "/app/frontend/.next/server/pages/index.html"
		}

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
