package gocover

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser/filtering"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/settings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Add blank imports to register the necessary formatters for this test.
	_ "github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/formatter/default"
	_ "github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/formatter/golang"
)

// MockFileInfo implements fs.FileInfo for testing.
type MockFileInfo struct {
	name string
}

func (m MockFileInfo) Name() string       { return m.name }
func (m MockFileInfo) Size() int64        { return 0 }
func (m MockFileInfo) Mode() fs.FileMode  { return 0 }
func (m MockFileInfo) ModTime() time.Time { return time.Now() }
func (m MockFileInfo) IsDir() bool        { return false }
func (m MockFileInfo) Sys() interface{}   { return nil }

// MockFileReader for testing without hitting the disk.
type MockFileReader struct {
	Files map[string]string
}

func (m *MockFileReader) ReadFile(path string) ([]string, error) {
	content, ok := m.Files[path]
	if !ok {
		return nil, errors.New("file not found: " + path)
	}
	return strings.Split(content, "\n"), nil
}

func (m *MockFileReader) CountLines(path string) (int, error) {
	lines, err := m.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return len(lines), nil
}

func (m *MockFileReader) Stat(name string) (fs.FileInfo, error) {
	if _, ok := m.Files[name]; ok {
		return MockFileInfo{name: filepath.Base(name)}, nil
	}
	return nil, os.ErrNotExist
}

// mockParserConfig for providing test configuration.
type mockParserConfig struct {
	srcDirs        []string
	assemblyFilter filtering.IFilter
	classFilter    filtering.IFilter
	fileFilter     filtering.IFilter
	settings       *settings.Settings
}

func (m *mockParserConfig) SourceDirectories() []string        { return m.srcDirs }
func (m *mockParserConfig) AssemblyFilters() filtering.IFilter { return m.assemblyFilter }
func (m *mockParserConfig) ClassFilters() filtering.IFilter    { return m.classFilter }
func (m *mockParserConfig) FileFilters() filtering.IFilter     { return m.fileFilter }
func (m *mockParserConfig) Settings() *settings.Settings       { return m.settings }

func newTestConfig() *mockParserConfig {
	noFilter, _ := filtering.NewDefaultFilter(nil)
	return &mockParserConfig{
		srcDirs:        []string{"/project/src"},
		assemblyFilter: noFilter,
		classFilter:    noFilter,
		fileFilter:     noFilter,
		settings:       settings.NewSettings(),
	}
}

func TestSupportsFile(t *testing.T) {
	t.Run("ValidGoCoverFile", func(t *testing.T) {
		file, err := os.CreateTemp("", "gocover_*.out")
		require.NoError(t, err)
		defer os.Remove(file.Name())
		_, err = file.WriteString("mode: set\nfile.go:1.1,2.2 1 1\n")
		require.NoError(t, err)
		file.Close()

		p := NewGoCoverParser(nil)
		assert.True(t, p.SupportsFile(file.Name()))
	})

	t.Run("InvalidFile_WrongPrefix", func(t *testing.T) {
		file, err := os.CreateTemp("", "gocover_*.out")
		require.NoError(t, err)
		defer os.Remove(file.Name())
		_, err = file.WriteString("not mode: set\n")
		require.NoError(t, err)
		file.Close()

		p := NewGoCoverParser(nil)
		assert.False(t, p.SupportsFile(file.Name()))
	})

	t.Run("InvalidFile_Empty", func(t *testing.T) {
		file, err := os.CreateTemp("", "gocover_*.out")
		require.NoError(t, err)
		defer os.Remove(file.Name())
		file.Close()

		p := NewGoCoverParser(nil)
		assert.False(t, p.SupportsFile(file.Name()))
	})
}

