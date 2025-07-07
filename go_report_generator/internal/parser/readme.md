## Manual: Creating a New Parser for Go ReportGenerator

This guide provides a comprehensive walkthrough for adding a new coverage report parser to the `go_report_generator` project. We will use the native Go coverage profile format (`coverage.out`) as our example.

### 1. Core Concepts and Responsibilities

Before writing code, it's essential to understand the architecture and the role of a parser in this system. The design is heavily inspired by the original C# project, and your port has done a great job of maintaining that structure.

#### The Parser's Role
A parser's single responsibility is to **translate a specific coverage report format into the project's internal, universal data model**. It does not deal with generating HTML, text summaries, or any other output format. It simply reads a file, understands its structure, and populates the Go structs found in `internal/model/`.

#### Key Components
1.  **The `IParser` Interface (`internal/parser/parser_config.go`):** This is the contract every parser must fulfill.
    *   `Name() string`: Returns the parser's name (e.g., "Cobertura", "GoCover").
    *   `SupportsFile(filePath string) bool`: A crucial method that quickly checks if the parser can handle a given file. This is used by the factory to auto-detect the correct parser. It should be fast and not read the entire file if possible.
    *   `Parse(filePath string, config ParserConfig) (*ParserResult, error)`: The main method. It reads and processes the entire report file and returns a `ParserResult`.

2.  **The Parser Factory (`internal/parser/factory.go`):** This is the "plugin" system.
    *   It maintains a list of all available parsers.
    *   To make a new parser available to the application, you must **register** it with the factory. This is typically done in an `init()` function within your parser's package.
    *   The `FindParserForFile` function iterates through registered parsers, calling `SupportsFile` on each until it finds a match.

3.  **The Universal Data Model (`internal/model/`):** This is the heart of the application. Your parser's output must be a `ParserResult` containing a tree of these structs. Understanding this model is critical.

| Struct | C# Equivalent | Responsibility / Meaning in Go Port | How to Populate for `go cover` |
| :--- | :--- | :--- | :--- |
| **`model.SummaryResult`** | `SummaryResult` | Top-level container for the entire parsed report. Your parser doesn't create this; it creates `ParserResult`s which are later merged into a `SummaryResult`. | *(Not created directly by parser)* |
| **`parser.ParserResult`** | `ParserResult` | The direct output of your `Parse` method. Contains a list of assemblies and other metadata. | This is the main struct your `Parse` method will return. |
| **`model.Assembly`** | `Assembly` | A logical grouping of classes, typically a DLL or EXE. | Go doesn't have assemblies. A good convention is to use the **Go module name** (from `go.mod`) as a single, top-level assembly. |
| **`model.Class`** | `Class` | Represents a single class. | Go doesn't have classes. A good convention is to use the **Go package path** (e.g., `calculator`, `internal/utils`) as the "class". This provides a logical grouping for files within a package. |
| **`model.CodeFile`** | `CodeFile` | Represents a single source code file. | The file path from the `coverage.out` line (e.g., `calculator/calculator.go`). Your parser must resolve this to an absolute path. |
| **`model.Method`** | `Method` | Represents a method or property within a class. | The `go cover` format **does not provide method information**, only line ranges. You have two options: 1) **(Easy)** Don't populate this. 2) **(Advanced)** Use Go's AST (`go/parser` and `go/ast`) to parse the source file, find function declarations, and map coverage line ranges to those functions. For this guide, we'll start with the easy option. |
| **`model.Line`** | `LineAnalysis` | Represents a single line in a source file with its coverage data. | This is the core data from `go cover`. You'll populate `LineNumber`, `Hits` (from the `visits` column), and `LineVisitStatus` (based on `Hits`). |
| **`model.Branch`** | `Branch` | Represents a single branch on a line (e.g., if/else). | The `go cover` format **does not provide branch information**. `IsBranchPoint` will be `false`, and `Branch` slices will be empty. |

---

### 2. Step-by-Step: Creating a `GoCover` Parser

Let's build a parser for `coverage.out`.

#### Step 1: Create the Directory and Files

Following the existing structure, create a new directory for your parser.

1.  Create the directory: `go_report_generator/internal/parser/gocover/`
2.  Inside this directory, create the following files:
    *   `parser.go`: Will contain the `IParser` implementation.
    *   `processing.go`: Will contain the core logic for reading and processing the `coverage.out` file.
    *   `parser_test.go`: Will contain unit tests for your parser.

