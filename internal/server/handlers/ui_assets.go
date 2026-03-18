package handlers

import (
	"embed"
	"net/http"
)

//go:embed ui/templates ui/assets
var uiFS embed.FS

// ServeUIAsset handles requests to /ui/assets/ by stripping the prefix
// and using the embedded filesystem to return the file.
func ServeUIAsset(w http.ResponseWriter, r *http.Request) {
	// If it's a JS file, manually set content type just in case
	importPath := "ui/assets/" + r.URL.Path[len("/ui/assets/"):]
	
	data, err := uiFS.ReadFile(importPath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	
	if len(importPath) > 3 && importPath[len(importPath)-3:] == ".js" {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	} else if len(importPath) > 4 && importPath[len(importPath)-4:] == ".css" {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	}
	setSecurityHeaders(w)
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// setSecurityHeaders applies a baseline set of security headers to every UI response.
func setSecurityHeaders(w http.ResponseWriter) {
	// Restrict resource loading to same origin; allow inline styles/scripts needed by
	// the current single-file page architecture. eval() and external sources are blocked.
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; frame-ancestors 'none'; form-action 'self'; object-src 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
}

