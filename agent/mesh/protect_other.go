//go:build !windows

package mesh

import "os"

// Non-Windows builds are development/test harnesses. Files are mode 0600.
func protect(data []byte) ([]byte, error)   { return data, nil }
func unprotect(data []byte) ([]byte, error) { return data, nil }
func restrictDirectory(dir string) error    { return os.Chmod(dir, 0700) }