#### Step 2: Implement the `IParser` Interface (`gocover/parser.go`)

This file is the public entry point for your parser and connects it to the factory.

```go
// in: go_report_generator/internal/parser/gocover/parser.go

package gocover

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser"
)

// GoCoverParser implements the IParser interface for Go's native coverage format.
type GoCoverParser struct{}

// NewGoCoverParser creates a new GoCoverParser.
func NewGoCoverParser() parser.IParser {
	return &GoCoverParser{}
}

// Name returns the name of the parser.
func (p *GoCoverParser) Name() string {
	return "GoCover"
}

// SupportsFile checks if the given file is a Go coverage profile.
// It does this by checking if the first line starts with "mode:".
func (p *GoCoverParser) SupportsFile(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		return strings.HasPrefix(scanner.Text(), "mode:")
	}

	return false
}

// Parse processes the Go coverage file and transforms it into a common ParserResult.
func (p *GoCoverParser) Parse(filePath string, config parser.ParserConfig) (*parser.ParserResult, error) {
	lines, err := readLines(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read go cover file %s: %w", filePath, err)
	}

	if len(lines) == 0 || !strings.HasPrefix(lines[0], "mode:") {
		return nil, fmt.Errorf("file %s is not a valid go cover file (missing 'mode:' header)", filePath)
	}

	// The heavy lifting is delegated to a processor, keeping this file clean.
	processor := newGoCoverProcessor(config, config.SourceDirectories())
	assemblies, sourceDirs, err := processor.Process(lines)
	if err != nil {
		return nil, fmt.Errorf("failed to process go cover data from %s: %w", filePath, err)
	}

	return &parser.ParserResult{
		Assemblies:             assemblies,
		SourceDirectories:      sourceDirs, // Go format does not specify source dirs
		SupportsBranchCoverage: false,      // Go format does not have branch data
		ParserName:             p.Name(),
		MinimumTimeStamp:       nil, // Go format does not have timestamps
		MaximumTimeStamp:       nil,
	}, nil
}

// readLines is a simple helper to read a file into a slice of strings.
func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
```

#### Step 3: Register the Parser

Now, make the application aware of your new parser. Add an `init()` function to `gocover/parser.go` to register it with the factory.

```go
// in: go_report_generator/internal/parser/gocover/parser.go
// Add this function to the file.

func init() {
	parser.RegisterParser(NewGoCoverParser())
}
```

This single step ensures that when `main.go` calls `parser.FindParserForFile`, your `GoCoverParser` will be in the list of candidates.

#### Step 4: Implement the Processing Logic (`gocover/processing.go`)

This is where you'll translate the `coverage.out` format into the `model` structs.

