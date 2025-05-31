package analyzer

import (
	"math"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/inputxml"

	// "github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/inputxml" // Removed duplicate
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
)

// calculateCrapScore calculates the CRAP score for a method.
// coverage is expected as a float between 0.0 and 1.0.
// complexity is the cyclomatic complexity.
func calculateCrapScore(coverage float64, complexity float64) float64 {
	if math.IsNaN(coverage) || math.IsInf(coverage, 0) || coverage < 0 || coverage > 1 {
		coverage = 0 // Treat invalid coverage as 0 for CRAP score calculation
	}
	if math.IsNaN(complexity) || math.IsInf(complexity, 0) || complexity < 0 {
		return math.NaN() // Invalid complexity
	}

	uncoveredRatio := 1.0 - coverage
	// CRAP = (complexity^2 * uncoveredRatio^3) + complexity
	crap := (math.Pow(complexity, 2) * math.Pow(uncoveredRatio, 3)) + complexity
	return crap
}

// processMethodXML processes an inputxml.MethodXML and transforms it into a model.Method.
// sourceLines are the lines of the source file this method belongs to.
func processMethodXML(methodXML inputxml.MethodXML, sourceLines []string) (*model.Method, error) {
	method := model.Method{
		// Original C# ReportGenerator uses the method name from XML directly here.
		// It then cleans it up (removes generics, etc.) for display in some places.
		// For the `Name` field that's used as a key or for raw lookup, keep it as is from XML.
		Name:       methodXML.Name,
		Signature:  methodXML.Signature,
		Complexity: parseFloat(methodXML.Complexity),
	}

	var methodLinesCovered, methodLinesValid int
	var methodBranchesCovered, methodBranchesValid int

	minLine := math.MaxInt32
	maxLine := 0

	for _, lineXML := range methodXML.Lines.Line {
		currentLineNum := parseInt(lineXML.Number)
		if currentLineNum < minLine {
			minLine = currentLineNum
		}
		if currentLineNum > maxLine {
			maxLine = currentLineNum
		}

		lineModel, lineMetrics := processLineXML(lineXML, sourceLines)
		method.Lines = append(method.Lines, lineModel)

		// Only count lines with hits >= 0 as coverable by definition of coverable lines in Cobertura
		if lineModel.Hits >= 0 { // Line is coverable
			methodLinesValid++
			if lineModel.Hits > 0 {
				methodLinesCovered++
			}
		}
		methodBranchesCovered += lineMetrics.branchesCovered
		methodBranchesValid += lineMetrics.branchesValid
	}

	if minLine == math.MaxInt32 {
		method.FirstLine = 0
	} else {
		method.FirstLine = minLine
	}
	method.LastLine = maxLine

	// Calculate line and branch rates
	if methodLinesValid > 0 {
		method.LineRate = float64(methodLinesCovered) / float64(methodLinesValid)
	} else {
		method.LineRate = 0 // Or NaN if you prefer, but 0 is simpler for display
	}

	if methodBranchesValid > 0 {
		method.BranchRate = float64(methodBranchesCovered) / float64(methodBranchesValid)
	} else {
		method.BranchRate = 0 // Or NaN
	}

	// --- Populate MethodMetrics for the standard set ---
	method.MethodMetrics = []model.MethodMetric{} // Initialize/clear

	// 1. Cyclomatic Complexity
	complexityValue := parseFloat(methodXML.Complexity) // Already parsed
	if !math.IsNaN(complexityValue) {                   // Only add if valid
		method.MethodMetrics = append(method.MethodMetrics, model.MethodMetric{
			Name: methodXML.Name, // Method name for grouping
			Line: method.FirstLine,
			Metrics: []model.Metric{
				{Name: "Cyclomatic complexity", Value: complexityValue, Status: model.StatusOk},
			},
		})
	}

	// 2. Line Coverage (as a metric)
	// Ensure method.LineRate is 0.0 to 1.0
	lineCoveragePercentage := method.LineRate * 100.0
	if !math.IsNaN(lineCoveragePercentage) {
		method.MethodMetrics = append(method.MethodMetrics, model.MethodMetric{
			Name: methodXML.Name,
			Line: method.FirstLine,
			Metrics: []model.Metric{
				{Name: "Line coverage", Value: lineCoveragePercentage, Status: model.StatusOk},
			},
		})
	}

	// 3. Branch Coverage (as a metric)
	// Ensure method.BranchRate is 0.0 to 1.0
	branchCoveragePercentage := method.BranchRate * 100.0
	if !math.IsNaN(branchCoveragePercentage) {
		method.MethodMetrics = append(method.MethodMetrics, model.MethodMetric{
			Name: methodXML.Name,
			Line: method.FirstLine,
			Metrics: []model.Metric{
				{Name: "Branch coverage", Value: branchCoveragePercentage, Status: model.StatusOk},
			},
		})
	}

	// 4. CrapScore
	// Use method.LineRate (0.0-1.0) for CrapScore calculation
	crapScoreValue := calculateCrapScore(method.LineRate, complexityValue)
	if !math.IsNaN(crapScoreValue) {
		method.MethodMetrics = append(method.MethodMetrics, model.MethodMetric{
			Name: methodXML.Name,
			Line: method.FirstLine,
			Metrics: []model.Metric{
				{Name: "CrapScore", Value: crapScoreValue, Status: model.StatusOk},
			},
		})
	}

	return &method, nil
}


