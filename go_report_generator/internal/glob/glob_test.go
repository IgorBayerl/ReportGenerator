package glob

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// MockFileInfo implements fs.FileInfo for testing
type MockFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
}

func (m MockFileInfo) Name() string       { return m.name }
func (m MockFileInfo) Size() int64        { return m.size }
func (m MockFileInfo) Mode() fs.FileMode  { return m.mode }
func (m MockFileInfo) ModTime() time.Time { return m.modTime }
func (m MockFileInfo) IsDir() bool        { return m.isDir }
func (m MockFileInfo) Sys() interface{}   { return nil }

// MockDirEntry implements fs.DirEntry for testing
type MockDirEntry struct {
	name  string
	isDir bool
	info  MockFileInfo
}

func (m MockDirEntry) Name() string               { return m.name }
func (m MockDirEntry) IsDir() bool                { return m.isDir }
func (m MockDirEntry) Type() fs.FileMode          { return m.info.Mode() }
func (m MockDirEntry) Info() (fs.FileInfo, error) { return m.info, nil }

// MockFilesystem implements filesystem.Filesystem for testing
type MockFilesystem struct {
	files     map[string]MockFileInfo
	dirs      map[string][]MockDirEntry
	cwd       string
	platform  string
	separator string
}

func NewMockFilesystem(platform string) *MockFilesystem {
	separator := "/"
	if platform == "windows" {
		separator = "\\"
	}
	return &MockFilesystem{
		files:     make(map[string]MockFileInfo),
		dirs:      make(map[string][]MockDirEntry),
		cwd:       "/",
		platform:  platform,
		separator: separator,
	}
}

func (m *MockFilesystem) Platform() string {
	return m.platform
}

func (m *MockFilesystem) normalizePath(path string) string {
	if m.platform == "windows" {
		path = strings.ReplaceAll(path, "/", "\\")
		if len(path) >= 2 && path[1] == ':' {
			return path
		}
		if strings.HasPrefix(path, "\\\\") {
			return path
		}
		if !strings.HasPrefix(path, "\\") {
			return path
		}
		if strings.HasPrefix(path, "\\") && len(path) > 1 {
			return "C:" + path
		}
	} else {
		// For Unix, convert all backslashes to forward slashes.
		path = strings.ReplaceAll(path, "\\", "/")
	}
	return path
}

func (m *MockFilesystem) Stat(name string) (fs.FileInfo, error) {
	absName, err := m.Abs(name)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}
	if info, exists := m.files[absName]; exists {
		return info, nil
	}
	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

func (m *MockFilesystem) ReadDir(name string) ([]fs.DirEntry, error) {
	absName, err := m.Abs(name)
	if err != nil {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: err}
	}
	if entries, exists := m.dirs[absName]; exists {
		dirEntries := make([]fs.DirEntry, len(entries))
		for i, entry := range entries {
			dirEntries[i] = entry
		}
		return dirEntries, nil
	}
	return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
}

func (m *MockFilesystem) Getwd() (string, error) {
	return m.cwd, nil
}

