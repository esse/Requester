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
	if !strings.HasPrefix(absPath, absSnapshotDir+string(filepath.Separator)) && absPath != absSnapshotDir {
		return ErrPathTraversal
	}
	
	return nil
}

// SanitizePath removes dangerous characters and sequences from a path.
func SanitizePath(path string) string {
	// Clean the path to remove any .. sequences
	cleaned := filepath.Clean(path)
	
	// Remove any leading ../
	for strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "..\\") {
		cleaned = cleaned[3:]
	}
	
	return cleaned
}
