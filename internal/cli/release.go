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
	releaseServer  string
	releaseToken   string
	releaseProject string
	releaseEnv     string
	releaseBuild   int
	releaseVersion string
	releaseNotes   string
	releaseJSON    bool
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Manage releases",
}

var releasePromoteCmd = &cobra.Command{
	Use:   "promote",
	Short: "Promote a build to an environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		token := resolveToken(releaseToken)
		if token == "" {
			return fmt.Errorf("auth token required: use --token or HYDRARELEASE_AUTH_TOKEN env")
		}
		if releaseProject == "" {
			return fmt.Errorf("--project is required")
		}
		if releaseEnv == "" {
			return fmt.Errorf("--env is required")
		}
		if releaseBuild <= 0 {
			return fmt.Errorf("--build is required")
		}
		if releaseVersion == "" {
			return fmt.Errorf("--version is required")
		}

		body := map[string]any{
			"project":       releaseProject,
			"environment":   releaseEnv,
			"build_number":  releaseBuild,
			"version":       releaseVersion,
			"released_by":   "",
			"release_notes": releaseNotes,
		}

		resp, err := doJSON(releaseServer, token, "POST", "/api/v1/releases", body)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)

		if resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("promote failed (%d): %v", resp.StatusCode, result["error"])
		}

		if releaseJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}

		fmt.Printf("Promoted build #%d to %s/%s as %s\n", releaseBuild, releaseProject, releaseEnv, releaseVersion)
		return nil
	},
}

var releaseRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Roll back to previous build in an environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		token := resolveToken(releaseToken)
		if token == "" {
			return fmt.Errorf("auth token required: use --token or HYDRARELEASE_AUTH_TOKEN env")
		}
		if releaseProject == "" {
			return fmt.Errorf("--project is required")
		}
		if releaseEnv == "" {
			return fmt.Errorf("--env is required")
		}

		body := map[string]any{
			"project":        releaseProject,
			"environment":    releaseEnv,
			"rolled_back_by": "",
		}

		resp, err := doJSON(releaseServer, token, "POST", "/api/v1/releases/rollback", body)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("rollback failed (%d): %v", resp.StatusCode, result["error"])
		}

		if releaseJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}

		fmt.Printf("Rolled back %s/%s to build #%.0f\n", releaseProject, releaseEnv, result["build_number"])
		return nil
	},
}

var releaseListCmd = &cobra.Command{
	Use:   "list",
	Short: "List release history for a project",
	RunE: func(cmd *cobra.Command, args []string) error {
		if releaseProject == "" {
			return fmt.Errorf("--project is required")
		}

		url := fmt.Sprintf("%s/api/v1/releases?project=%s",
			strings.TrimRight(releaseServer, "/"), releaseProject)

		resp, err := http.Get(url)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		var releases []map[string]any
		json.NewDecoder(resp.Body).Decode(&releases)

		if releaseJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(releases)
		}

		if len(releases) == 0 {
			fmt.Println("No releases found.")
			return nil
		}

		tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintf(tw, "ENV\tBUILD\tVERSION\tRELEASED BY\tRELEASED AT\n")
		for _, r := range releases {
			fmt.Fprintf(tw, "%s\t#%.0f\t%s\t%s\t%s\n",
				r["environment"], r["build_number"], r["version"], r["released_by"], r["released_at"])
		}
		return tw.Flush()
	},
}

var releaseShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current release for a project and environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		if releaseProject == "" {
			return fmt.Errorf("--project is required")
		}
		if releaseEnv == "" {
			return fmt.Errorf("--env is required")
		}

		url := fmt.Sprintf("%s/api/v1/releases/%s/%s",
			strings.TrimRight(releaseServer, "/"), releaseProject, releaseEnv)

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
	releaseCmd.PersistentFlags().StringVar(&releaseServer, "server", "https://releases.experiencenet.com", "release server URL")
	releaseCmd.PersistentFlags().StringVar(&releaseToken, "token", "", "auth bearer token (or HYDRARELEASE_AUTH_TOKEN env)")
	releaseCmd.PersistentFlags().StringVar(&releaseProject, "project", "", "project name")
	releaseCmd.PersistentFlags().BoolVar(&releaseJSON, "json", false, "output as JSON")

	releasePromoteCmd.Flags().StringVar(&releaseEnv, "env", "", "environment (dev, staging, prod)")
	releasePromoteCmd.Flags().IntVar(&releaseBuild, "build", 0, "build number to promote")
	releasePromoteCmd.Flags().StringVar(&releaseVersion, "version", "", "version string")
	releasePromoteCmd.Flags().StringVar(&releaseNotes, "notes", "", "release notes")

	releaseRollbackCmd.Flags().StringVar(&releaseEnv, "env", "", "environment (dev, staging, prod)")

	releaseShowCmd.Flags().StringVar(&releaseEnv, "env", "", "environment (dev, staging, prod)")

	releaseCmd.AddCommand(releasePromoteCmd, releaseRollbackCmd, releaseListCmd, releaseShowCmd)
	rootCmd.AddCommand(releaseCmd)
}