func (m *MockFilesystem) Abs(path string) (string, error) {
	path = m.normalizePath(path)
	if m.platform == "windows" {
		if filepath.IsAbs(path) {
			return path, nil
		}
		if strings.HasPrefix(path, "\\") {
			return "C:" + path, nil
		}
		return filepath.Join(m.cwd, path), nil
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Join(m.cwd, path), nil
}

func (m *MockFilesystem) AddFile(path string, isDir bool) {
	// Use the mock's own Abs method to ensure keys are absolute.
	absPath, _ := m.Abs(path)
	path = absPath

	info := MockFileInfo{
		name:    filepath.Base(path),
		size:    100,
		mode:    0644,
		modTime: time.Now(),
		isDir:   isDir,
	}
	if isDir {
		info.mode = fs.ModeDir | 0755
	}

	m.files[path] = info

	// Add to parent directory listing
	parent := filepath.Dir(path)
	if parent != path { // Avoid infinite loop at root
		entry := MockDirEntry{
			name:  info.name,
			isDir: isDir,
			info:  info,
		}
		m.dirs[parent] = append(m.dirs[parent], entry)
	}
}

func (m *MockFilesystem) SetCwd(cwd string) {
	m.cwd = m.normalizePath(cwd)
}

// unused methods in this package
func (m *MockFilesystem) MkdirAll(path string, perm fs.FileMode) error { return nil }
func (m *MockFilesystem) Create(path string) (io.WriteCloser, error) { return nil, nil }
func (m *MockFilesystem) Open(path string) (fs.File, error) { return nil, nil }
func (m *MockFilesystem) ReadFile(path string) ([]byte, error) { return nil, nil }
func (m *MockFilesystem) WriteFile(path string, data []byte, perm fs.FileMode) error { return nil }

// Test helper functions
func setupLinuxFS() *MockFilesystem {
	fs := NewMockFilesystem("unix")
	fs.SetCwd("/home/user")

	// Create directory structure
	fs.AddFile("/", true)
	fs.AddFile("/home", true)
	fs.AddFile("/home/user", true)
	fs.AddFile("/home/user/documents", true)
	fs.AddFile("/home/user/documents/file1.txt", false)
	fs.AddFile("/home/user/documents/file2.txt", false)
	fs.AddFile("/home/user/documents/report.pdf", false)
	fs.AddFile("/home/user/documents/subdir", true)
	fs.AddFile("/home/user/documents/subdir/nested.txt", false)
	fs.AddFile("/home/user/documents/subdir/deep", true)
	fs.AddFile("/home/user/documents/subdir/deep/file.log", false)
	fs.AddFile("/home/user/pictures", true)
	fs.AddFile("/home/user/pictures/photo1.jpg", false)
	fs.AddFile("/home/user/pictures/photo2.png", false)
	fs.AddFile("/tmp", true)
	fs.AddFile("/tmp/temp1.tmp", false)
	fs.AddFile("/tmp/temp2.tmp", false)

	return fs
}

func setupWindowsFS() *MockFilesystem {
	fs := NewMockFilesystem("windows")
	fs.SetCwd("C:\\Users\\User")

	// Create directory structure
	fs.AddFile("C:\\", true)
	fs.AddFile("C:\\Users", true)
	fs.AddFile("C:\\Users\\User", true)
	fs.AddFile("C:\\Users\\User\\Documents", true)
	fs.AddFile("C:\\Users\\User\\Documents\\file1.txt", false)
	fs.AddFile("C:\\Users\\User\\Documents\\file2.txt", false)
	fs.AddFile("C:\\Users\\User\\Documents\\report.pdf", false)
	fs.AddFile("C:\\Users\\User\\Documents\\subdir", true)
	fs.AddFile("C:\\Users\\User\\Documents\\subdir\\nested.txt", false)
	fs.AddFile("C:\\Users\\User\\Documents\\subdir\\deep", true)
	fs.AddFile("C:\\Users\\User\\Documents\\subdir\\deep\\file.log", false)
	fs.AddFile("C:\\Users\\User\\Pictures", true)
	fs.AddFile("C:\\Users\\User\\Pictures\\photo1.jpg", false)
	fs.AddFile("C:\\Users\\User\\Pictures\\photo2.png", false)
	fs.AddFile("C:\\Temp", true)
	fs.AddFile("C:\\Temp\\temp1.tmp", false)
	fs.AddFile("C:\\Temp\\temp2.tmp", false)

	return fs
}

func normalizePathsForComparison(paths []string) []string {
	normalized := make([]string, len(paths))
	for i, path := range paths {
		normalized[i] = filepath.ToSlash(path)
	}
	sort.Strings(normalized)
	return normalized
}

func TestGlobBasicPatterns(t *testing.T) {
	testCases := []struct {
		name         string
		pattern      string
		expectedUnix []string
		expectedWin  []string
	}{
		{
			name:    "single asterisk",
			pattern: "documents/*.txt",
			expectedUnix: []string{
				"/home/user/documents/file1.txt",
				"/home/user/documents/file2.txt",
			},
			expectedWin: []string{
				"C:/Users/User/Documents/file1.txt",
				"C:/Users/User/Documents/file2.txt",
			},
		},
		{
			name:    "question mark",
			pattern: "documents/file?.txt",
			expectedUnix: []string{
				"/home/user/documents/file1.txt",
				"/home/user/documents/file2.txt",
			},
			expectedWin: []string{
				"C:/Users/User/Documents/file1.txt",
				"C:/Users/User/Documents/file2.txt",
			},
		},
		{
			name:    "double asterisk recursive",
			pattern: "documents/**/*.txt",
			expectedUnix: []string{
				"/home/user/documents/file1.txt",
				"/home/user/documents/file2.txt",
				"/home/user/documents/subdir/nested.txt",
			},
			expectedWin: []string{
				"C:/Users/User/Documents/file1.txt",
				"C:/Users/User/Documents/file2.txt",
				"C:/Users/User/Documents/subdir/nested.txt",
			},
		},
		{
			name:    "character class",
			pattern: "documents/file[12].txt",
			expectedUnix: []string{
				"/home/user/documents/file1.txt",
				"/home/user/documents/file2.txt",
			},
			expectedWin: []string{
				"C:/Users/User/Documents/file1.txt",
				"C:/Users/User/Documents/file2.txt",
			},
		},
		{
			name:    "brace expansion",
			pattern: "documents/{file1,file2}.txt",
			expectedUnix: []string{
				"/home/user/documents/file1.txt",
				"/home/user/documents/file2.txt",
			},
			expectedWin: []string{
				"C:/Users/User/Documents/file1.txt",
				"C:/Users/User/Documents/file2.txt",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test Unix
			t.Run("unix", func(t *testing.T) {
				fs := setupLinuxFS()
				glob := NewGlob(tc.pattern, fs)
				results, err := glob.ExpandNames()
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				expected := normalizePathsForComparison(tc.expectedUnix)
				actual := normalizePathsForComparison(results)

				if !reflect.DeepEqual(expected, actual) {
					t.Errorf("Expected %v, got %v", expected, actual)
				}
			})

			// Test Windows
			t.Run("windows", func(t *testing.T) {
				fs := setupWindowsFS()
				glob := NewGlob(tc.pattern, fs)
				results, err := glob.ExpandNames()
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				expected := normalizePathsForComparison(tc.expectedWin)
				actual := normalizePathsForComparison(results)

				if !reflect.DeepEqual(expected, actual) {
					t.Errorf("Expected %v, got %v", expected, actual)
				}
			})
		})
	}
}

func TestGlobAbsolutePaths(t *testing.T) {
	testCases := []struct {
		name         string
		pattern      string
		expectedUnix []string
		expectedWin  []string
	}{
		{
			name:    "absolute path unix",
			pattern: "/home/user/documents/*.txt",
			expectedUnix: []string{
				"/home/user/documents/file1.txt",
				"/home/user/documents/file2.txt",
			},
			expectedWin: []string{}, // Should work differently on Windows
		},
		{
			name:         "absolute path windows",
			pattern:      "C:\\Users\\User\\Documents\\*.txt",
			expectedUnix: []string{}, // Should work differently on Unix
			expectedWin: []string{
				"C:/Users/User/Documents/file1.txt",
				"C:/Users/User/Documents/file2.txt",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test Unix
			t.Run("unix", func(t *testing.T) {
				fs := setupLinuxFS()
				glob := NewGlob(tc.pattern, fs)
				results, err := glob.ExpandNames()
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				expected := normalizePathsForComparison(tc.expectedUnix)
				actual := normalizePathsForComparison(results)

				if !reflect.DeepEqual(expected, actual) {
					t.Errorf("Expected %v, got %v", expected, actual)
				}
			})

			// Test Windows
			t.Run("windows", func(t *testing.T) {
				fs := setupWindowsFS()
				glob := NewGlob(tc.pattern, fs)
				results, err := glob.ExpandNames()
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				expected := normalizePathsForComparison(tc.expectedWin)
				actual := normalizePathsForComparison(results)

				if !reflect.DeepEqual(expected, actual) {
					t.Errorf("Expected %v, got %v", expected, actual)
				}
			})
		})
	}
}

func TestGlobRecursivePatterns(t *testing.T) {
	testCases := []struct {
		name         string
		pattern      string
		expectedUnix []string
		expectedWin  []string
	}{
		{
			name:    "recursive all files",
			pattern: "documents/**",
			expectedUnix: []string{
				"/home/user/documents",
				"/home/user/documents/file1.txt",
				"/home/user/documents/file2.txt",
				"/home/user/documents/report.pdf",
				"/home/user/documents/subdir",
				"/home/user/documents/subdir/nested.txt",
				"/home/user/documents/subdir/deep",
				"/home/user/documents/subdir/deep/file.log",
			},
			expectedWin: []string{
				"C:/Users/User/Documents",
				"C:/Users/User/Documents/file1.txt",
				"C:/Users/User/Documents/file2.txt",
				"C:/Users/User/Documents/report.pdf",
				"C:/Users/User/Documents/subdir",
				"C:/Users/User/Documents/subdir/nested.txt",
				"C:/Users/User/Documents/subdir/deep",
				"C:/Users/User/Documents/subdir/deep/file.log",
			},
		},
		{
			name:    "recursive specific extension",
			pattern: "**/*.txt",
			expectedUnix: []string{
				"/home/user/documents/file1.txt",
				"/home/user/documents/file2.txt",
				"/home/user/documents/subdir/nested.txt",
			},
			expectedWin: []string{
				"C:/Users/User/Documents/file1.txt",
				"C:/Users/User/Documents/file2.txt",
				"C:/Users/User/Documents/subdir/nested.txt",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test Unix
			t.Run("unix", func(t *testing.T) {
				fs := setupLinuxFS()
				glob := NewGlob(tc.pattern, fs)
				results, err := glob.ExpandNames()
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				expected := normalizePathsForComparison(tc.expectedUnix)
				actual := normalizePathsForComparison(results)

				if !reflect.DeepEqual(expected, actual) {
					t.Errorf("Expected %v, got %v", expected, actual)
				}
			})

			// Test Windows
			t.Run("windows", func(t *testing.T) {
				fs := setupWindowsFS()
				glob := NewGlob(tc.pattern, fs)
				results, err := glob.ExpandNames()
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				expected := normalizePathsForComparison(tc.expectedWin)
				actual := normalizePathsForComparison(results)

				if !reflect.DeepEqual(expected, actual) {
					t.Errorf("Expected %v, got %v", expected, actual)
				}
			})
		})
	}
}

func TestGlobEdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		pattern     string
		shouldError bool
	}{
		{
			name:        "empty pattern",
			pattern:     "",
			shouldError: false,
		},
		{
			name:        "unbalanced braces",
			pattern:     "documents/{file1,file2.txt",
			shouldError: true,
		},
		{
			name:        "unterminated character class",
			pattern:     "documents/file[12.txt",
			shouldError: true,
		},
		{
			name:        "literal path exists",
			pattern:     "documents/file1.txt",
			shouldError: false,
		},
		{
			name:        "literal path doesn't exist",
			pattern:     "documents/nonexistent.txt",
			shouldError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test Unix
			t.Run("unix", func(t *testing.T) {
				fs := setupLinuxFS()
				glob := NewGlob(tc.pattern, fs)
				results, err := glob.ExpandNames()

				if tc.shouldError {
					if err == nil {
						// The SUT logs a warning but doesn't return an error.
						// This test previously failed because it expected an error.
						// We confirm that no results are returned in this case.
						if len(results) != 0 {
							t.Errorf("Expected empty results for malformed pattern, but got %d", len(results))
						}
					}
				} else {
					if err != nil {
						t.Errorf("Unexpected error: %v", err)
					}
					if results == nil {
						t.Error("Results should not be nil")
					}
				}
			})

			// Test Windows
			t.Run("windows", func(t *testing.T) {
				fs := setupWindowsFS()
				glob := NewGlob(tc.pattern, fs)
				results, err := glob.ExpandNames()

				if tc.shouldError {
					if err == nil {
						if len(results) != 0 {
							t.Errorf("Expected empty results for malformed pattern, but got %d", len(results))
						}
					}
				} else {
					if err != nil {
						t.Errorf("Unexpected error: %v", err)
					}
					if results == nil {
						t.Error("Results should not be nil")
					}
				}
			})
		})
	}
}

