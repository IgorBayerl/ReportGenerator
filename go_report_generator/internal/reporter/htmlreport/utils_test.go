package htmlreport

import (
	"strings"
	"testing"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
)

// TestGenerateUniqueFilename tests the generateUniqueFilename function.
func TestGenerateUniqueFilename(t *testing.T) {
	// Re-define for test scope if not exported or to ensure test uses specific values
	// If sanitizeFilenameChars and maxFilenameLengthBase are exported from utils.go, you can use them directly.
	// Otherwise, define them here for the test.
	// For this example, let's assume they are package-level vars/consts in the SUT (System Under Test).
	// sanitizeFilenameChars := regexp.MustCompile(`[^a-zA-Z0-9_-]+`) // Already package-level in utils.go
	// const maxFilenameLengthBase = 95 // Already package-level in utils.go

	tests := []struct {
		name              string
		assemblyShortName string
		className         string
		existingFilenames map[string]struct{}
		want              string
		wantExistingCount int // How many entries we expect in existingFilenames after the call
	}{
		{
			name:              "simple case, no existing",
			assemblyShortName: "MyAssembly",
			className:         "MyClass",
			existingFilenames: make(map[string]struct{}),
			want:              "MyAssemblyMyClass.html",
			wantExistingCount: 1,
		},
		{
			name:              "with namespace, no existing",
			assemblyShortName: "MyAssembly",
			className:         "MyNamespace.Core.MyClass",
			existingFilenames: make(map[string]struct{}),
			want:              "MyAssemblyMyClass.html",
			wantExistingCount: 1,
		},
		{
			name:              "sanitize special chars",
			assemblyShortName: "My.Assembly",
			className:         "MyClass<T>",
			existingFilenames: make(map[string]struct{}),
			want:              "MyAssemblyMyClassT.html", // '<' and '>' removed
			wantExistingCount: 1,
		},
		{
			name:              "sanitize more special chars",
			assemblyShortName: "Test.Proj",
			className:         "SomeClass::Sub/Inner",
			existingFilenames: make(map[string]struct{}),
			want:              "TestProjInner.html", // '::' and '/' removed, takes last part after namespace
			wantExistingCount: 1,
		},
		{
			name:              "filename collision, simple case",
			assemblyShortName: "MyAssembly",
			className:         "MyClass",
			existingFilenames: map[string]struct{}{
				"myassemblymyclass.html": {}, // Note: existingFilenames uses ToLower
			},
			want:              "MyAssemblyMyClass2.html",
			wantExistingCount: 2,
		},
		{
			name:              "filename collision, multiple existing",
			assemblyShortName: "MyAssembly",
			className:         "MyClass",
			existingFilenames: map[string]struct{}{
				"myassemblymyclass.html":  {},
				"myassemblymyclass2.html": {},
			},
			want:              "MyAssemblyMyClass3.html",
			wantExistingCount: 3,
		},
		{
			name:              "empty assembly name",
			assemblyShortName: "",
			className:         "MyNamespace.MyClass",
			existingFilenames: make(map[string]struct{}),
			want:              "MyClass.html",
			wantExistingCount: 1,
		},
		{
			name:              "empty class name (after processing)",
			assemblyShortName: "MyAssembly",
			className:         "MyNamespace.", // results in empty processedClassName
			existingFilenames: make(map[string]struct{}),
			want:              "MyAssembly.html", // baseName becomes just assemblyShortName
			wantExistingCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of existingFilenames if the test needs to check its modification
			// without affecting other subtests that might share the map if not careful.
			// For this structure, tt.existingFilenames is unique per subtest.
			got := generateUniqueFilename(tt.assemblyShortName, tt.className, tt.existingFilenames)
			if got != tt.want {
				t.Errorf("generateUniqueFilename() got = %v, want %v", got, tt.want)
			}
			if len(tt.existingFilenames) != tt.wantExistingCount {
				t.Errorf("generateUniqueFilename() modified existingFilenames to count %d, want %d. Map: %v", len(tt.existingFilenames), tt.wantExistingCount, tt.existingFilenames)
			}
			// Check if the generated filename (lowercase) is indeed in the map
			if _, ok := tt.existingFilenames[strings.ToLower(tt.want)]; !ok {
				t.Errorf("generateUniqueFilename() expected filename %s (lowercase) to be in existingFilenames map, but it was not. Map: %v", strings.ToLower(tt.want), tt.existingFilenames)
			}
		})
	}
}
func TestCountTotalClasses(t *testing.T) {
	tests := []struct {
		name       string
		assemblies []model.Assembly
		want       int
	}{
		{"no assemblies", []model.Assembly{}, 0},
		{"one assembly no classes", []model.Assembly{{Name: "A1"}}, 0},
		{
			"multiple assemblies with classes",
			[]model.Assembly{
				{Name: "A1", Classes: []model.Class{{Name: "C1"}, {Name: "C2"}}},
				{Name: "A2", Classes: []model.Class{{Name: "C3"}}},
			},
			3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countTotalClasses(tt.assemblies); got != tt.want {
				t.Errorf("countTotalClasses() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCountUniqueFiles(t *testing.T) {
	tests := []struct {
		name       string
		assemblies []model.Assembly
		want       int
	}{
		{"no assemblies", []model.Assembly{}, 0},
		{
			"single file",
			[]model.Assembly{{
				Classes: []model.Class{{
					Files: []model.CodeFile{{Path: "file1.cs"}},
				}},
			}},
			1,
		},
		{
			"multiple unique files",
			[]model.Assembly{{
				Classes: []model.Class{
					{Files: []model.CodeFile{{Path: "file1.cs"}}},
					{Files: []model.CodeFile{{Path: "file2.cs"}}},
				},
			}},
			2,
		},
		{
			"duplicate files across classes/assemblies",
			[]model.Assembly{
				{Classes: []model.Class{
					{Files: []model.CodeFile{{Path: "file1.cs"}}},
				}},
				{Classes: []model.Class{
					{Files: []model.CodeFile{{Path: "file1.cs"}}}, // Duplicate
					{Files: []model.CodeFile{{Path: "file2.cs"}}},
				}},
			},
			2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countUniqueFiles(tt.assemblies); got != tt.want {
				t.Errorf("countUniqueFiles() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetermineLineVisitStatus(t *testing.T) {
	tests := []struct {
		name            string
		hits            int
		isBranchPoint   bool
		coveredBranches int
		totalBranches   int
		want            model.LineVisitStatus
	}{
		// Non-branch points
		{"not coverable (negative hits)", -1, false, 0, 0, lineVisitStatusNotCoverable},
		{"covered (positive hits)", 10, false, 0, 0, lineVisitStatusCovered},
		{"not covered (zero hits)", 0, false, 0, 0, lineVisitStatusNotCovered},

		// Branch points
		{"branch, not coverable (negative hits)", -1, true, 0, 2, lineVisitStatusNotCoverable},
		{"branch, fully covered", 10, true, 2, 2, lineVisitStatusCovered},
		{"branch, partially covered", 5, true, 1, 2, lineVisitStatusPartiallyCovered},
		{"branch, not covered (zero hits on line, zero branches covered)", 0, true, 0, 2, lineVisitStatusNotCovered},
		{"branch, not covered (positive hits on line, but zero branches covered)", 5, true, 0, 2, lineVisitStatusNotCovered},
		{"branch, but no branches defined (totalBranches=0)", 1, true, 0, 0, lineVisitStatusNotCoverable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := determineLineVisitStatus(tt.hits, tt.isBranchPoint, tt.coveredBranches, tt.totalBranches); got != tt.want {
				t.Errorf("determineLineVisitStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLineVisitStatusToString(t *testing.T) {
	tests := []struct {
		name   string
		status model.LineVisitStatus
		want   string
	}{
		{"covered", lineVisitStatusCovered, "green"},
		{"not covered", lineVisitStatusNotCovered, "red"},
		{"partially covered", lineVisitStatusPartiallyCovered, "orange"},
		{"not coverable", lineVisitStatusNotCoverable, "gray"},
		{"unknown status defaults to gray", 99, "gray"}, // Test default case
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lineVisitStatusToString(tt.status); got != tt.want {
				t.Errorf("lineVisitStatusToString() = %v, want %v", got, tt.want)
			}
		})
	}
}
