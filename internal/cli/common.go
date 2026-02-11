package cli

import (
	"fmt"

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

func init() {
	rootCmd.AddCommand(versionCmd)
}