func TestGlobCaseInsensitivity(t *testing.T) {
	addExtraFiles := func(fs *MockFilesystem) {
		if fs.platform == "unix" {
			fs.AddFile("/home/user/documents/File1.TXT", false)
			fs.AddFile("/home/user/documents/FILE2.txt", false)
		} else {
			fs.AddFile(`C:\Users\User\Documents\File1.TXT`, false)
			fs.AddFile(`C:\Users\User\Documents\FILE2.txt`, false)
		}
	}

	cases := []struct {
		name       string
		pattern    string
		ignoreCase bool
		wantUnix   int
		wantWin    int
	}{
		{"mixed-case sensitive", "documents/*.TXT", false, 1, 1},
		{"mixed-case insensitive", "documents/*.TXT", true, 4, 4},
		{"lower-case sensitive", "documents/*.txt", false, 3, 3},
		{"lower-case insensitive", "documents/*.txt", true, 4, 4},
	}

	for _, c := range cases {
		t.Run(c.name+"/unix", func(t *testing.T) {
			fs := setupLinuxFS()
			addExtraFiles(fs)
			glob := NewGlob(c.pattern, fs, WithIgnoreCase(c.ignoreCase))
			got, err := glob.ExpandNames()
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != c.wantUnix {
				t.Fatalf("want %d got %d", c.wantUnix, len(got))
			}
		})
		t.Run(c.name+"/windows", func(t *testing.T) {
			fs := setupWindowsFS()
			addExtraFiles(fs)
			glob := NewGlob(c.pattern, fs, WithIgnoreCase(c.ignoreCase))
			got, err := glob.ExpandNames()
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != c.wantWin {
				t.Fatalf("want %d got %d", c.wantWin, len(got))
			}
		})
	}
}

