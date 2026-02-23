package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/cederikdotcom/hydraapi"
	"github.com/cederikdotcom/hydrarelease/internal/store"
)

var validNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func validatePublishParams(project, channel, version, binary string) error {
	if !validNameRe.MatchString(project) {
		return fmt.Errorf("invalid project name: %q", project)
	}
	if channel != "production" && channel != "staging" {
		return fmt.Errorf("invalid channel: %q (must be production or staging)", channel)
	}
	if !validNameRe.MatchString(version) {
		return fmt.Errorf("invalid version: %q", version)
	}
	if binary != "" {
		if binary == "finalize" {
			return fmt.Errorf("binary name %q is reserved", binary)
		}
		if !validNameRe.MatchString(binary) {
			return fmt.Errorf("invalid binary name: %q", binary)
		}
	}
	return nil
}

// handleUploadBinary streams a binary upload directly to hydramirror,
// computing the SHA256 hash on the fly for later use in finalize.
func (s *Server) handleUploadBinary(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	channel := r.PathValue("channel")
	version := r.PathValue("version")
	binary := r.PathValue("binary")

	if err := validatePublishParams(project, channel, version, binary); err != nil {
		hydraapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if s.MirrorURL == "" {
		hydraapi.WriteError(w, http.StatusServiceUnavailable, "mirror not configured")
		return
	}

	// Stream body to mirror while computing SHA256.
	mirrorPath := fmt.Sprintf("releases/%s/%s/%s/%s", project, channel, version, binary)
	url := strings.TrimRight(s.MirrorURL, "/") + "/api/v1/files/" + mirrorPath

	hasher := sha256.New()
	body := io.TeeReader(r.Body, hasher)

	req, err := http.NewRequest("PUT", url, body)
	if err != nil {
		log.Printf("publish: create mirror request: %v", err)
		hydraapi.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	req.Header.Set("Authorization", "Bearer "+s.MirrorToken)

	client := &http.Client{Timeout: 5 * 60 * 1000000000} // 5 minutes
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("publish: mirror PUT %s: %v", mirrorPath, err)
		hydraapi.WriteError(w, http.StatusBadGateway, "failed to upload to mirror")
		return
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		log.Printf("publish: mirror PUT %s returned %d", mirrorPath, resp.StatusCode)
		hydraapi.WriteError(w, http.StatusBadGateway, fmt.Sprintf("mirror returned %d", resp.StatusCode))
		return
	}

	// Store hash for finalize.
	hash := hex.EncodeToString(hasher.Sum(nil))
	sessionKey := project + "/" + channel + "/" + version

	s.uploadMu.Lock()
	if s.uploadSessions == nil {
		s.uploadSessions = make(map[string]map[string]string)
	}
	if s.uploadSessions[sessionKey] == nil {
		s.uploadSessions[sessionKey] = make(map[string]string)
	}
	s.uploadSessions[sessionKey][binary] = hash
	s.uploadMu.Unlock()

	log.Printf("publish: uploaded %s/%s/%s/%s to mirror", project, channel, version, binary)
	hydraapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "binary": binary})
}

// handleFinalize generates SHA256SUMS from tracked hashes, uploads it to mirror,
// and updates the latest version tracking.
func (s *Server) handleFinalize(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	channel := r.PathValue("channel")
	version := r.PathValue("version")

	if err := validatePublishParams(project, channel, version, ""); err != nil {
		hydraapi.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if s.MirrorURL == "" {
		hydraapi.WriteError(w, http.StatusServiceUnavailable, "mirror not configured")
		return
	}

	// Get tracked hashes from upload session.
	sessionKey := project + "/" + channel + "/" + version

	s.uploadMu.Lock()
	files := s.uploadSessions[sessionKey]
	delete(s.uploadSessions, sessionKey)
	s.uploadMu.Unlock()

	if len(files) == 0 {
		hydraapi.WriteError(w, http.StatusBadRequest, "no files uploaded for this version; upload binaries first")
		return
	}

	// Generate SHA256SUMS content.
	var sums strings.Builder
	for name, hash := range files {
		fmt.Fprintf(&sums, "%s  %s\n", hash, name)
	}

	// Upload SHA256SUMS to mirror.
	sumsContent := sums.String()
	mirrorPath := fmt.Sprintf("releases/%s/%s/%s/SHA256SUMS", project, channel, version)
	url := strings.TrimRight(s.MirrorURL, "/") + "/api/v1/files/" + mirrorPath

	req, err := http.NewRequest("PUT", url, strings.NewReader(sumsContent))
	if err != nil {
		log.Printf("publish: create SHA256SUMS request: %v", err)
		hydraapi.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	req.Header.Set("Authorization", "Bearer "+s.MirrorToken)

	client := &http.Client{Timeout: 30 * 1000000000} // 30 seconds
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("publish: mirror PUT SHA256SUMS: %v", err)
		hydraapi.WriteError(w, http.StatusBadGateway, "failed to upload SHA256SUMS to mirror")
		return
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		log.Printf("publish: mirror PUT SHA256SUMS returned %d", resp.StatusCode)
		hydraapi.WriteError(w, http.StatusBadGateway, fmt.Sprintf("mirror returned %d", resp.StatusCode))
		return
	}

	// Persist to ReleaseStore so latest survives restarts.
	cleanVersion := strings.TrimPrefix(version, "v")
	env := channelToEnv(channel)
	_, err = s.Releases.Promote(store.PromoteRequest{
		Project:     project,
		Environment: env,
		Version:     cleanVersion,
		ReleasedBy:  "publish-api",
	})
	if err != nil {
		log.Printf("publish: warning: failed to persist release to store: %v", err)
	}

	// Update latest version tracking.
	s.SetLatest(project, channel, cleanVersion, 0)

	log.Printf("publish: finalized %s/%s/%s (%d files)", project, channel, version, len(files))
	hydraapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": cleanVersion, "channel": channel})
}
