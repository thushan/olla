package dashboard

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

// Embed the built dashboard files
//
//go:embed all:dist
var dashboardFS embed.FS

// Handler returns an http.Handler that serves the embedded dashboard
func Handler() (http.Handler, error) {
	// Get the dist subdirectory
	distFS, err := fs.Sub(dashboardFS, "dist")
	if err != nil {
		return nil, err
	}

	// Return handler that serves the dashboard
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path - the handler already has /dashboard stripped
		path := r.URL.Path
		if path == "/" || path == "" {
			path = "/index.html"
		}
		
		// Remove leading slash for fs.Open
		cleanPath := strings.TrimPrefix(path, "/")
		
		// Try to open the file
		file, err := distFS.Open(cleanPath)
		if err != nil {
			// If file not found, serve index.html for SPA routing
			w.Header().Set("Content-Type", "text/html")
			file, err := distFS.Open("index.html")
			if err != nil {
				http.Error(w, "Dashboard not found", http.StatusNotFound)
				return
			}
			defer file.Close()
			http.ServeContent(w, r, "index.html", time.Time{}, file.(io.ReadSeeker))
			return
		}
		defer file.Close()
		
		// Set correct MIME types
		switch {
		case strings.HasSuffix(path, ".js"):
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		case strings.HasSuffix(path, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(path, ".html"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case strings.HasSuffix(path, ".svg"):
			w.Header().Set("Content-Type", "image/svg+xml")
		}
		
		// Serve the file
		http.ServeContent(w, r, cleanPath, time.Time{}, file.(io.ReadSeeker))
	}), nil
}

// isAssetPath checks if the path is for a static asset
func isAssetPath(path string) bool {
	// Common asset extensions
	extensions := []string{
		".js", ".css", ".svg", ".png", ".jpg", ".jpeg", 
		".gif", ".ico", ".woff", ".woff2", ".ttf", ".eot",
	}
	
	for _, ext := range extensions {
		if len(path) > len(ext) && path[len(path)-len(ext):] == ext {
			return true
		}
	}
	
	// Check for specific directories
	if len(path) > 8 && path[:8] == "/assets/" {
		return true
	}
	
	return false
}