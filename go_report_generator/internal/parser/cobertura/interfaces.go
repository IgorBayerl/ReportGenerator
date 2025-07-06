package cobertura

// FileReader defines an interface for reading source files. This abstraction
// is crucial for dependency injection, allowing the parsing logic to be
// unit-tested without hitting the actual file system. A production implementation
// will read from disk, while a test implementation can use an in-memory map.
type FileReader interface {
	// ReadFile reads all lines from a file at the given path.
	ReadFile(path string) ([]string, error)
	// CountLines counts the number of lines in a file at the given path.
	CountLines(path string) (int, error)
}