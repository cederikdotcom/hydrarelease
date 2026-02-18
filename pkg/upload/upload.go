package upload

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// HashFile returns the hex-encoded SHA256 hash of the file at path.
func HashFile(path string) (string, error) {
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

// WriteChunk writes data from r to filePath at the given byte offset.
// It returns the number of bytes written.
func WriteChunk(filePath string, offset int64, r io.Reader) (int64, error) {
	f, err := os.OpenFile(filePath, os.O_WRONLY, 0644)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return 0, err
	}

	return io.Copy(f, r)
}

// ParseContentRange parses a Content-Range header of the form "bytes start-end/total".
func ParseContentRange(header string) (start, end, total int64, err error) {
	_, err = fmt.Sscanf(header, "bytes %d-%d/%d", &start, &end, &total)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid Content-Range format: %w", err)
	}
	return start, end, total, nil
}

// AtomicWrite writes data from r to destPath by first writing to a temporary file
// in the same directory, then renaming it into place.
func AtomicWrite(destPath string, r io.Reader, perm os.FileMode) error {
	dir := filepath.Dir(destPath)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Chmod(tmpPath, perm); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("setting permissions: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}
