package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cederikdotcom/hydraapi"
	"github.com/cederikdotcom/hydrarelease/pkg/upload"
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

func handleUploadBinary(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		channel := r.PathValue("channel")
		version := r.PathValue("version")
		binary := r.PathValue("binary")

		if err := validatePublishParams(project, channel, version, binary); err != nil {
			hydraapi.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		versionDir := filepath.Join(dir, project, channel, version)
		if err := os.MkdirAll(versionDir, 0755); err != nil {
			log.Printf("publish: mkdir %s: %v", versionDir, err)
			hydraapi.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}

		destPath := filepath.Join(versionDir, binary)
		if err := upload.AtomicWrite(destPath, r.Body, 0755); err != nil {
			log.Printf("publish: write %s: %v", destPath, err)
			hydraapi.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}

		log.Printf("publish: uploaded %s/%s/%s/%s", project, channel, version, binary)
		hydraapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "binary": binary})
	}
}

func handleFinalize(dir, mirrorURL, mirrorToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.PathValue("project")
		channel := r.PathValue("channel")
		version := r.PathValue("version")

		if err := validatePublishParams(project, channel, version, ""); err != nil {
			hydraapi.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		versionDir := filepath.Join(dir, project, channel, version)
		entries, err := os.ReadDir(versionDir)
		if err != nil {
			hydraapi.WriteError(w, http.StatusBadRequest, "version directory not found; upload binaries first")
			return
		}

		// Generate SHA256SUMS.
		var sums strings.Builder
		for _, e := range entries {
			if e.IsDir() || e.Name() == "SHA256SUMS" {
				continue
			}
			hash, err := upload.HashFile(filepath.Join(versionDir, e.Name()))
			if err != nil {
				log.Printf("publish: hash %s: %v", e.Name(), err)
				hydraapi.WriteError(w, http.StatusInternalServerError, "internal error")
				return
			}
			fmt.Fprintf(&sums, "%s  %s\n", hash, e.Name())
		}

		sumsPath := filepath.Join(versionDir, "SHA256SUMS")
		if err := atomicWriteFile(sumsPath, []byte(sums.String()), 0644); err != nil {
			log.Printf("publish: write SHA256SUMS: %v", err)
			hydraapi.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}

		// Strip leading "v" for latest.json.
		cleanVersion := strings.TrimPrefix(version, "v")

		// Write latest.json.
		channelDir := filepath.Join(dir, project, channel)
		latestJSON, _ := json.Marshal(map[string]string{"version": cleanVersion})
		if err := atomicWriteFile(filepath.Join(channelDir, "latest.json"), latestJSON, 0644); err != nil {
			log.Printf("publish: write latest.json: %v", err)
			hydraapi.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}

		// Update latest symlink.
		latestLink := filepath.Join(channelDir, "latest")
		os.Remove(latestLink)
		os.Symlink(version, latestLink)

		// Push version files to hydramirror (best-effort, non-blocking).
		if mirrorURL != "" {
			go pushToMirror(mirrorURL, mirrorToken, dir, project, channel, version)
		}

		log.Printf("publish: finalized %s/%s/%s", project, channel, version)
		hydraapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": cleanVersion, "channel": channel})
	}
}

// pushToMirror pushes all files in a version directory to hydramirror.
func pushToMirror(mirrorURL, mirrorToken, dir, project, channel, version string) {
	versionDir := filepath.Join(dir, project, channel, version)
	entries, err := os.ReadDir(versionDir)
	if err != nil {
		log.Printf("mirror-push: failed to read %s: %v", versionDir, err)
		return
	}

	client := &http.Client{Timeout: 5 * 60 * 1000000000} // 5 minutes
	for _, e := range entries {
		if e.IsDir() {
			continue
		}

		mirrorPath := fmt.Sprintf("releases/%s/%s/%s/%s", project, channel, version, e.Name())
		filePath := filepath.Join(versionDir, e.Name())

		f, err := os.Open(filePath)
		if err != nil {
			log.Printf("mirror-push: failed to open %s: %v", filePath, err)
			continue
		}

		url := strings.TrimRight(mirrorURL, "/") + "/api/v1/files/" + mirrorPath
		req, err := http.NewRequest("PUT", url, f)
		if err != nil {
			f.Close()
			log.Printf("mirror-push: failed to create request for %s: %v", mirrorPath, err)
			continue
		}
		req.Header.Set("Authorization", "Bearer "+mirrorToken)

		resp, err := client.Do(req)
		f.Close()
		if err != nil {
			log.Printf("mirror-push: failed to push %s: %v", mirrorPath, err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			log.Printf("mirror-push: %s returned %d", mirrorPath, resp.StatusCode)
			continue
		}

		log.Printf("mirror-push: pushed %s", mirrorPath)
	}
}