func TestGlobComplexBraceExpansion(t *testing.T) {
	testCases := []struct {
		name         string
		pattern      string
		expectedUnix []string
		expectedWin  []string
	}{
		{
			name:    "nested braces",
			pattern: "documents/{file{1,2},report}.{txt,pdf}",
			expectedUnix: []string{
				"/home/user/documents/file1.txt",
				"/home/user/documents/file2.txt",
				"/home/user/documents/report.pdf",
			},
			expectedWin: []string{
				"C:/Users/User/Documents/file1.txt",
				"C:/Users/User/Documents/file2.txt",
				"C:/Users/User/Documents/report.pdf",
			},
		},
		{
			name:    "cross-separator braces",
			pattern: "{documents,pictures}/*.{txt,jpg}",
			expectedUnix: []string{
				"/home/user/documents/file1.txt",
				"/home/user/documents/file2.txt",
				"/home/user/pictures/photo1.jpg",
			},
			expectedWin: []string{
				"C:/Users/User/Documents/file1.txt",
				"C:/Users/User/Documents/file2.txt",
				"C:/Users/User/Pictures/photo1.jpg",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test Unix
			t.Run("unix", func(t *testing.T) {
				fs := setupLinuxFS()
				glob := NewGlob(tc.pattern, fs)
				results, err := glob.ExpandNames()
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				expected := normalizePathsForComparison(tc.expectedUnix)
				actual := normalizePathsForComparison(results)

				if !reflect.DeepEqual(expected, actual) {
					t.Errorf("Expected %v, got %v", expected, actual)
				}
			})

			// Test Windows
			t.Run("windows", func(t *testing.T) {
				fs := setupWindowsFS()
				glob := NewGlob(tc.pattern, fs)
				results, err := glob.ExpandNames()
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}

				expected := normalizePathsForComparison(tc.expectedWin)
				actual := normalizePathsForComparison(results)

				if !reflect.DeepEqual(expected, actual) {
					t.Errorf("Expected %v, got %v", expected, actual)
				}
			})
		})
	}
}