```go
// in: go_report_generator/internal/parser/gocover/processing.go

package gocover

import (
	"fmt"
	"go/build"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/filereader"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/utils"
)

type goCoverProcessor struct {
	config     parser.ParserConfig
	sourceDirs []string
}

func newGoCoverProcessor(config parser.ParserConfig, sourceDirs []string) *goCoverProcessor {
	return &goCoverProcessor{
		config:     config,
		sourceDirs: sourceDirs,
	}
}

// Process parses the raw lines from a coverage.out file.
func (p *goCoverProcessor) Process(lines []string) ([]model.Assembly, []string, error) {
	// 1. Group coverage data by file
	coverageByFile := make(map[string][]coverBlock)
	for _, line := range lines[1:] { // Skip "mode: set"
		block, err := parseCoverLine(line)
		if err != nil {
			slog.Warn("Skipping invalid go cover line", "line", line, "error", err)
			continue
		}
		coverageByFile[block.filePath] = append(coverageByFile[block.filePath], block)
	}

	// 2. Group files by package (which we treat as a "Class")
	filesByPackage := make(map[string][]string)
	for filePath := range coverageByFile {
		// Attempt to resolve the Go package path from the file path.
		// This might need adjustment based on project structure.
		// A simple way is to use the directory name.
		pkgPath := filepath.Dir(filePath)
		filesByPackage[pkgPath] = append(filesByPackage[pkgPath], filePath)
	}

	// 3. Create the data model
	// We'll use the module name as the single "Assembly"
	moduleName, err := p.getModuleName()
	if err != nil {
		slog.Warn("Could not determine Go module name, using 'DefaultAssembly'", "error", err)
		moduleName = "DefaultAssembly"
	}
	
	assembly := model.Assembly{
		Name:    moduleName,
		Classes: []model.Class{},
	}

	for pkgPath, filePaths := range filesByPackage {
		if !p.config.ClassFilters().IsElementIncludedInReport(pkgPath) {
			continue
		}

		class := model.Class{
			Name:        pkgPath, // Use package path as class name
			DisplayName: pkgPath,
			Files:       []model.CodeFile{},
		}

		for _, filePath := range filePaths {
			if !p.config.FileFilters().IsElementIncludedInReport(filePath) {
				continue
			}

			codeFile, err := p.processFile(filePath, coverageByFile[filePath])
			if err != nil {
				slog.Warn("Failed to process file, skipping.", "file", filePath, "error", err)
				continue
			}

			class.Files = append(class.Files, *codeFile)

			// Aggregate stats from file to class
			class.LinesCovered += codeFile.CoveredLines
			class.LinesValid += codeFile.CoverableLines
			class.TotalLines += codeFile.TotalLines
		}
		
		if len(class.Files) > 0 {
			assembly.Classes = append(assembly.Classes, class)
		}
	}

	// Aggregate stats from class to assembly
	for _, class := range assembly.Classes {
		assembly.LinesCovered += class.LinesCovered
		assembly.LinesValid += class.LinesValid
		assembly.TotalLines += class.TotalLines
	}

	return []model.Assembly{assembly}, nil, nil
}

// processFile handles a single source file and its coverage blocks.
func (p *goCoverProcessor) processFile(filePath string, blocks []coverBlock) (*model.CodeFile, error) {
	fullPath, err := utils.FindFileInSourceDirs(filePath, p.sourceDirs)
	if err != nil {
		// If file not found, we can't get line content or total lines, but can still report coverage.
		slog.Warn("Source file not found, line content will be missing.", "file", filePath, "error", err)
		fullPath = filePath // Use original as fallback
	}

	sourceLines, _ := filereader.ReadLinesInFile(fullPath)
	totalLines := len(sourceLines)

	// Create a map to hold coverage data per line number
	lineHits := make(map[int]int)
	for _, block := range blocks {
		for lineNum := block.startLine; lineNum <= block.endLine; lineNum++ {
			lineHits[lineNum] += block.visits
		}
	}

	maxLineNum := 0
	for lineNum := range lineHits {
		if lineNum > maxLineNum {
			maxLineNum = lineNum
		}
	}
	if totalLines > maxLineNum {
		maxLineNum = totalLines
	}
	
	finalLines := make([]model.Line, 0, maxLineNum)
	coveredLinesInFile := 0
	coverableLinesInFile := 0

	for i := 1; i <= maxLineNum; i++ {
		hits, ok := lineHits[i]
		
		line := model.Line{
			Number: i,
			Hits:   -1, // Default to NotCoverable
		}

		if ok {
			line.Hits = hits
			coverableLinesInFile++
			if hits > 0 {
				coveredLinesInFile++
			}
		}

		if i <= len(sourceLines) {
			line.Content = sourceLines[i-1]
		}
		
		line.LineVisitStatus = determineLineVisitStatus(line.Hits, line.IsBranchPoint, line.CoveredBranches, line.TotalBranches)
		finalLines = append(finalLines, line)
	}

	return &model.CodeFile{
		Path:           fullPath,
		Lines:          finalLines,
		CoveredLines:   coveredLinesInFile,
		CoverableLines: coverableLinesInFile,
		TotalLines:     totalLines,
	}, nil
}

// coverBlock represents a single line from the coverage.out file.
type coverBlock struct {
	filePath  string
	startLine int
	endLine   int
	stmts     int
	visits    int
}

// parseCoverLine parses a line like "file.go:1.2,3.4 5 6"
func parseCoverLine(line string) (coverBlock, error) {
	// Example: calculator/calculator.go:4.24,6.2 1 1
	parts := strings.Fields(line)
	if len(parts) != 3 {
		return coverBlock{}, fmt.Errorf("invalid line format: expected 3 parts, got %d", len(parts))
	}

	fileAndRange := parts[0]
	stmtsStr := parts[1]
	visitsStr := parts[2]

	colonIndex := strings.LastIndex(fileAndRange, ":")
	if colonIndex == -1 {
		return coverBlock{}, fmt.Errorf("invalid line format: missing ':' in position part")
	}
	filePath := fileAndRange[:colonIndex]
	lineRange := fileAndRange[colonIndex+1:]

	rangeParts := strings.Split(lineRange, ",")
	if len(rangeParts) != 2 {
		return coverBlock{}, fmt.Errorf("invalid line range format")
	}

	startPos, err := strconv.Atoi(strings.Split(rangeParts[0], ".")[0])
	if err != nil {
		return coverBlock{}, fmt.Errorf("invalid start line number: %w", err)
	}
	endPos, err := strconv.Atoi(strings.Split(rangeParts[1], ".")[0])
	if err != nil {
		return coverBlock{}, fmt.Errorf("invalid end line number: %w", err)
	}

	stmts, _ := strconv.Atoi(stmtsStr)
	visits, _ := strconv.Atoi(visitsStr)

	return coverBlock{
		filePath:  filePath,
		startLine: startPos,
		endLine:   endPos,
		stmts:     stmts,
		visits:    visits,
	}, nil
}

// getModuleName finds the go.mod file to determine the module name.
func (p *goCoverProcessor) getModuleName() (string, error) {
	// This helper could be in a shared utility package.
	// For simplicity, we define it here. It searches for go.mod upwards from a source directory.
	
	// Use the Go build context to find the project root from GOPATH or module info
    // This is a more robust way to find the project root than walking up directories manually.
    pkg, err := build.Default.ImportDir(".", 0)
    if err != nil || pkg.Module == nil {
        return "", fmt.Errorf("could not determine module path: %w. Make sure to run from within a Go module.", err)
    }

	return pkg.Module.Path, nil
}


// Duplicated from cobertura/processing_orchestrator.go for now. Could be moved to a shared utility.
func determineLineVisitStatus(hits int, isBranchPoint bool, coveredBranches int, totalBranches int) model.LineVisitStatus {
	if hits < 0 {
		return model.NotCoverable
	}
	if isBranchPoint {
		if totalBranches == 0 {
			return model.NotCoverable
		}
		if coveredBranches == totalBranches {
			return model.Covered
		}
		if coveredBranches > 0 {
			return model.PartiallyCovered
		}
		return model.NotCovered
	}
	if hits > 0 {
		return model.Covered
	}
	return model.NotCovered
}
```

