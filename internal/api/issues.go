package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// resolveIssues resolves the given issue IDs via the issue tracker API.
// It runs asynchronously and logs results without affecting the caller.
func (s *Server) resolveIssues(issueIDs []string, version, project string) {
	if s.IssueTrackerURL == "" || s.IssueTrackerToken == "" {
		log.Printf("issues: skipping resolution (issue tracker not configured)")
		return
	}

	go func() {
		client := &http.Client{Timeout: 10 * time.Second}
		apiBase := strings.TrimRight(s.IssueTrackerURL, "/") + "/api/v1"

		for _, id := range issueIDs {
			// PATCH status to resolved.
			patchBody := `{"status":"resolved"}`
			req, err := http.NewRequest("PATCH", fmt.Sprintf("%s/issues/%s", apiBase, id), strings.NewReader(patchBody))
			if err != nil {
				log.Printf("issues: failed to create PATCH request for issue %s: %v", id, err)
				continue
			}
			req.Header.Set("Authorization", "Bearer "+s.IssueTrackerToken)
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				log.Printf("issues: failed to resolve issue %s: %v", id, err)
				continue
			}
			resp.Body.Close()

			if resp.StatusCode >= 300 {
				log.Printf("issues: resolve issue %s returned %d", id, resp.StatusCode)
				continue
			}

			// POST comment.
			comment := fmt.Sprintf(`{"author":"hydrarelease","text":"Resolved in %s %s"}`, project, version)
			req, err = http.NewRequest("POST", fmt.Sprintf("%s/issues/%s/comments", apiBase, id), strings.NewReader(comment))
			if err != nil {
				log.Printf("issues: failed to create comment request for issue %s: %v", id, err)
				continue
			}
			req.Header.Set("Authorization", "Bearer "+s.IssueTrackerToken)
			req.Header.Set("Content-Type", "application/json")

			resp, err = client.Do(req)
			if err != nil {
				log.Printf("issues: failed to comment on issue %s: %v", id, err)
				continue
			}
			resp.Body.Close()

			log.Printf("issues: resolved HYDRA-%s", id)
		}
	}()
}
