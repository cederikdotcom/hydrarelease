package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cederikdotcom/hydraapi"
	"github.com/cederikdotcom/hydramonitor"
	"github.com/cederikdotcom/hydrarelease/internal/store"
)

type promoteRequest struct {
	Project      string `json:"project"`
	Environment  string `json:"environment"`
	BuildNumber  int    `json:"build_number"`
	Version      string `json:"version"`
	ReleasedBy   string `json:"released_by"`
	ReleaseNotes string `json:"release_notes"`
}

type rollbackRequest struct {
	Project      string `json:"project"`
	Environment  string `json:"environment"`
	RolledBackBy string `json:"rolled_back_by"`
}

var validEnvironments = map[string]bool{
	"dev":     true,
	"staging": true,
	"prod":    true,
}

func (s *Server) handlePromoteRelease(w http.ResponseWriter, r *http.Request) {
	var req promoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hydraapi.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Project == "" {
		hydraapi.WriteError(w, http.StatusBadRequest, "project is required")
		return
	}
	if !validEnvironments[req.Environment] {
		hydraapi.WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid environment: %q (must be dev, staging, or prod)", req.Environment))
		return
	}
	if req.BuildNumber <= 0 {
		hydraapi.WriteError(w, http.StatusBadRequest, "build_number must be positive")
		return
	}
	if req.Version == "" {
		hydraapi.WriteError(w, http.StatusBadRequest, "version is required")
		return
	}

	// Verify the build exists.
	if _, err := s.Builds.Get(req.Project, req.BuildNumber); err != nil {
		hydraapi.WriteError(w, http.StatusBadRequest, fmt.Sprintf("build %s/%d not found", req.Project, req.BuildNumber))
		return
	}

	rel, err := s.Releases.Promote(store.PromoteRequest{
		Project:      req.Project,
		Environment:  req.Environment,
		BuildNumber:  req.BuildNumber,
		Version:      req.Version,
		ReleasedBy:   req.ReleasedBy,
		ReleaseNotes: req.ReleaseNotes,
	})
	if err != nil {
		hydraapi.WriteError(w, http.StatusInternalServerError, "failed to promote release")
		return
	}

	// Emit SSE event.
	s.Monitor.Emit(hydramonitor.Event{
		Type: "release.promoted",
		Data: map[string]any{
			"district":     "",
			"timestamp":    rel.ReleasedAt.Format("2006-01-02T15:04:05Z07:00"),
			"project":      rel.Project,
			"environment":  rel.Environment,
			"build_number": rel.BuildNumber,
			"version":      rel.Version,
			"released_by":  rel.ReleasedBy,
		},
	})

	hydraapi.WriteJSON(w, http.StatusCreated, rel)
}

func (s *Server) handleRollbackRelease(w http.ResponseWriter, r *http.Request) {
	var req rollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hydraapi.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Project == "" {
		hydraapi.WriteError(w, http.StatusBadRequest, "project is required")
		return
	}
	if !validEnvironments[req.Environment] {
		hydraapi.WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid environment: %q", req.Environment))
		return
	}

	// Get current release before rollback for the SSE event.
	current, err := s.Releases.Get(req.Project, req.Environment)
	if err != nil {
		hydraapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	rel, err := s.Releases.Rollback(req.Project, req.Environment, req.RolledBackBy)
	if err != nil {
		hydraapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Emit SSE event.
	s.Monitor.Emit(hydramonitor.Event{
		Type: "release.rolled-back",
		Data: map[string]any{
			"district":       "",
			"timestamp":      rel.ReleasedAt.Format("2006-01-02T15:04:05Z07:00"),
			"project":        rel.Project,
			"environment":    rel.Environment,
			"from_build":     current.BuildNumber,
			"to_build":       rel.BuildNumber,
			"rolled_back_by": req.RolledBackBy,
		},
	})

	hydraapi.WriteJSON(w, http.StatusOK, rel)
}

func (s *Server) handleListReleases(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		hydraapi.WriteError(w, http.StatusBadRequest, "project query parameter is required")
		return
	}

	releases, err := s.Releases.List(project)
	if err != nil {
		hydraapi.WriteError(w, http.StatusInternalServerError, "failed to list releases")
		return
	}

	if releases == nil {
		releases = []store.ReleaseIndexEntry{}
	}
	hydraapi.WriteJSON(w, http.StatusOK, releases)
}

func (s *Server) handleGetRelease(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	env := r.PathValue("env")

	rel, err := s.Releases.Get(project, env)
	if err != nil {
		hydraapi.WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	hydraapi.WriteJSON(w, http.StatusOK, rel)
}