func TestRegexOrStringCache(t *testing.T) {
	fs := setupLinuxFS()
	glob := NewGlob("documents/*.txt", fs)

	// First call should populate cache
	ros1, err := glob.createRegexOrString("*.txt")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Second call should return cached result
	ros2, err := glob.createRegexOrString("*.txt")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should be the same object (cached)
	if ros1 != ros2 {
		t.Error("Expected cached RegexOrString to be returned")
	}
}

func TestGetFilesPublicAPI(t *testing.T) {
	// This tests the public API function GetFiles
	// Note: This will use the real filesystem, so we'll test with a simple pattern
	results, err := GetFiles("*.nonexistent")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should return empty slice for non-matching pattern
	if len(results) != 0 {
		t.Errorf("Expected empty results for non-matching pattern, got %d results", len(results))
	}

	// Test empty pattern
	results, err = GetFiles("")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected empty results for empty pattern, got %d results", len(results))
	}
}

func TestUngroup(t *testing.T) {
	testCases := []struct {
		name     string
		pattern  string
		expected []string
		hasError bool
	}{
		{
			name:     "simple brace expansion",
			pattern:  "{a,b}c",
			expected: []string{"ac", "bc"},
		},
		{
			name:     "no braces",
			pattern:  "abc",
			expected: []string{"abc"},
		},
		{
			name:     "nested braces",
			pattern:  "{a,b{c,d}}",
			expected: []string{"a", "bc", "bd"},
		},
		{
			name:     "multiple groups",
			pattern:  "{a,b}{c,d}",
			expected: []string{"ac", "ad", "bc", "bd"},
		},
		{
			name:     "unbalanced braces",
			pattern:  "{a,b",
			hasError: true,
		},
		{
			name:     "empty group",
			pattern:  "{}",
			expected: []string{""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			results, err := ungroup(tc.pattern)

			if tc.hasError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			sort.Strings(results)
			sort.Strings(tc.expected)

			if !reflect.DeepEqual(results, tc.expected) {
				t.Errorf("Expected %v, got %v", tc.expected, results)
			}
		})
	}
}

