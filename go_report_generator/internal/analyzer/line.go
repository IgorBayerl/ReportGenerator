// File: internal/analyzer/line.go
package analyzer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/inputxml"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
)

// conditionCoverageRegex matches the (covered/total) part of Cobertura's condition-coverage attribute,
// using named capture groups similar to C#.
// C# equivalent: private static readonly Regex BranchCoverageRegex = new Regex("\\((?<NumberOfCoveredBranches>\\d+)/(?<NumberOfTotalBranches>\\d+)\\)$", RegexOptions.Compiled);
var conditionCoverageRegex = regexp.MustCompile(`\((?P<NumberOfCoveredBranches>\d+)/(?P<NumberOfTotalBranches>\d+)\)$`)

// lineProcessingMetrics holds branch metrics for a single line.
type lineProcessingMetrics struct {
	branchesCovered int
	branchesValid   int
}

// processLineXML transforms an inputxml.LineXML into a model.Line and its associated metrics.
// sourceLines contains all lines from the source file this XML line belongs to.
func processLineXML(lineXML inputxml.LineXML, sourceLines []string) (model.Line, lineProcessingMetrics) {
	metrics := lineProcessingMetrics{}
	lineNumber := parseInt(lineXML.Number) // Assuming parseInt is defined elsewhere (e.g., analyzer.go)

	line := model.Line{
		Number:            lineNumber,
		Hits:              parseInt(lineXML.Hits),
		IsBranchPoint:     (lineXML.Branch == "true"),
		ConditionCoverage: lineXML.ConditionCoverage, // Store original string
		Branch:            make([]model.BranchCoverageDetail, 0), // Initialize as empty slice
	}

	if lineNumber > 0 && lineNumber <= len(sourceLines) {
		line.Content = sourceLines[lineNumber-1]
	} else {
		line.Content = ""
	}

	hasDetailedConditions := len(lineXML.Conditions.Condition) > 0

	if hasDetailedConditions {
		for _, conditionXML := range lineXML.Conditions.Condition {
			branchDetail := model.BranchCoverageDetail{
				Identifier: conditionXML.Number,
				Visits:     0,
			}

			if strings.HasPrefix(conditionXML.Coverage, "100") {
				branchDetail.Visits = 1
			}
			line.Branch = append(line.Branch, branchDetail)

			if branchDetail.Visits > 0 {
				line.CoveredBranches++
			}
			line.TotalBranches++
		}
	} else if line.IsBranchPoint { // lineXML.Branch == "true" but no <conditions>
		conditionCoverageAttr := lineXML.ConditionCoverage
		matches := conditionCoverageRegex.FindStringSubmatch(conditionCoverageAttr)

		if len(matches) > 0 { // Check if there was a match (FindStringSubmatch returns empty slice if no match)
			groupNames := conditionCoverageRegex.SubexpNames()
			var coveredStr, totalStr string

			for i, name := range groupNames {
				if i > 0 && name != "" { // i > 0 to skip the full match
					if name == "NumberOfCoveredBranches" {
						coveredStr = matches[i]
					} else if name == "NumberOfTotalBranches" {
						totalStr = matches[i]
					}
				}
			}

			if coveredStr != "" && totalStr != "" {
				numberOfCoveredBranches, errC := strconv.Atoi(coveredStr)
				numberOfTotalBranches, errT := strconv.Atoi(totalStr)

				if errC == nil && errT == nil && numberOfTotalBranches > 0 {
					line.CoveredBranches = numberOfCoveredBranches
					line.TotalBranches = numberOfTotalBranches

					for i := 0; i < numberOfTotalBranches; i++ {
						branchIdentifier := fmt.Sprintf("%d_%d", lineNumber, i)
						visits := 0
						if i < numberOfCoveredBranches {
							visits = 1 // Mark as covered
						}
						line.Branch = append(line.Branch, model.BranchCoverageDetail{
							Identifier: branchIdentifier,
							Visits:     visits,
						})
					}
				} else { // Fallback if regex matched but numbers were invalid or total was 0
					setFallbackBranchData(&line)
				}
			} else { // Fallback if named groups weren't found in the match (should not happen if regex is correct and matches)
				setFallbackBranchData(&line)
			}
		} else { // Fallback if regex did not match at all
			setFallbackBranchData(&line)
		}
	}
	// If !line.IsBranchPoint, CoveredBranches and TotalBranches will remain 0 from initialization.

	metrics.branchesCovered = line.CoveredBranches
	metrics.branchesValid = line.TotalBranches

	return line, metrics
}

// setFallbackBranchData is a helper to set default branch info when detailed parsing fails.
func setFallbackBranchData(line *model.Line) {
	if line.Hits > 0 {
		line.CoveredBranches = 1
	} else {
		line.CoveredBranches = 0
	}
	line.TotalBranches = 1
	line.Branch = append(line.Branch, model.BranchCoverageDetail{
		Identifier: fmt.Sprintf("%d_0", line.Number), // Line number and a default branch index
		Visits:     line.CoveredBranches,
	})
}