func TestParse(t *testing.T) {
	// Arrange
	coverProfileContent := `mode: set
calculator/calculator.go:4.24,6.2 1 1
calculator/calculator.go:9.29,11.2 1 1
calculator/calculator.go:15.29,16.22 1 1
calculator/calculator.go:16.22,18.3 1 1
calculator/calculator.go:19.2,19.14 1 0`

	calculatorGoContent := `package calculator

// Add performs addition
func Add(a, b int) int {
	return a + b
}

// Subtract performs subtraction
func Subtract(a, b int) int {
	return a - b
}

// Multiply performs multiplication
func Multiply(a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}
	return a * b
}
func Divide(a, b int) int {
	return a / b // Bug: division by zero not handled
}
`
	reportFile, err := os.CreateTemp(t.TempDir(), "cover_*.out")
	require.NoError(t, err)
	_, err = reportFile.WriteString(coverProfileContent)
	require.NoError(t, err)
	reportFile.Close()

	mockFileReader := &MockFileReader{
		Files: map[string]string{
			filepath.Join("/project/src", "calculator/calculator.go"): calculatorGoContent,
		},
	}

	p := NewGoCoverParser(mockFileReader)
	config := newTestConfig()
	config.Settings().DefaultAssemblyName = "GoTestAssembly"

	result, err := p.Parse(reportFile.Name(), config)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Assert
	assemblies := result.Assemblies
	require.Len(t, assemblies, 1, "Should have 1 assembly")
	assembly := assemblies[0]
	assert.Equal(t, "GoTestAssembly", assembly.Name)
	assert.Nil(t, assembly.BranchesCovered, "Go cover has no branch data")

	require.Len(t, assembly.Classes, 1, "Should have 1 class (package)")
	class := assembly.Classes[0]
	assert.Equal(t, "calculator", class.Name)
	assert.Equal(t, "calculator", class.DisplayName)

	require.Len(t, class.Files, 1, "Should have 1 file in the class")
	file := class.Files[0]
	assert.Equal(t, filepath.Join("/project/src", "calculator/calculator.go"), file.Path)

	// Based on the coverage blocks:
	// - Lines 4, 5: Add function (covered)
	// - Lines 9, 10: Subtract function (covered)
	// - Lines 15, 16: Multiply function condition (covered)
	// - Lines 16, 18: Multiply function if-block (covered, line 16 gets double-counted)
	// - Line 19: Closing brace (0 hits, but should be non-coverable)
	//
	// Closing braces (lines 6, 11, 17, 19) should be non-coverable
	// Expected coverable lines: 4, 5, 9, 10, 15, 16, 18 = 7 lines
	// Expected covered lines: 4, 5, 9, 10, 15, 16, 18 = 7 lines

	assert.Equal(t, 7, file.CoverableLines, "Expected 7 coverable lines")
	assert.Equal(t, 7, file.CoveredLines, "Expected 7 covered lines")
	assert.Equal(t, 7, class.LinesValid, "Expected 7 valid lines in class")
	assert.Equal(t, 7, class.LinesCovered, "Expected 7 covered lines in class")
	assert.Equal(t, 7, assembly.LinesValid, "Expected 7 valid lines in assembly")
	assert.Equal(t, 7, assembly.LinesCovered, "Expected 7 covered lines in assembly")

	// Check specific line hits
	lineHits := make(map[int]int)
	lineStatus := make(map[int]model.LineVisitStatus)
	for _, l := range file.Lines {
		if l.Hits > -1 {
			lineHits[l.Number] = l.Hits
		}
		lineStatus[l.Number] = l.LineVisitStatus
	}

	// Check covered lines
	assert.Equal(t, 1, lineHits[4], "Line 4 should have 1 hit (Add function)")
	assert.Equal(t, 1, lineHits[5], "Line 5 should have 1 hit (Add return)")
	assert.Equal(t, 1, lineHits[9], "Line 9 should have 1 hit (Subtract function)")
	assert.Equal(t, 1, lineHits[10], "Line 10 should have 1 hit (Subtract return)")
	assert.Equal(t, 1, lineHits[15], "Line 15 should have 1 hit (Multiply if)")
	assert.Equal(t, 2, lineHits[16], "Line 16 should have 2 hits (covered by 2 blocks)")
	assert.Equal(t, 1, lineHits[18], "Line 18 should have 1 hit (return a * b)")

	// Check that closing braces are not coverable
	assert.Equal(t, model.NotCoverable, lineStatus[6], "Line 6 (closing brace) should not be coverable")
	assert.Equal(t, model.NotCoverable, lineStatus[11], "Line 11 (closing brace) should not be coverable")
	assert.Equal(t, model.NotCoverable, lineStatus[17], "Line 17 (closing brace) should not be coverable")
	assert.Equal(t, model.NotCoverable, lineStatus[19], "Line 19 (closing brace) should not be coverable")

	// Verify closing braces don't have hit counts in the map
	_, exists := lineHits[6]
	assert.False(t, exists, "Line 6 (closing brace) should not have hit count")
	_, exists = lineHits[11]
	assert.False(t, exists, "Line 11 (closing brace) should not have hit count")
	_, exists = lineHits[17]
	assert.False(t, exists, "Line 17 (closing brace) should not have hit count")
	_, exists = lineHits[19]
	assert.False(t, exists, "Line 19 (closing brace) should not have hit count")
}

