package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

// FindFileInSourceDirs attempts to locate a file, first checking if it's absolute,
// then searching through the provided source directories.
// This was previously in analyzer/codefile.go
func FindFileInSourceDirs(relativePath string, sourceDirs []string) (string, error) {
	if filepath.IsAbs(relativePath) {
		if _, err := os.Stat(relativePath); err == nil {
			return relativePath, nil
		}
		// If absolute path doesn't exist, still try source dirs in case it's a "rooted" path
		// from a different environment but shares a common suffix with files in sourceDirs.
	}

	cleanedRelativePath := filepath.Clean(relativePath)

	for _, dir := range sourceDirs {
		// Try joining directly
		absPath := filepath.Join(filepath.Clean(dir), cleanedRelativePath)
		if _, err := os.Stat(absPath); err == nil {
			return absPath, nil
		}

		// Handle cases where 'relativePath' might be like 'C:\Path\To\Project\File.cs'
		// and 'dir' is 'D:\SomeOther\Path\To\Project' but 'File.cs' exists in 'dir'.
		// This logic is similar to C# ReportGenerator's LocalFileReader.MapPath.
		// It tries to find the file by checking parts of the relativePath suffix against source dirs.
		pathParts := strings.Split(cleanedRelativePath, string(os.PathSeparator))
		for i := 0; i < len(pathParts); i++ {
			suffixToTry := filepath.Join(pathParts[i:]...)
			potentialPath := filepath.Join(filepath.Clean(dir), suffixToTry)
			if _, err := os.Stat(potentialPath); err == nil {
				return potentialPath, nil
			}
		}
	}
	return "", fmt.Errorf("file %q not found in any source directory (%v) or as absolute path", relativePath, sourceDirs)
}