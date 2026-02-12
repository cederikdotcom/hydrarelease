package cli

import (
	"fmt"
	"os"

	"github.com/cederikdotcom/hydrarelease/pkg/updater"
	"github.com/spf13/cobra"
)

// version is set at build time via main.go
var version = "dev"

// SetVersion sets the version string (called from main.go with ldflags value).
func SetVersion(v string) {
	version = v
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print HydraRelease version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("hydrarelease %s\n", version)
	},
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update HydraRelease to the latest release",
	RunE: func(cmd *cobra.Command, args []string) error {
		u := updater.NewProductionUpdater("hydrarelease", version)
		u.SetServiceName("hydrarelease")

		fmt.Println("Checking for updates...")
		info, err := u.CheckForUpdate()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to check for updates: %s\n", err)
			fmt.Fprintf(os.Stderr, "\nManual download: https://releases.experiencenet.com/hydrarelease/\n")
			return err
		}

		fmt.Printf("Current version: %s\n", info.CurrentVersion)
		fmt.Printf("Latest version:  %s\n", info.LatestVersion)

		if !info.Available {
			fmt.Println("\nAlready running the latest version!")
			return nil
		}

		fmt.Println("\nA new version is available!")

		fmt.Print("\nUpdate now? (yes/no): ")
		var response string
		fmt.Scanln(&response)

		if response != "yes" && response != "y" {
			fmt.Println("Update cancelled.")
			return nil
		}

		fmt.Println()
		return u.PerformUpdate()
	},
}

var checkUpdateCmd = &cobra.Command{
	Use:   "check-update",
	Short: "Check if a new version is available",
	RunE: func(cmd *cobra.Command, args []string) error {
		u := updater.NewProductionUpdater("hydrarelease", version)

		info, err := u.CheckForUpdate()
		if err != nil {
			return err
		}

		if info.Available {
			fmt.Printf("Update available: %s -> %s\n", info.CurrentVersion, info.LatestVersion)
			fmt.Println("Run 'hydrarelease update' to install.")
		} else {
			fmt.Printf("Already up to date: %s\n", info.CurrentVersion)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd, updateCmd, checkUpdateCmd)
}
