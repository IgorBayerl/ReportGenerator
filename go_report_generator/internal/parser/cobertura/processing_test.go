package cobertura

import (
	"fmt"
	"testing"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/inputxml"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser/filtering"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/settings"
)

// MockFileReader implements the FileReader interface for testing purposes.
type MockFileReader struct{}

func (mfr *MockFileReader) ReadFile(path string) ([]string, error) {
	// For these tests, we don't need actual source content.
	return []string{}, nil
}

func (mfr *MockFileReader) CountLines(path string) (int, error) {
	return 0, nil
}

// MockParserConfig implements the parser.ParserConfig interface for testing.
type MockParserConfig struct {
	TestSettings *settings.Settings
}

func (mpc *MockParserConfig) SourceDirectories() []string { return []string{} }
func (mpc *MockParserConfig) AssemblyFilters() filtering.IFilter {
	f, _ := filtering.NewDefaultFilter(nil)
	return f
}
func (mpc *MockParserConfig) ClassFilters() filtering.IFilter {
	f, _ := filtering.NewDefaultFilter(nil)
	return f
}
func (mpc *MockParserConfig) FileFilters() filtering.IFilter {
	f, _ := filtering.NewDefaultFilter(nil, true)
	return f
}
func (mpc *MockParserConfig) Settings() *settings.Settings { return mpc.TestSettings }

// newTestOrchestrator is a helper to create a pre-configured orchestrator for tests.
func newTestOrchestrator() *processingOrchestrator {
	mockConfig := &MockParserConfig{TestSettings: settings.NewSettings()}
	return newProcessingOrchestrator(&MockFileReader{}, mockConfig, nil, true)
}

func TestProcessMethodXML_BranchCoverage(t *testing.T) {
	testCases := []struct {
		name                   string
		supportsBranchCoverage bool // New field to simulate global context
		methodXML              inputxml.MethodXML
		expectedBranchRate     *float64 // Pointer to handle nil
	}{
		{
			name:                   "With branch support, method with no branches has 100% coverage",
			supportsBranchCoverage: true,
			methodXML: inputxml.MethodXML{
				Name:  "NoBranchMethod",
				Lines: inputxml.LinesXML{Line: []inputxml.LineXML{{Number: "1", Hits: "1", Branch: "false"}}},
			},
			expectedBranchRate: float64p(1.0), // Should be 100%
		},
		{
			name:                   "With branch support, method with branches has calculated coverage",
			supportsBranchCoverage: true,
			methodXML: inputxml.MethodXML{
				Name:  "PartialBranchMethod",
				Lines: inputxml.LinesXML{Line: []inputxml.LineXML{{Number: "1", Hits: "1", Branch: "true", ConditionCoverage: "50% (1/2)"}}},
			},
			expectedBranchRate: float64p(0.5), // 50%
		},
		{
			name:                   "Without branch support, method branch rate is nil (N/A)",
			supportsBranchCoverage: false,
			methodXML: inputxml.MethodXML{
				Name:  "NoBranchSupportMethod",
				Lines: inputxml.LinesXML{Line: []inputxml.LineXML{{Number: "1", Hits: "1", Branch: "false"}}},
			},
			expectedBranchRate: nil, // Should be N/A
		},
		{
			name:                   "Without branch support, even a method with branch data has nil rate",
			supportsBranchCoverage: false,
			methodXML: inputxml.MethodXML{
				Name:  "BranchDataIgnoredMethod",
				Lines: inputxml.LinesXML{Line: []inputxml.LineXML{{Number: "1", Hits: "1", Branch: "true", ConditionCoverage: "50% (1/2)"}}},
			},
			expectedBranchRate: nil, // Branch data is ignored if format doesn't support it
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create orchestrator with the specific context for this test case
			mockConfig := &MockParserConfig{TestSettings: settings.NewSettings()}
			orchestrator := newProcessingOrchestrator(&MockFileReader{}, mockConfig, nil, tc.supportsBranchCoverage)

			methodModel, err := orchestrator.processMethodXML(tc.methodXML, []string{}, "TestClass")

			if err != nil {
				t.Fatalf("processMethodXML returned an unexpected error: %v", err)
			}

			// Assertions
			if tc.expectedBranchRate == nil {
				if methodModel.BranchRate != nil {
					t.Errorf("Expected BranchRate to be nil, but got %v", *methodModel.BranchRate)
				}
			} else {
				if methodModel.BranchRate == nil {
					t.Errorf("Expected BranchRate to be %v, but got nil", *tc.expectedBranchRate)
				} else if *methodModel.BranchRate != *tc.expectedBranchRate {
					t.Errorf("Expected BranchRate to be %.2f, but got %.2f", *tc.expectedBranchRate, *methodModel.BranchRate)
				}
			}
		})
	}
}

// float64p is a helper function to create a pointer to a float64 literal.
func float64p(v float64) *float64 {
	return &v
}

