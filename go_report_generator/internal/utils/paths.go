package utils

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Stater defines an interface for checking file existence.
// This is used by FindFileInSourceDirs to allow for mock filesystems in tests.
type Stater interface {
	Stat(name string) (fs.FileInfo, error)
}

// DefaultStater implements the Stater interface using the real OS filesystem.
type DefaultStater struct{}

// Stat performs a real os.Stat call.
func (ds DefaultStater) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

// ProjectRoot returns the absolute path to the go_report_generator directory
// by searching for the go.mod file in parent directories
func ProjectRoot() string {
	// Start with the working directory
	dir, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("failed to get working directory: %v", err))
	}

	// Keep going up until we find go.mod
	for {
		// Check if go.mod exists in the current directory
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		// Get the parent directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// We've reached the root directory without finding go.mod
			panic("could not find project root (no go.mod file found in parent directories)")
		}
		dir = parent
	}
}

// FindFileInSourceDirs attempts to locate a file using a Stater interface.
func FindFileInSourceDirs(relativePath string, sourceDirs []string, stater Stater) (string, error) {
	if filepath.IsAbs(relativePath) {
		if _, err := stater.Stat(relativePath); err == nil {
			return relativePath, nil
		}
	}

	cleanedRelativePath := filepath.Clean(relativePath)

	for _, dir := range sourceDirs {
		absPath := filepath.Join(filepath.Clean(dir), cleanedRelativePath)
		if _, err := stater.Stat(absPath); err == nil {
			return absPath, nil
		}

		pathParts := strings.Split(cleanedRelativePath, string(os.PathSeparator))
		for i := 0; i < len(pathParts); i++ {
			suffixToTry := filepath.Join(pathParts[i:]...)
			potentialPath := filepath.Join(filepath.Clean(dir), suffixToTry)
			if _, err := stater.Stat(potentialPath); err == nil {
				return potentialPath, nil
			}
		}
	}
	return "", fmt.Errorf("file %q not found in any source directory (%v) or as absolute path", relativePath, sourceDirs)
}
