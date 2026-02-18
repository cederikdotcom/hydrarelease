package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/cederikdotcom/hydraapi"
	semver "github.com/cederikdotcom/hydrarelease/pkg/updater/version"
	"github.com/spf13/cobra"
)

type projectDef struct {
	Name      string
	HealthURL string // empty if no direct health endpoint
}

var projects = []projectDef{
	{Name: "hydrarelease", HealthURL: "https://releases.experiencenet.com"},
	{Name: "hydratransfer", HealthURL: "https://hydratransfer.experiencenet.com"},
	{Name: "hydrapipeline", HealthURL: "https://hydrapipeline.experiencenet.com"},
	{Name: "hydraexperiencelibrary", HealthURL: "https://experiencenet.com"},
	{Name: "hydracluster", HealthURL: "https://hydracluster.experiencenet.com"},
	{Name: "hydraguard", HealthURL: ""},
	{Name: "hydrabody", HealthURL: ""},
}

var (
	verifyServer  string
	verifyProject string
	verifyJSON    bool
	verifyTimeout int
)

type verifyResult struct {
	Project  string `json:"project"`
	Released string `json:"released"`
	Instance string `json:"instance"`
	Deployed string `json:"deployed"`
	Status   string `json:"status"`
}

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify deployed versions match released versions",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := &http.Client{Timeout: time.Duration(verifyTimeout) * time.Second}

		selected := projects
		if verifyProject != "" {
			selected = nil
			for _, p := range projects {
				if p.Name == verifyProject {
					selected = []projectDef{p}
					break
				}
			}
			if selected == nil {
				return fmt.Errorf("unknown project: %s", verifyProject)
			}
		}

		var (
			mu      sync.Mutex
			wg      sync.WaitGroup
			results []verifyResult
		)

		// Pre-fetch cluster nodes for hydrabody checks.
		var clusterNodes []clusterNode
		for _, p := range selected {
			if p.Name == "hydrabody" {
				nodes, err := fetchClusterNodes(client)
				if err != nil {
					mu.Lock()
					results = append(results, verifyResult{
						Project:  "hydrabody",
						Instance: "-",
						Deployed: "-",
						Status:   fmt.Sprintf("error: %s", err),
					})
					mu.Unlock()
				} else {
					clusterNodes = nodes
				}
				break
			}
		}

		for _, p := range selected {
			p := p
			wg.Add(1)
			go func() {
				defer wg.Done()

				released, err := fetchLatestVersion(client, verifyServer, p.Name)
				if err != nil {
					mu.Lock()
					results = append(results, verifyResult{
						Project:  p.Name,
						Released: "-",
						Instance: "-",
						Deployed: "-",
						Status:   fmt.Sprintf("error: %s", err),
					})
					mu.Unlock()
					return
				}

				if p.Name == "hydrabody" {
					if clusterNodes == nil {
						return // error already recorded above
					}
					for _, node := range clusterNodes {
						status := "ok"
						if node.BodyVersion == "" {
							status = "unknown"
						} else if semver.Compare(released, node.BodyVersion) != 0 {
							status = "outdated"
						}
						label := node.Name
						if node.District != "" {
							label = fmt.Sprintf("%s (%s)", node.Name, node.District)
						}
						mu.Lock()
						results = append(results, verifyResult{
							Project:  p.Name,
							Released: released,
							Instance: label,
							Deployed: node.BodyVersion,
							Status:   status,
						})
						mu.Unlock()
					}
					return
				}

				if p.HealthURL == "" {
					mu.Lock()
					results = append(results, verifyResult{
						Project:  p.Name,
						Released: released,
						Instance: "-",
						Deployed: "-",
						Status:   "no endpoint",
					})
					mu.Unlock()
					return
				}

				deployed, err := fetchHealthVersion(client, p.HealthURL)
				if err != nil {
					mu.Lock()
					results = append(results, verifyResult{
						Project:  p.Name,
						Released: released,
						Instance: hostFromURL(p.HealthURL),
						Deployed: "-",
						Status:   fmt.Sprintf("error: %s", err),
					})
					mu.Unlock()
					return
				}

				status := "ok"
				if semver.Compare(released, deployed) != 0 {
					status = "outdated"
				}

				mu.Lock()
				results = append(results, verifyResult{
					Project:  p.Name,
					Released: released,
					Instance: hostFromURL(p.HealthURL),
					Deployed: deployed,
					Status:   status,
				})
				mu.Unlock()
			}()
		}

		wg.Wait()

		// Sort results to match project order.
		ordered := make([]verifyResult, 0, len(results))
		for _, p := range selected {
			for _, r := range results {
				if r.Project == p.Name {
					ordered = append(ordered, r)
				}
			}
		}
		results = ordered

		if verifyJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(results)
		}

		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintf(tw, "PROJECT\tRELEASED\tINSTANCE\tDEPLOYED\tSTATUS\n")
		for _, r := range results {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				r.Project, r.Released, r.Instance, r.Deployed, r.Status)
		}
		if err := tw.Flush(); err != nil {
			return err
		}

		for _, r := range results {
			if r.Status != "ok" && r.Status != "no endpoint" {
				os.Exit(1)
			}
		}

		return nil
	},
}

type latestJSON struct {
	Version string `json:"version"`
}

func fetchLatestVersion(client *http.Client, server, project string) (string, error) {
	url := fmt.Sprintf("%s/%s/production/latest.json",
		strings.TrimRight(server, "/"), project)

	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching latest.json: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("latest.json returned %d", resp.StatusCode)
	}

	var latest latestJSON
	if err := json.NewDecoder(resp.Body).Decode(&latest); err != nil {
		return "", fmt.Errorf("parsing latest.json: %w", err)
	}

	if latest.Version == "" {
		return "", fmt.Errorf("empty version in latest.json")
	}

	return latest.Version, nil
}

func fetchHealthVersion(client *http.Client, healthURL string) (string, error) {
	url := strings.TrimRight(healthURL, "/") + "/api/v1/health"

	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching health: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("health returned %d", resp.StatusCode)
	}

	var health hydraapi.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return "", fmt.Errorf("parsing health: %w", err)
	}

	return health.Version, nil
}

type clusterNode struct {
	Name        string
	District    string
	BodyVersion string
}

func fetchClusterNodes(client *http.Client) ([]clusterNode, error) {
	url := "https://hydracluster.experiencenet.com/api/v1/nodes"

	token := resolveToken("")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching cluster nodes: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cluster nodes returned %d", resp.StatusCode)
	}

	var raw []struct {
		Name        string `json:"name"`
		District    string `json:"district"`
		BodyVersion string `json:"body_version"`
		Online      bool   `json:"online"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("parsing cluster nodes: %w", err)
	}

	var nodes []clusterNode
	for _, n := range raw {
		if !n.Online {
			continue
		}
		nodes = append(nodes, clusterNode{
			Name:        n.Name,
			District:    n.District,
			BodyVersion: n.BodyVersion,
		})
	}

	return nodes, nil
}

func hostFromURL(rawURL string) string {
	s := strings.TrimPrefix(rawURL, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimRight(s, "/")
	return s
}

func init() {
	verifyCmd.Flags().StringVar(&verifyServer, "server", "https://releases.experiencenet.com", "release server URL")
	verifyCmd.Flags().StringVar(&verifyProject, "project", "", "check a single project only")
	verifyCmd.Flags().BoolVar(&verifyJSON, "json", false, "output as JSON")
	verifyCmd.Flags().IntVar(&verifyTimeout, "timeout", 10, "HTTP timeout in seconds")

	rootCmd.AddCommand(verifyCmd)
}
