package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cederikdotcom/hydraapi"
	"github.com/cederikdotcom/hydraauth"
	"github.com/cederikdotcom/hydramonitor"
	"github.com/cederikdotcom/hydrarelease/docs"
	"github.com/cederikdotcom/hydrarelease/internal/store"
)

// latestInfo holds the latest version info for a project/channel.
type latestInfo struct {
	Version     string `json:"version"`
	BuildNumber int    `json:"build_number,omitempty"`
}

// Server holds all dependencies for HTTP handlers.
type Server struct {
	Builds      *store.BuildStore
	Releases    *store.ReleaseStore
	Auth        *hydraauth.Auth
	Monitor     *hydramonitor.Monitor
	Version     string
	MirrorURL   string // hydramirror URL for file storage and redirects
	MirrorToken string // bearer token for hydramirror

	latestMu sync.RWMutex
	latest   map[string]latestInfo // key: "project/channel"

	// uploadSessions tracks SHA256 hashes for in-progress legacy publishes.
	uploadMu       sync.Mutex
	uploadSessions map[string]map[string]string // key: "project/channel/version" → filename → sha256
}

// SetLatest updates the latest version for a project/channel.
func (s *Server) SetLatest(project, channel, version string, buildNumber int) {
	s.latestMu.Lock()
	defer s.latestMu.Unlock()
	if s.latest == nil {
		s.latest = make(map[string]latestInfo)
	}
	s.latest[project+"/"+channel] = latestInfo{Version: version, BuildNumber: buildNumber}
}

// GetLatest returns the latest version for a project/channel.
// It checks the in-memory map first, then falls back to the ReleaseStore.
func (s *Server) GetLatest(project, channel string) (latestInfo, bool) {
	s.latestMu.RLock()
	info, ok := s.latest[project+"/"+channel]
	s.latestMu.RUnlock()
	if ok {
		return info, true
	}

	// Fall back to ReleaseStore with channel→env mapping.
	env := channelToEnv(channel)
	rel, err := s.Releases.Get(project, env)
	if err != nil {
		return latestInfo{}, false
	}
	return latestInfo{Version: rel.Version, BuildNumber: rel.BuildNumber}, true
}

// channelToEnv maps URL channel names to ReleaseStore environment names.
func channelToEnv(channel string) string {
	if channel == "production" {
		return "prod"
	}
	return channel
}

// envToChannel maps ReleaseStore environment names to URL channel names.
func envToChannel(env string) string {
	if env == "prod" {
		return "production"
	}
	return env
}

// InitLatest pre-populates the latest map from all current releases in the store.
// This ensures auto-updaters get valid latest.json responses immediately after startup.
func (s *Server) InitLatest() {
	releases, err := s.Releases.ListCurrentReleases()
	if err != nil {
		log.Printf("Warning: failed to load current releases: %v", err)
		return
	}
	for _, rel := range releases {
		channel := envToChannel(rel.Environment)
		s.SetLatest(rel.Project, channel, rel.Version, rel.BuildNumber)
	}
	if len(releases) > 0 {
		log.Printf("Pre-populated latest map with %d releases", len(releases))
	}
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
		data, _ := docs.Files.ReadFile("runbooks/runbook.md")
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
			s.Auth.RequireAuth(s.handleFinalize))
		mux.HandleFunc("POST /api/v1/publish/{project}/{channel}/{version}/{binary}",
			s.Auth.RequireAuth(s.handleUploadBinary))
	}

	// File serving via redirects to hydramirror.
	mux.HandleFunc("GET /{project}/{channel}/latest.json", s.handleLatestJSON)
	mux.HandleFunc("GET /{project}/{channel}/{version}/{file}", s.handleFileRedirect)

	return mux
}

// handleLatestJSON serves latest.json from the in-memory latest map or ReleaseStore.
func (s *Server) handleLatestJSON(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	channel := r.PathValue("channel")

	info, ok := s.GetLatest(project, channel)
	if !ok {
		hydraapi.WriteError(w, http.StatusNotFound, fmt.Sprintf("no release found for %s/%s", project, channel))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// handleFileRedirect returns a 302 redirect to hydramirror for file downloads.
func (s *Server) handleFileRedirect(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	channel := r.PathValue("channel")
	version := r.PathValue("version")
	file := r.PathValue("file")

	mirrorPath := fmt.Sprintf("releases/%s/%s/%s/%s", project, channel, version, file)
	redirectURL := strings.TrimRight(s.MirrorURL, "/") + "/api/v1/files/" + mirrorPath

	http.Redirect(w, r, redirectURL, http.StatusFound)
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
