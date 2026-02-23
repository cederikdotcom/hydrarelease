//go:build windows

package updater

import (
	"fmt"
	"os/exec"
)

func restartService(name string) error {
	// With StopExisting policy, calling /Run while the task is running
	// makes the Task Scheduler stop the existing instance and start a new one.
	if output, err := exec.Command("schtasks", "/Run", "/TN", name).CombinedOutput(); err != nil {
		return fmt.Errorf("restarting task: %w\nOutput: %s", err, string(output))
	}
	return nil
}
