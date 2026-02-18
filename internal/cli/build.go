package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	buildServer     string
	buildToken      string
	buildProject    string
	buildUploadedBy string
	buildNumber     int
	buildJSON       bool
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Manage builds",
}

var buildSubmitCmd = &cobra.Command{
	Use:   "submit [flags] <file> [file...]",
	Short: "Submit a new build",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		token := resolveToken(buildToken)
		if token == "" {
			return fmt.Errorf("auth token required: use --token or HYDRARELEASE_AUTH_TOKEN env")
		}
		if buildProject == "" {
			return fmt.Errorf("--project is required")
		}

		// Build the file list for the request.
		type fileEntry struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
			Size   int64  `json:"size"`
		}
		var files []fileEntry
		for _, path := range args {
			info, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("stat %s: %w", path, err)
			}
			files = append(files, fileEntry{
				Path: info.Name(),
				Size: info.Size(),
			})
		}

		body := map[string]any{
			"project":     buildProject,
			"uploaded_by": buildUploadedBy,
			"files":       files,
		}

		resp, err := doJSON(buildServer, token, "POST", "/api/v1/builds", body)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)

		if resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("submit failed (%d): %v", resp.StatusCode, result["error"])
		}

		if buildJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}

		fmt.Printf("Build %s/#%.0f submitted\n", buildProject, result["build_number"])
		return nil
	},
}

var buildListCmd = &cobra.Command{
	Use:   "list",
	Short: "List builds for a project",
	RunE: func(cmd *cobra.Command, args []string) error {
		if buildProject == "" {
			return fmt.Errorf("--project is required")
		}

		url := fmt.Sprintf("%s/api/v1/builds?project=%s",
			strings.TrimRight(buildServer, "/"), buildProject)

		resp, err := http.Get(url)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		var builds []map[string]any
		json.NewDecoder(resp.Body).Decode(&builds)

		if buildJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(builds)
		}

		if len(builds) == 0 {
			fmt.Println("No builds found.")
			return nil
		}

		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintf(tw, "BUILD\tUPLOADED BY\tFILES\tUPLOADED AT\n")
		for _, b := range builds {
			fmt.Fprintf(tw, "#%.0f\t%s\t%.0f\t%s\n",
				b["build_number"], b["uploaded_by"], b["file_count"], b["uploaded_at"])
		}
		return tw.Flush()
	},
}

var buildShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show build details",
	RunE: func(cmd *cobra.Command, args []string) error {
		if buildProject == "" {
			return fmt.Errorf("--project is required")
		}
		if buildNumber <= 0 {
			return fmt.Errorf("--build is required")
		}

		url := fmt.Sprintf("%s/api/v1/builds/%s/%d",
			strings.TrimRight(buildServer, "/"), buildProject, buildNumber)

		resp, err := http.Get(url)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("not found: %v", result["error"])
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	},
}

func init() {
	// Common flags for build commands.
	buildCmd.PersistentFlags().StringVar(&buildServer, "server", "https://releases.experiencenet.com", "release server URL")
	buildCmd.PersistentFlags().StringVar(&buildToken, "token", "", "auth bearer token (or HYDRARELEASE_AUTH_TOKEN env)")
	buildCmd.PersistentFlags().StringVar(&buildProject, "project", "", "project name")
	buildCmd.PersistentFlags().BoolVar(&buildJSON, "json", false, "output as JSON")

	buildSubmitCmd.Flags().StringVar(&buildUploadedBy, "uploaded-by", "", "who uploaded the build")
	buildShowCmd.Flags().IntVar(&buildNumber, "build", 0, "build number")

	buildCmd.AddCommand(buildSubmitCmd, buildListCmd, buildShowCmd)
	rootCmd.AddCommand(buildCmd)
}

func resolveToken(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if t := os.Getenv("HYDRARELEASE_AUTH_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("HYDRARELEASE_PUBLISH_TOKEN")
}

func doJSON(server, token, method, path string, body any) (*http.Response, error) {
	url := strings.TrimRight(server, "/") + path

	var r *strings.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request: %w", err)
		}
		r = strings.NewReader(string(data))
	} else {
		r = strings.NewReader("")
	}

	req, err := http.NewRequest(method, url, r)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	return http.DefaultClient.Do(req)
}
