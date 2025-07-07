// in: internal/filesystem/filesystem.go
package filesystem

import (
	"io/fs"
	"os"
	"path/filepath"
)

// This allows a mock to tell the code what environment it's simulating.
type Platformer interface {
	Platform() string
}

type Filesystem interface {
	Stat(name string) (fs.FileInfo, error)
	ReadDir(name string) ([]fs.DirEntry, error)
	Getwd() (string, error)
	Abs(path string) (string, error)
}

// DefaultFS implements the Filesystem interface using the standard `os` and `filepath` packages.
// It represents the real, underlying filesystem of the host operating system.
type DefaultFS struct{}

func (DefaultFS) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

func (DefaultFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}

func (DefaultFS) Getwd() (string, error) {
	return os.Getwd()
}

func (DefaultFS) Abs(path string) (string, error) {
	return filepath.Abs(path)
}