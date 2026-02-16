//go:build windows

package updater

import (
	"fmt"
	"os/exec"
)

func restartService(name string) error {
	// Stop the scheduled task, then start it again.
	if output, err := exec.Command("schtasks", "/End", "/TN", name).CombinedOutput(); err != nil {
		return fmt.Errorf("stopping task: %w\nOutput: %s", err, string(output))
	}
	if output, err := exec.Command("schtasks", "/Run", "/TN", name).CombinedOutput(); err != nil {
		return fmt.Errorf("starting task: %w\nOutput: %s", err, string(output))
	}
	return nil
}