func TestProcessLineXML(t *testing.T) {
	orchestrator := newTestOrchestrator()

	testCases := []struct {
		name                  string
		input                 inputxml.LineXML
		expectedCovered       int
		expectedTotal         int
		expectIsBranchPoint   bool
		expectedBranchDetails []model.BranchCoverageDetail
	}{
		{
			name: "Standard branch line (2/2)",
			input: inputxml.LineXML{
				Number:            "10",
				Hits:              "1",
				Branch:            "true",
				ConditionCoverage: "100% (2/2)",
			},
			expectedCovered:     2,
			expectedTotal:       2,
			expectIsBranchPoint: true,
			expectedBranchDetails: []model.BranchCoverageDetail{
				{Identifier: "10_0", Visits: 1},
				{Identifier: "10_1", Visits: 1},
			},
		},
		{
			name: "Partial branch line (1/2)",
			input: inputxml.LineXML{
				Number:            "20",
				Hits:              "1",
				Branch:            "true",
				ConditionCoverage: "50% (1/2)",
			},
			expectedCovered:     1,
			expectedTotal:       2,
			expectIsBranchPoint: true,
			expectedBranchDetails: []model.BranchCoverageDetail{
				{Identifier: "20_0", Visits: 1},
				{Identifier: "20_1", Visits: 0},
			},
		},
		{
			name: "No branch coverage (0/2)",
			input: inputxml.LineXML{
				Number:            "30",
				Hits:              "0",
				Branch:            "true",
				ConditionCoverage: "0% (0/2)",
			},
			expectedCovered:     0,
			expectedTotal:       2,
			expectIsBranchPoint: true,
			expectedBranchDetails: []model.BranchCoverageDetail{
				{Identifier: "30_0", Visits: 0},
				{Identifier: "30_1", Visits: 0},
			},
		},
		{
			name: "Branch line with conditions elements instead of attribute",
			input: inputxml.LineXML{
				Number: "40",
				Hits:   "1",
				Branch: "true",
				Conditions: inputxml.ConditionsXML{
					Condition: []inputxml.ConditionXML{
						{Number: "0", Type: "jump", Coverage: "100%"},
						{Number: "1", Type: "jump", Coverage: "0%"},
					},
				},
			},
			expectedCovered:     1,
			expectedTotal:       2,
			expectIsBranchPoint: true,
			expectedBranchDetails: []model.BranchCoverageDetail{
				{Identifier: "0", Visits: 1},
				{Identifier: "1", Visits: 0},
			},
		},
		{
			name: "Branch line with malformed condition-coverage (no panic)",
			input: inputxml.LineXML{
				Number:            "50",
				Hits:              "1",
				Branch:            "true",
				ConditionCoverage: "malformed",
			},
			expectedCovered:     1,
			expectedTotal:       1,
			expectIsBranchPoint: true,
			expectedBranchDetails: []model.BranchCoverageDetail{
				{Identifier: "50_0", Visits: 1},
			},
		},
		{
			name: "Non-branch line",
			input: inputxml.LineXML{
				Number: "60",
				Hits:   "3",
				Branch: "false",
			},
			expectedCovered:     0,
			expectedTotal:       0,
			expectIsBranchPoint: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			line, metrics := orchestrator.processLineXML(tc.input)

			if line.CoveredBranches != tc.expectedCovered {
				t.Errorf("CoveredBranches: got %d, want %d", line.CoveredBranches, tc.expectedCovered)
			}
			if line.TotalBranches != tc.expectedTotal {
				t.Errorf("TotalBranches: got %d, want %d", line.TotalBranches, tc.expectedTotal)
			}
			if line.IsBranchPoint != tc.expectIsBranchPoint {
				t.Errorf("IsBranchPoint: got %v, want %v", line.IsBranchPoint, tc.expectIsBranchPoint)
			}
			if metrics.branchesCovered != tc.expectedCovered {
				t.Errorf("Metrics covered: got %d, want %d", metrics.branchesCovered, tc.expectedCovered)
			}
			if metrics.branchesValid != tc.expectedTotal {
				t.Errorf("Metrics total: got %d, want %d", metrics.branchesValid, tc.expectedTotal)
			}

			// Use a more robust comparison for slices that ignores order
			if len(line.Branch) != len(tc.expectedBranchDetails) {
				t.Errorf("Branch details length mismatch: got %d, want %d", len(line.Branch), len(tc.expectedBranchDetails))
			} else {
				// Create maps for an order-independent comparison
				expectedMap := make(map[string]int)
				for _, bd := range tc.expectedBranchDetails {
					expectedMap[bd.Identifier] = bd.Visits
				}
				actualMap := make(map[string]int)
				for _, bd := range line.Branch {
					actualMap[bd.Identifier] = bd.Visits
				}
				if fmt.Sprintf("%v", expectedMap) != fmt.Sprintf("%v", actualMap) {
					t.Errorf("Branch details mismatch: got %v, want %v", actualMap, expectedMap)
				}
			}
		})
	}
}
