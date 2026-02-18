package store

import "os"

// atomicWriteFile writes data to a temporary file then renames it into place,
// ensuring the target file is never partially written.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
