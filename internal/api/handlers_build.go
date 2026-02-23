package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/cederikdotcom/hydraapi"
	"github.com/cederikdotcom/hydramonitor"
	"github.com/cederikdotcom/hydrarelease/internal/store"
)

type createBuildRequest struct {
	Project    string            `json:"project"`
	UploadedBy string            `json:"uploaded_by"`
	Source     string            `json:"source,omitempty"`
	SourceRef  string            `json:"source_ref,omitempty"`
	SourceMeta map[string]string `json:"source_meta,omitempty"`
	Files      []store.BuildFile `json:"files"`
}

func (s *Server) handleCreateBuild(w http.ResponseWriter, r *http.Request) {
	var req createBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		hydraapi.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Project == "" {
		hydraapi.WriteError(w, http.StatusBadRequest, "project is required")
		return
	}
	if len(req.Files) == 0 {
		hydraapi.WriteError(w, http.StatusBadRequest, "at least one file is required")
		return
	}

	build, err := s.Builds.Create(store.CreateParams{
		Project:    req.Project,
		UploadedBy: req.UploadedBy,
		Source:     req.Source,
		SourceRef:  req.SourceRef,
		SourceMeta: req.SourceMeta,
		Files:      req.Files,
	})
	if err != nil {
		hydraapi.WriteError(w, http.StatusInternalServerError, "failed to create build")
		return
	}

	// Emit SSE event.
	var totalBytes int64
	for _, f := range build.Files {
		totalBytes += f.Size
	}
	eventData := map[string]any{
		"district":     "",
		"timestamp":    build.UploadedAt.Format("2006-01-02T15:04:05Z07:00"),
		"project":      build.Project,
		"build_number": build.BuildNumber,
		"uploaded_by":  build.UploadedBy,
		"file_count":   len(build.Files),
		"total_bytes":  totalBytes,
	}
	if build.Source != "" {
		eventData["source"] = build.Source
	}
	if build.SourceRef != "" {
		eventData["source_ref"] = build.SourceRef
	}
	s.Monitor.Emit(hydramonitor.Event{
		Type: "build.uploaded",
		Data: eventData,
	})

	// Create mirror hardlinks for files with mirror_path (best-effort, non-blocking).
	if s.MirrorURL != "" {
		go s.linkMirrorFiles(build)
	}

	hydraapi.WriteJSON(w, http.StatusCreated, build)
}

// linkMirrorFiles calls hydramirror's link endpoint for each file that has a mirror_path.
// For each file, the source is the mirror_path (where the file was pushed during finalize)
// and the target is a build-specific path.
func (s *Server) linkMirrorFiles(build *store.Build) {
	for _, f := range build.Files {
		if f.MirrorPath == "" {
			continue
		}

		target := fmt.Sprintf("builds/%s/%d/%s", build.Project, build.BuildNumber, f.Path)

		body, _ := json.Marshal(map[string]any{
			"source":  f.MirrorPath,
			"targets": []string{target},
		})

		url := strings.TrimRight(s.MirrorURL, "/") + "/api/v1/link"
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			log.Printf("[mirror-link] failed to create request: %v", err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+s.MirrorToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("[mirror-link] failed to link %s -> %s: %v", f.MirrorPath, target, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			log.Printf("[mirror-link] linked %s -> %s", f.MirrorPath, target)
		} else {
			log.Printf("[mirror-link] link %s -> %s returned %d", f.MirrorPath, target, resp.StatusCode)
		}
	}
}

func (s *Server) handleListBuilds(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		hydraapi.WriteError(w, http.StatusBadRequest, "project query parameter is required")
		return
	}

	builds, err := s.Builds.List(project)
	if err != nil {
		hydraapi.WriteError(w, http.StatusInternalServerError, "failed to list builds")
		return
	}

	if builds == nil {
		builds = []store.BuildIndexEntry{}
	}
	hydraapi.WriteJSON(w, http.StatusOK, builds)
}

func (s *Server) handleGetBuild(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	numberStr := r.PathValue("number")

	number, err := strconv.Atoi(numberStr)
	if err != nil {
		hydraapi.WriteError(w, http.StatusBadRequest, "invalid build number")
		return
	}

	build, err := s.Builds.Get(project, number)
	if err != nil {
		hydraapi.WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	hydraapi.WriteJSON(w, http.StatusOK, build)
}
