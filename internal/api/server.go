package api

import (
	"net/http"
	"time"

	"github.com/cederikdotcom/hydraapi"
	"github.com/cederikdotcom/hydraauth"
	"github.com/cederikdotcom/hydramonitor"
	"github.com/cederikdotcom/hydrarelease/docs"
	"github.com/cederikdotcom/hydrarelease/internal/store"
)

// Server holds all dependencies for HTTP handlers.
type Server struct {
	Builds      *store.BuildStore
	Releases    *store.ReleaseStore
	Auth        *hydraauth.Auth
	Monitor     *hydramonitor.Monitor
	FileDir     string // directory served by the file server (e.g. /var/www/releases)
	Version     string
	MirrorURL   string // optional hydramirror URL to push files to
	MirrorToken string // bearer token for hydramirror
}

// Handler returns the top-level HTTP handler with all routes registered.
func (s *Server) Handler(publishToken string, startTime time.Time) http.Handler {
	mux := http.NewServeMux()

	// Health endpoint (standard hydraapi).
	mux.HandleFunc("GET /api/v1/health", hydraapi.NewHealthHandler(
		"hydrarelease", s.Version, "", startTime, s.healthExtra,
	))

	// Runbook
	mux.HandleFunc("GET /api/v1/runbook", func(w http.ResponseWriter, r *http.Request) {
		data, _ := docs.Files.ReadFile("runbook.md")
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Write(data)
	})

	// SSE events.
	mux.HandleFunc("GET /api/v1/events", s.Auth.RequireAuth(s.Monitor.HandleEvents))

	// Build endpoints.
	mux.HandleFunc("POST /api/v1/builds", s.Auth.RequireAuth(s.handleCreateBuild))
	mux.HandleFunc("GET /api/v1/builds", s.handleListBuilds)
	mux.HandleFunc("GET /api/v1/builds/{project}/{number}", s.handleGetBuild)

	// Release endpoints.
	mux.HandleFunc("POST /api/v1/releases", s.Auth.RequireAuth(s.handlePromoteRelease))
	mux.HandleFunc("POST /api/v1/releases/rollback", s.Auth.RequireAuth(s.handleRollbackRelease))
	mux.HandleFunc("GET /api/v1/releases", s.handleListReleases)
	mux.HandleFunc("GET /api/v1/releases/{project}/{env}", s.handleGetRelease)

	// Legacy publish endpoints (backward compat for existing CI).
	if publishToken != "" {
		mux.HandleFunc("POST /api/v1/publish/{project}/{channel}/{version}/finalize",
			s.Auth.RequireAuth(handleFinalize(s.FileDir, s.MirrorURL, s.MirrorToken)))
		mux.HandleFunc("POST /api/v1/publish/{project}/{channel}/{version}/{binary}",
			s.Auth.RequireAuth(handleUploadBinary(s.FileDir)))
	}

	// Static file server.
	mux.Handle("/", http.FileServer(http.Dir(s.FileDir)))

	return mux
}

func (s *Server) healthExtra() map[string]any {
	extra := make(map[string]any)

	buildCount, projects, err := s.Builds.Stats()
	if err == nil {
		extra["build_count"] = buildCount
		extra["projects"] = projects
	}

	releaseCount, err := s.Releases.Stats()
	if err == nil {
		extra["release_count"] = releaseCount
	}

	return extra
}