#### Step 5: Testing Your Parser (`gocover/parser_test.go`)

Unit testing is crucial. Since your processing logic now depends on reading files (`go.mod`, `coverage.out`, source files), you need a way to mock this. The `cobertura` parser defines a `FileReader` interface, which is an excellent pattern to copy.

1.  **Define a `FileReader` interface** in a new file, e.g., `internal/parser/gocover/interfaces.go` (or reuse the one from Cobertura if it's made more generic). For this example, let's assume you create a local one or make a central one.
2.  **Update `goCoverProcessor`** to accept this interface.
3.  **Create a `MockFileReader`** in your test file.

**Example `gocover/parser_test.go`:**

```go
// in: go_report_generator/internal/parser/gocover/parser_test.go

package gocover

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser/filtering"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/settings"
)

// MockParserConfig for testing
type mockConfig struct{}
func (m *mockConfig) SourceDirectories() []string        { return []string{"/test/project"} }
func (m *mockConfig) AssemblyFilters() filtering.IFilter { f, _ := filtering.NewDefaultFilter(nil); return f }
func (m *mockConfig) ClassFilters() filtering.IFilter    { f, _ := filtering.NewDefaultFilter(nil); return f }
func (m *mockConfig) FileFilters() filtering.IFilter     { f, _ := filtering.NewDefaultFilter(nil, true); return f }
func (m *mockConfig) Settings() *settings.Settings       { return settings.NewSettings() }

func TestParse(t *testing.T) {
	// Create a temporary coverage.out file
	coverFileContent := `mode: set
my/module/calculator.go:4.24,6.2 1 1
my/module/calculator.go:9.29,11.2 1 0
`
	coverFilePath := filepath.Join(t.TempDir(), "coverage.out")
	err := os.WriteFile(coverFilePath, []byte(coverFileContent), 0644)
	require.NoError(t, err)

	// Create a temporary source directory and source file
	sourceDir := t.TempDir()
	calculatorDir := filepath.Join(sourceDir, "my", "module")
	err = os.MkdirAll(calculatorDir, 0755)
	require.NoError(t, err)
	
	sourceFileContent := `package calculator

func Add(a, b int) int {
	// Line 4
	if a > 0 {
		return a + b
	} // Line 6
	return 0
}

func Subtract(a, b int) int {
	// Line 10
	return a - b
} // Line 11
`
	sourceFilePath := filepath.Join(calculatorDir, "calculator.go")
	err = os.WriteFile(sourceFilePath, []byte(sourceFileContent), 0644)
	require.NoError(t, err)
	
	// Create a temporary go.mod
	goModContent := "module my/module\n"
	goModPath := filepath.Join(sourceDir, "go.mod")
	err = os.WriteFile(goModPath, []byte(goModContent), 0644)
	require.NoError(t, err)
	
	// Change working directory to the test project root
	originalWD, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(sourceDir)
	require.NoError(t, err)
	defer os.Chdir(originalWD)

	// --- Run Parser ---
	cfg := &mockConfig{}
	parser := NewGoCoverParser()
	result, err := parser.Parse(coverFilePath, cfg)
	
	// --- Assertions ---
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, "GoCover", result.ParserName)
	assert.False(t, result.SupportsBranchCoverage)
	
	// Assembly assertions
	require.Len(t, result.Assemblies, 1)
	assembly := result.Assemblies[0]
	assert.Equal(t, "my/module", assembly.Name)
	
	// Class assertions
	require.Len(t, assembly.Classes, 1)
	class := assembly.Classes[0]
	assert.Equal(t, "my/module", class.Name)
	
	// File assertions
	require.Len(t, class.Files, 1)
	codeFile := class.Files[0]
	// Path should be resolved to be absolute
	expectedAbsPath, _ := filepath.Abs(sourceFilePath)
	assert.Equal(t, expectedAbsPath, codeFile.Path)

	// Line coverage assertions
	assert.Equal(t, 1, codeFile.CoveredLines)
	assert.Equal(t, 2, codeFile.CoverableLines)
	
	// Check specific lines
	line4 := codeFile.Lines[3] // 0-indexed
	assert.Equal(t, 4, line4.Number)
	assert.Equal(t, 1, line4.Hits)

	line10 := codeFile.Lines[9]
	assert.Equal(t, 10, line10.Number)
	assert.Equal(t, 0, line10.Hits)
}
```

This test is more of an integration test as it hits the filesystem. A true unit test would use the `FileReader` mock pattern.

#### Step 6: Integration and End-to-End Testing

The final step is to ensure your parser works with the full application.

1.  **Build the Go report generator** (`go build ./cmd/...`).
2.  **Update `Testprojects/generate_reports.py`**:
    *   In the `run_go_project_workflow` function, you currently use `gocover-cobertura` to convert to Cobertura XML first.
    *   Add a new workflow path that directly calls `go_report_generator` with the native `coverage.out` file. Since `FindParserForFile` will auto-detect the format, you don't need to specify a parser type. The command would look something like this:

        ```python
        # In generate_reports.py, inside run_go_project_workflow
        print("\n--- Generating Go project report with Go tool (Native Format) ---")
        go_report_command_native = [
            "path/to/your/go_report_generator_executable", # or just the name if in PATH
            f"-report={GO_PROJECT_NATIVE_COVERAGE_FILE.resolve()}", # Use coverage.out directly
            f"-output={GO_PROJECT_REPORTS_FROM_GO_TOOL_NATIVE_DIR.resolve()}", # A new output dir
            f"-reporttypes=Html" # Or other types
        ]
        run_command(go_report_command_native, command_name="Go Report Generator (Native)")
        ```

3.  **Run the Python script** and inspect the generated HTML report. Verify that the line coverage numbers and file contents are displayed correctly.

---
### Best Practices and Final Thoughts

*   **Follow the Cobertura Pattern:** The existing `cobertura` parser is your best reference. Its separation of concerns (Parser, Processor, testing with mocks) is a solid foundation.
*   **Handle Errors Gracefully:** Don't let your parser crash the whole application on a single malformed line. Log warnings (`slog.Warn`) and skip the problematic entry.
*   **Documentation:** Add comments to your code explaining why you made certain mapping decisions (e.g., "Go packages are mapped to Classes", "Go modules are mapped to Assemblies").
*   **Limitations:** Be aware of the limitations of the format you are parsing. Explicitly document that the Go format does not support branch coverage or (by default) method-level metrics. This manages expectations for users of your tool.

By following this guide, you can systematically and robustly add support for any new coverage format to your Go ReportGenerator project.