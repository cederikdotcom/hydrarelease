package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cederikdotcom/hydrarelease/pkg/updater/version"
)

const releaseBaseURL = "https://releases.experiencenet.com"

type latestManifest struct {
	Version string `json:"version"`
}

// Channel represents a release channel.
type Channel string

const (
	Production Channel = "production"
	Staging    Channel = "staging"
)

type UpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
	Available      bool
}

type Updater struct {
	project        string
	currentVersion string
	serviceName    string
	channel        Channel
}

// SetServiceName sets the systemd service to restart after a successful update.
func (u *Updater) SetServiceName(name string) {
	u.serviceName = name
}

// NewProductionUpdater creates an updater that tracks the production release channel.
func NewProductionUpdater(project, currentVersion string) *Updater {
	return &Updater{project: project, currentVersion: currentVersion, channel: Production}
}

// NewStagingUpdater creates an updater that tracks the staging release channel.
func NewStagingUpdater(project, currentVersion string) *Updater {
	return &Updater{project: project, currentVersion: currentVersion, channel: Staging}
}

func (u *Updater) channelURL() string {
	return releaseBaseURL + "/" + u.project + "/" + string(u.channel)
}

func (u *Updater) CheckForUpdate() (*UpdateInfo, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Get(u.channelURL() + "/latest.json")
	if err != nil {
		return nil, fmt.Errorf("checking for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release server returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var manifest latestManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	latestVersion := strings.TrimPrefix(manifest.Version, "v")
	currentVersion := strings.TrimPrefix(u.currentVersion, "v")

	return &UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
		Available:      version.Compare(latestVersion, currentVersion) > 0,
	}, nil
}

func (u *Updater) PerformUpdate() error {
	updateInfo, err := u.CheckForUpdate()
	if err != nil {
		return fmt.Errorf("checking for updates: %w", err)
	}

	if !updateInfo.Available {
		fmt.Println("Already on the latest version!")
		return nil
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolving symlink: %w", err)
	}

	binaryName := fmt.Sprintf("%s-%s-%s", u.project, runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	ver := "v" + updateInfo.LatestVersion
	downloadURL := fmt.Sprintf("%s/%s/%s", u.channelURL(), ver, binaryName)

	fmt.Printf("Downloading %s %s for %s/%s...\n", u.project, ver, runtime.GOOS, runtime.GOARCH)

	tmpFile := filepath.Join(os.TempDir(), u.project+"-update")
	if runtime.GOOS == "windows" {
		tmpFile += ".exe"
	}
	if err := downloadFile(downloadURL, tmpFile); err != nil {
		return fmt.Errorf("downloading binary: %w\n\nManual download: %s/%s/", err, u.channelURL(), ver)
	}

	info, err := os.Stat(tmpFile)
	if err != nil || info.Size() == 0 {
		os.Remove(tmpFile)
		return fmt.Errorf("downloaded file is empty or missing")
	}

	// Verify checksum
	fmt.Println("Verifying checksum...")
	if err := u.verifyChecksum(tmpFile, binaryName, ver); err != nil {
		os.Remove(tmpFile)
		return err
	}

	if err := os.Chmod(tmpFile, 0755); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("making executable: %w", err)
	}

	// Verify the binary runs
	verifyCmd := exec.Command(tmpFile, "version")
	if output, err := verifyCmd.CombinedOutput(); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("binary verification failed: %w\nOutput: %s", err, string(output))
	}

	// Backup current binary
	backupPath := execPath + ".backup"
	fmt.Printf("Backing up current version to %s\n", backupPath)
	os.Remove(backupPath)

	if err := os.Rename(execPath, backupPath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("backing up current version: %w", err)
	}

	// Atomic replace
	fmt.Printf("Installing update to %s\n", execPath)
	if err := os.Rename(tmpFile, execPath); err != nil {
		os.Rename(backupPath, execPath) // rollback
		os.Remove(tmpFile)
		return fmt.Errorf("installing update: %w", err)
	}

	os.Remove(tmpFile)
	fmt.Println("\nUpdate completed successfully!")
	fmt.Printf("Backup saved at: %s\n", backupPath)

	if u.serviceName != "" {
		fmt.Printf("\nRestarting service %s...\n", u.serviceName)
		if err := restartService(u.serviceName); err != nil {
			fmt.Printf("Warning: failed to restart %s: %s\n", u.serviceName, err)
		} else {
			fmt.Printf("Service %s restarted.\n", u.serviceName)
		}
	} else {
		fmt.Printf("Run '%s version' to verify.\n", u.project)
	}

	return nil
}

// StartAutoCheck runs a background goroutine that periodically checks for
// updates. If autoApply is true, updates are downloaded and installed
// automatically, and the systemd service is restarted. If false, it only logs
// that an update is available.
func (u *Updater) StartAutoCheck(interval time.Duration, autoApply bool) {
	go func() {
		for {
			time.Sleep(interval)

			info, err := u.CheckForUpdate()
			if err != nil {
				log.Printf("[updater] check failed: %v", err)
				continue
			}

			if !info.Available {
				continue
			}

			if !autoApply {
				log.Printf("[updater] update available: %s -> %s (run '%s update' to install)", info.CurrentVersion, info.LatestVersion, u.project)
				continue
			}

			log.Printf("[updater] updating %s -> %s", info.CurrentVersion, info.LatestVersion)
			if err := u.PerformUpdate(); err != nil {
				log.Printf("[updater] auto-update failed: %v", err)
			}
		}
	}()
}

func (u *Updater) verifyChecksum(filePath, binaryName, ver string) error {
	sumsURL := fmt.Sprintf("%s/%s/SHA256SUMS", u.channelURL(), ver)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(sumsURL)
	if err != nil {
		return fmt.Errorf("fetching SHA256SUMS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SHA256SUMS not found (status %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading SHA256SUMS: %w", err)
	}

	// Parse "hash  filename" lines
	var expected string
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == binaryName {
			expected = parts[0]
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("no checksum found for %s in SHA256SUMS", binaryName)
	}

	actual, err := hashFile(filePath)
	if err != nil {
		return fmt.Errorf("hashing downloaded file: %w", err)
	}

	if actual != expected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}

	return nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func downloadFile(url, destPath string) error {
	client := &http.Client{Timeout: 5 * time.Minute}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}