func TestParseDebug(t *testing.T) {
	// Arrange
	coverProfileContent := `mode: set
calculator/calculator.go:4.24,6.2 1 1
calculator/calculator.go:9.29,11.2 1 1
calculator/calculator.go:15.29,16.22 1 1
calculator/calculator.go:16.22,18.3 1 1
calculator/calculator.go:19.2,19.14 1 0`

	calculatorGoContent := `package calculator

// Add performs addition
func Add(a, b int) int {
	return a + b
}

// Subtract performs subtraction
func Subtract(a, b int) int {
	return a - b
}

// Multiply performs multiplication
func Multiply(a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}
	return a * b
}
func Divide(a, b int) int {
	return a / b // Bug: division by zero not handled
}
`

	reportFile, err := os.CreateTemp(t.TempDir(), "cover_*.out")
	require.NoError(t, err)
	_, err = reportFile.WriteString(coverProfileContent)
	require.NoError(t, err)
	reportFile.Close()

	mockFileReader := &MockFileReader{
		Files: map[string]string{
			filepath.Join("/project/src", "calculator/calculator.go"): calculatorGoContent,
		},
	}

	p := NewGoCoverParser(mockFileReader)
	config := newTestConfig()
	config.Settings().DefaultAssemblyName = "GoTestAssembly"

	result, err := p.Parse(reportFile.Name(), config)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Debug: Print line-by-line coverage
	file := result.Assemblies[0].Classes[0].Files[0]
	t.Logf("File: %s", file.Path)
	t.Logf("CoverableLines: %d, CoveredLines: %d", file.CoverableLines, file.CoveredLines)

	coverableCount := 0
	coveredCount := 0

	for _, line := range file.Lines {
		if line.LineVisitStatus == model.Covered {
			coveredCount++
			t.Logf("Line %d: COVERED (hits: %d) - %s", line.Number, line.Hits, strings.TrimSpace(line.Content))
		} else if line.LineVisitStatus == model.NotCovered {
			t.Logf("Line %d: NOT COVERED (hits: %d) - %s", line.Number, line.Hits, strings.TrimSpace(line.Content))
		}

		if line.LineVisitStatus == model.Covered || line.LineVisitStatus == model.NotCovered {
			coverableCount++
		}
	}

	t.Logf("Actual coverable: %d, covered: %d", coverableCount, coveredCount)

	// Based on the coverage blocks, here's what should be covered:
	// Block 1: 4.24,6.2 (lines 4-6, hit=1) - Add function
	// Block 2: 9.29,11.2 (lines 9-11, hit=1) - Subtract function
	// Block 3: 15.29,16.22 (lines 15-16, hit=1) - Multiply function start
	// Block 4: 16.22,18.3 (lines 16-18, hit=1) - Multiply if block
	// Block 5: 19.2,19.14 (line 19, hit=0) - Divide function

	// Expected coverable lines: 5, 10, 15, 16, 17, 19 (line 6, 11, 18 might be closing braces)
	// Let's check what the actual coverage shows
}
