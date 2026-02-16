//go:build !windows

package updater

import (
	"fmt"
	"os/exec"
)

func restartService(name string) error {
	output, err := exec.Command("systemctl", "restart", name).CombinedOutput()
	if err != nil && len(output) > 0 {
		return fmt.Errorf("%w\nOutput: %s", err, string(output))
	}
	return err
}
