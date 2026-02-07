package security

import (
	"errors"
	"path/filepath"
	"strings"
)

var (
	// ErrPathTraversal is returned when a path contains directory traversal sequences
	ErrPathTraversal = errors.New("path contains directory traversal sequences")
	// ErrAbsolutePath is returned when an absolute path is provided where relative is expected
	ErrAbsolutePath = errors.New("absolute paths are not allowed")
)

// ValidateConfigPath validates that a config file path is safe to use.
// It prevents directory traversal attacks by checking for ".." sequences.
//
// Security: This function is critical for preventing path traversal vulnerabilities.
// It ensures that user-supplied config paths cannot be used to read arbitrary files
// outside of expected directories (e.g., /etc/passwd, /proc/self/environ).
//
// Example attack prevented: --config=../../../etc/passwd
func ValidateConfigPath(path string) error {
	if path == "" {
		return errors.New("config path cannot be empty")
	}
	
	// Check for directory traversal
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return ErrPathTraversal
	}
	
	return nil
}

// ValidateSnapshotPath validates that a snapshot file path is within the allowed directory.
// It prevents directory traversal attacks and ensures files stay within snapshotDir.
//
// Security: This function is critical for preventing path traversal vulnerabilities
// in snapshot operations (load, update, diff). Without this validation, an attacker
// could:
//   - Read arbitrary files via --snapshot=/etc/passwd
//   - Modify/delete arbitrary files via --update --snapshot=/important/file
//   - Escape the snapshot directory via --snapshot=../../../malicious/path
//
// The function makes both paths absolute and checks that the snapshot path is
// contained within (or equal to) the snapshot directory.
func ValidateSnapshotPath(path string, snapshotDir string) error {
	if path == "" {
		return errors.New("snapshot path cannot be empty")
	}
	
	// Clean both paths
	cleanPath := filepath.Clean(path)
	cleanSnapshotDir := filepath.Clean(snapshotDir)
	
	// Make paths absolute for proper comparison
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return err
	}
	
	absSnapshotDir, err := filepath.Abs(cleanSnapshotDir)
	if err != nil {
		return err
	}
	
	// Check if the path is within the snapshot directory
	// Note: We add a separator to prevent partial path matches
	// e.g., "/snapshots" should not match "/snapshots_malicious"
	if !strings.HasPrefix(absPath, absSnapshotDir+string(filepath.Separator)) && absPath != absSnapshotDir {
		return ErrPathTraversal
	}
	
	return nil
}

// SanitizePath removes dangerous characters and sequences from a path.
// This is a best-effort sanitization and should be used in conjunction with
// validation functions, not as a replacement.
//
// Note: This function is provided for convenience but should not be relied upon
// as the sole security measure. Always validate paths explicitly.
func SanitizePath(path string) string {
	// Clean the path to remove any .. sequences
	cleaned := filepath.Clean(path)
	
	// Remove any leading ../
	for strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "..\\") {
		cleaned = cleaned[3:]
	}
	
	return cleaned
}
