package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	publishServer  string
	publishToken   string
	publishProject string
	publishVersion string
	publishChannel string
)

var publishCmd = &cobra.Command{
	Use:   "publish [flags] <binary> [binary...]",
	Short: "Publish release binaries to the release server",
	Long:  "Upload one or more binaries and finalize the release (generates SHA256SUMS and latest.json).",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		token := publishToken
		if token == "" {
			token = os.Getenv("HYDRARELEASE_PUBLISH_TOKEN")
		}
		if token == "" {
			return fmt.Errorf("publish token required: use --token or HYDRARELEASE_PUBLISH_TOKEN env")
		}

		// Upload each binary
		for _, path := range args {
			binaryName := filepath.Base(path)
			fmt.Printf("Uploading %s...\n", binaryName)

			f, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("opening %s: %w", path, err)
			}

			url := fmt.Sprintf("%s/api/v1/publish/%s/%s/%s/%s",
				strings.TrimRight(publishServer, "/"),
				publishProject, publishChannel, publishVersion, binaryName)

			req, err := http.NewRequest("POST", url, f)
			if err != nil {
				f.Close()
				return fmt.Errorf("creating request: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/octet-stream")

			resp, err := http.DefaultClient.Do(req)
			f.Close()
			if err != nil {
				return fmt.Errorf("uploading %s: %w", binaryName, err)
			}

			var result map[string]string
			json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("upload %s failed (%d): %s", binaryName, resp.StatusCode, result["error"])
			}

			fmt.Printf("  OK: %s\n", binaryName)
		}

		// Finalize
		fmt.Println("Finalizing release...")
		url := fmt.Sprintf("%s/api/v1/publish/%s/%s/%s/finalize",
			strings.TrimRight(publishServer, "/"),
			publishProject, publishChannel, publishVersion)

		req, err := http.NewRequest("POST", url, nil)
		if err != nil {
			return fmt.Errorf("creating finalize request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("finalize request: %w", err)
		}

		var result map[string]string
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("finalize failed (%d): %s", resp.StatusCode, result["error"])
		}

		fmt.Printf("Published %s/%s/%s\n", publishProject, publishChannel, publishVersion)
		fmt.Printf("URL: %s/%s/%s/%s/\n",
			strings.TrimRight(publishServer, "/"),
			publishProject, publishChannel, publishVersion)
		return nil
	},
}

func init() {
	publishCmd.Flags().StringVar(&publishServer, "server", "https://releases.experiencenet.com", "release server URL")
	publishCmd.Flags().StringVar(&publishToken, "token", "", "publish bearer token (or HYDRARELEASE_PUBLISH_TOKEN env)")
	publishCmd.Flags().StringVar(&publishProject, "project", "", "project name (required)")
	publishCmd.Flags().StringVar(&publishVersion, "version", "", "version e.g. v1.2.0 (required)")
	publishCmd.Flags().StringVar(&publishChannel, "channel", "production", "release channel (production or staging)")

	publishCmd.MarkFlagRequired("project")
	publishCmd.MarkFlagRequired("version")

	rootCmd.AddCommand(publishCmd)
}
