//go:build windows

package updater

import (
	"fmt"
	"os/exec"
)

func restartService(name string) error {
	// Spawn a detached cmd.exe that stops us, waits, then starts the new binary.
	// We can't do /End then /Run sequentially because /End kills our own process
	// before /Run executes. The detached cmd.exe survives our termination.
	script := fmt.Sprintf(
		`schtasks /End /TN "%s" & timeout /t 3 /nobreak >nul & schtasks /Run /TN "%s"`,
		name, name,
	)
	cmd := exec.Command("cmd", "/c", script)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawning restart script: %w", err)
	}
	// Don't wait — let the detached process handle it while we get killed.
	return nil
}