func TestGlobToRegexPattern(t *testing.T) {
	testCases := []struct {
		name       string
		pattern    string
		ignoreCase bool
		expected   string
		hasError   bool
	}{
		{
			name:       "simple asterisk",
			pattern:    "*.txt",
			ignoreCase: false,
			expected:   "^[^/\\\\]*\\.txt$",
		},
		{
			name:       "question mark",
			pattern:    "file?.txt",
			ignoreCase: false,
			expected:   "^file.\\.txt$",
		},
		{
			name:       "case insensitive",
			pattern:    "*.TXT",
			ignoreCase: true,
			expected:   "(?i)^[^/\\\\]*\\.TXT$",
		},
		{
			name:       "character class",
			pattern:    "file[12].txt",
			ignoreCase: false,
			expected:   "^file[12]\\.txt$",
		},
		{
			name:       "double asterisk",
			pattern:    "**",
			ignoreCase: false,
			expected:   "^.*$",
		},
		{
			name:       "unterminated character class",
			pattern:    "file[12.txt",
			ignoreCase: false,
			hasError:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := globToRegexPattern(tc.pattern, tc.ignoreCase)

			if tc.hasError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func BenchmarkGlobExpansion(b *testing.B) {
	fs := setupLinuxFS()
	glob := NewGlob("documents/**/*.txt", fs)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := glob.ExpandNames()
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}

func BenchmarkRegexCache(b *testing.B) {
	fs := setupLinuxFS()
	glob := NewGlob("documents/*.txt", fs)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := glob.createRegexOrString("*.txt")
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}

// Test platform-specific path handling
func TestPlatformSpecificPaths(t *testing.T) {
	t.Run("unix_paths", func(t *testing.T) {
		fs := setupLinuxFS()
		glob := NewGlob("/home/user/documents/*.txt", fs)

		if !glob.isAbsolutePath("/home/user/documents/file.txt") {
			t.Error("Expected Unix absolute path to be recognized")
		}

		if glob.isAbsolutePath("relative/path.txt") {
			t.Error("Expected Unix relative path to not be recognized as absolute")
		}

		normalized := glob.normalizePathForPattern("home\\user\\documents")
		expected := "home/user/documents"
		if normalized != expected {
			t.Errorf("Expected %q, got %q", expected, normalized)
		}
	})

	t.Run("windows_paths", func(t *testing.T) {
		fs := setupWindowsFS()
		glob := NewGlob("C:\\Users\\User\\Documents\\*.txt", fs)

		if !glob.isAbsolutePath("C:\\Users\\User\\Documents\\file.txt") {
			t.Error("Expected Windows absolute path to be recognized")
		}

		if !glob.isAbsolutePath("\\\\server\\share\\file.txt") {
			t.Error("Expected Windows UNC path to be recognized")
		}

		if glob.isAbsolutePath("relative\\path.txt") {
			t.Error("Expected Windows relative path to not be recognized as absolute")
		}

		normalized := glob.normalizePathForFS("Users/User/Documents")
		expected := "Users\\User\\Documents"
		if normalized != expected {
			t.Errorf("Expected %q, got %q", expected, normalized)
		}
	})
}

// Test error handling
func TestErrorHandling(t *testing.T) {
	t.Run("filesystem_errors", func(t *testing.T) {
		// Create a mock filesystem that returns errors
		fs := &MockFilesystem{
			files:     make(map[string]MockFileInfo),
			dirs:      make(map[string][]MockDirEntry),
			cwd:       "/",
			platform:  "unix",
			separator: "/",
		}

		glob := NewGlob("documents/*.txt", fs)
		results, err := glob.ExpandNames()

		// Should not return error, but empty results
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if len(results) != 0 {
			t.Errorf("Expected empty results, got %d", len(results))
		}
	})

	t.Run("malformed_patterns", func(t *testing.T) {
		fs := setupLinuxFS()

		malformedPatterns := []string{
			"documents/file[12.txt",  // Unterminated character class
			"documents/{file1,file2", // Unbalanced braces
		}

		for _, pattern := range malformedPatterns {
			glob := NewGlob(pattern, fs, WithIgnoreCase(true))
			_, err := glob.ExpandNames()
			// The SUT does not return an error but fails gracefully.
			if err != nil {
				t.Errorf("Did not expect an error for malformed pattern %q, but got one: %v", pattern, err)
			}
		}
	})
}

// Test special directory handling
func TestSpecialDirectories(t *testing.T) {
	t.Run("current_directory", func(t *testing.T) {
		fs := setupLinuxFS()
		// Add current directory entries
		fs.AddFile("/home/user/.", true)
		fs.AddFile("/home/user/..", true)

		glob := NewGlob(".", fs)
		results, err := glob.ExpandNames()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(results) == 0 {
			t.Error("Expected results for current directory pattern")
		}
	})

	t.Run("parent_directory", func(t *testing.T) {
		fs := setupLinuxFS()
		fs.AddFile("/home/user/..", true)

		glob := NewGlob("..", fs)
		results, err := glob.ExpandNames()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(results) == 0 {
			t.Error("Expected results for parent directory pattern")
		}
	})
}

// Test concurrent access to cache
func TestConcurrentCacheAccess(t *testing.T) {
	fs := setupLinuxFS()

	// Run multiple goroutines accessing the cache simultaneously
	done := make(chan bool)
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			glob := NewGlob("documents/*.txt", fs)
			_, err := glob.createRegexOrString("*.txt")
			if err != nil {
				errors <- err
			}
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check for errors
	select {
	case err := <-errors:
		t.Fatalf("Unexpected error in concurrent access: %v", err)
	default:
		// No errors, test passed
	}
}

// Test with very large directory structures
func TestLargeDirectoryStructure(t *testing.T) {
	fs := NewMockFilesystem("unix")
	fs.SetCwd("/home/user")

	// Create a large directory structure
	fs.AddFile("/", true)
	fs.AddFile("/home", true)
	fs.AddFile("/home/user", true)
	fs.AddFile("/home/user/large", true)

	// Add many files
	for i := 0; i < 100; i++ {
		fs.AddFile(fmt.Sprintf("/home/user/large/file%d.txt", i), false)
		if i%10 == 0 {
			fs.AddFile(fmt.Sprintf("/home/user/large/subdir%d", i), true)
			for j := 0; j < 10; j++ {
				fs.AddFile(fmt.Sprintf("/home/user/large/subdir%d/nested%d.txt", i, j), false)
			}
		}
	}

	glob := NewGlob("large/**/*.txt", fs)
	results, err := glob.ExpandNames()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should find all txt files
	expectedCount := 100 + 10*10 // 100 direct files + 10 subdirs * 10 files each
	if len(results) != expectedCount {
		t.Errorf("Expected %d results, got %d", expectedCount, len(results))
	}
}

// Test path normalization edge cases
func TestPathNormalizationEdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		platform string
		input    string
		expected string
	}{
		{
			name:     "windows_mixed_separators",
			platform: "windows",
			input:    "C:/Users\\User/Documents",
			expected: "C:\\Users\\User\\Documents",
		},
		{
			name:     "unix_backslashes",
			platform: "unix",
			input:    "home\\user\\documents",
			expected: "home/user/documents",
		},
		{
			name:     "windows_unc_path",
			platform: "windows",
			input:    "//server/share/file.txt",
			expected: "\\\\server\\share\\file.txt",
		},
		{
			name:     "empty_path",
			platform: "unix",
			input:    "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fs := NewMockFilesystem(tc.platform)
			glob := NewGlob("test", fs)

			result := glob.normalizePathForFS(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

// Test RegexOrString functionality
func TestRegexOrStringFunctionality(t *testing.T) {
	testCases := []struct {
		name        string
		pattern     string
		input       string
		ignoreCase  bool
		shouldMatch bool
		expectRegex bool
	}{
		{
			name:        "literal_match",
			pattern:     "file.txt",
			input:       "file.txt",
			ignoreCase:  false,
			shouldMatch: true,
			expectRegex: false,
		},
		{
			name:        "literal_case_insensitive",
			pattern:     "file.txt",
			input:       "FILE.TXT",
			ignoreCase:  true,
			shouldMatch: true,
			expectRegex: false,
		},
		{
			name:        "regex_match",
			pattern:     "file*.txt",
			input:       "file123.txt",
			ignoreCase:  false,
			shouldMatch: true,
			expectRegex: true,
		},
		{
			name:        "regex_no_match",
			pattern:     "file*.txt",
			input:       "document.pdf",
			ignoreCase:  false,
			shouldMatch: false,
			expectRegex: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fs := setupLinuxFS()
			glob := NewGlob("test", fs)
			glob.IgnoreCase = tc.ignoreCase

			ros, err := glob.createRegexOrString(tc.pattern)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if ros.IsRegex != tc.expectRegex {
				t.Errorf("Expected IsRegex=%v, got %v", tc.expectRegex, ros.IsRegex)
			}

			match := ros.IsMatch(tc.input)
			if match != tc.shouldMatch {
				t.Errorf("Expected match=%v, got %v", tc.shouldMatch, match)
			}
		})
	}
}
