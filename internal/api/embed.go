package api

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:webdist
var webDistRaw embed.FS

// webDist strips the "webdist" embed prefix so the file server sees
// paths the way a browser requests them ("/", "/assets/...").
var webDist = mustSub(webDistRaw, "webdist")

func mustSub(f embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

// registerStaticRoutes serves the built Vue SPA for every path go:embed
// doesn't recognize as /api/*. It falls back to index.html for unknown
// paths so client-side routes (e.g. /epochs/5, a full page load) resolve
// instead of 404ing — vue-router then takes over in the browser.
// In setup-only mode the public app is not configured yet, so HTML routes
// redirect to /setup and unknown /api/* calls return setup_required.
func (a *API) registerStaticRoutes(mux *http.ServeMux) {
	fileServer := http.FileServerFS(webDist)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			if a.setupOnly {
				writeAPIError(w, http.StatusServiceUnavailable, "setup_required", "pool is not configured")
				return
			}
			writeError(w, http.StatusNotFound, "API endpoint not found")
			return
		}
		if a.setupOnly {
			rel := strings.TrimPrefix(r.URL.Path, "/")
			if rel != "" {
				if _, err := fs.Stat(webDist, rel); err == nil {
					fileServer.ServeHTTP(w, r)
					return
				}
			}
			if r.URL.Path != "/setup" {
				http.Redirect(w, r, "/setup", http.StatusFound)
				return
			}
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(webDist, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
