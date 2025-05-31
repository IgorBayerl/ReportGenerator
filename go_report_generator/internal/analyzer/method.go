package analyzer

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/inputxml"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/utils"

	// "github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/inputxml" // Removed duplicate
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
)

// -- Regexes for method name processing (ported from C# CoberturaParser) --

// lambdaMethodNameRegex helps identify lambda expressions.
// C# original: "<.+>.+__"
var lambdaMethodNameRegex = regexp.MustCompile(`<.+>.+__`)

// compilerGeneratedMethodNameRegex helps identify compiler-generated async/iterator MoveNext methods.
// C# original: (?<ClassName>.+)(/|\.)<(?<CompilerGeneratedName>.+)>.+__.+MoveNext\(\)$
// Go equivalent (using non-capturing group for separator):
var compilerGeneratedMethodNameRegex = regexp.MustCompile(`(?P<ClassName>.+)(?:/|\.)<(?P<CompilerGeneratedName>.+)>.*__.+MoveNext\(\)$`)

// localFunctionMethodNameRegex helps identify .NET local functions (nested methods).
// C# original: ^.*(?<ParentMethodName><.+>).*__(?<NestedMethodName>[^\|]+)\|.*$
// This is complex to port 1:1 due to Go's named group handling.
// This simplified regex aims to capture the essential part for display.
var localFunctionMethodNameRegex = regexp.MustCompile(`(?:.*<(?P<ParentMethodName>[^>]+)>g__)?(?P<NestedMethodName>[^|]+)\|`)

// -- Helper functions for method name processing --

// extractMethodName attempts to simplify compiler-generated method names (async, local functions)
// to a more human-readable form, similar to C# ReportGenerator's CoberturaParser.
// `methodNamePlusSignature` is the combined name and signature from the XML.
// `classNameFromXML` is the `name` attribute of the parent `<class>` XML tag, providing context.
func extractMethodName(methodNamePlusSignature, classNameFromXML string) string {
	combinedNameForContext := classNameFromXML + methodNamePlusSignature // Used for regexes needing class context

	// Handle .NET local functions (e.g., "ContainingClass.<ParentMethod>g__LocalFuncName|0_0(params)")
	// The C# regex is more complex. This Go version tries to extract the core local function name.
	if strings.Contains(methodNamePlusSignature, "|") && (strings.Contains(classNameFromXML, ">g__") || strings.Contains(methodNamePlusSignature, ">g__")) {
		match := localFunctionMethodNameRegex.FindStringSubmatch(combinedNameForContext)
		nameIndex := localFunctionMethodNameRegex.SubexpIndex("NestedMethodName")
		if len(match) > nameIndex && match[nameIndex] != "" {
			nestedName := match[nameIndex]
			if nestedName != "" {
				return nestedName + "()" // ALWAYS append "()" for local functions
			}
		}
	}

	// Handle async/iterator state machine methods (MoveNext)
	if strings.HasSuffix(methodNamePlusSignature, "MoveNext()") {
		match := compilerGeneratedMethodNameRegex.FindStringSubmatch(combinedNameForContext)
		// (?P<ClassName>.+)(?:/|\.)<(?P<CompilerGeneratedName>.+)>.*__.+MoveNext\(\)$
		// Index for CompilerGeneratedName
		nameIndex := compilerGeneratedMethodNameRegex.SubexpIndex("CompilerGeneratedName")
		if len(match) > nameIndex && match[nameIndex] != "" {
			return match[nameIndex] + "()" // Append "()" as C# does
		}
	}

	return methodNamePlusSignature // Return original if no specific transformation applied
}

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

// processMethodXML is the main orchestrator for processing a single method from XML.
// classNameFromXML is the 'name' attribute of the parent <class> XML tag, used for context in name extraction.
func processMethodXML(methodXML inputxml.MethodXML, sourceLines []string, classNameFromXML string) (*model.Method, error) {
	rawMethodName := methodXML.Name
	rawSignature := methodXML.Signature
	fullNameFromXML := rawMethodName + rawSignature

	// Apply C#-like extraction rules to get a cleaner/original method name for display or special handling
	extractedFullNameForDisplay := extractMethodName(fullNameFromXML, classNameFromXML)

	// Check for lambda pattern on the extracted name (as C# does before adding to MethodMetrics)
	if strings.Contains(extractedFullNameForDisplay, "__") && lambdaMethodNameRegex.MatchString(extractedFullNameForDisplay) {
		// This method is considered a lambda by C# logic and would be skipped for the metrics table.
		// We return an error to indicate it should be skipped by the caller.
		return nil, fmt.Errorf("method '%s' (extracted: '%s') is a lambda and skipped for metrics table representation", fullNameFromXML, extractedFullNameForDisplay)
	}

	method := model.Method{
		Name:        rawMethodName, // Store original name from XML for model.Method.Name
		Signature:   rawSignature,  // Store original signature for model.Method.Signature
		DisplayName: extractedFullNameForDisplay,
		Complexity:  utils.ParseFloat(methodXML.Complexity),
	}

	processMethodLines(methodXML, &method, sourceLines)
	// calculateMethodCoverageRates is now integrated into processMethodLines

	// Use the extracted/cleaned full name for grouping and naming in the metrics table.
	// The ShortName for display will be derived from this by the HTML builder.
	populateStandardMethodMetrics(&method, extractedFullNameForDisplay)

	return &method, nil
}

// processMethodLines processes the <line> elements within a <method> XML tag.
// It populates the method's Lines slice, calculates first/last line numbers, and coverage rates.
func processMethodLines(methodXML inputxml.MethodXML, method *model.Method, sourceLines []string) {
	minLine := math.MaxInt32
	maxLine := 0
	var methodLinesCovered, methodLinesValid int
	var methodBranchesCovered, methodBranchesValid int

	for _, lineXML := range methodXML.Lines.Line {
		currentLineNum := utils.ParseInt(lineXML.Number, 0)
		if currentLineNum < minLine {
			minLine = currentLineNum
		}
		if currentLineNum > maxLine {
			maxLine = currentLineNum
		}

		lineModel, lineMetricsStats := processLineXML(lineXML, sourceLines) // processLineXML is in line.go
		method.Lines = append(method.Lines, lineModel)

		if lineModel.Hits >= 0 { // Line is coverable
			methodLinesValid++
			if lineModel.Hits > 0 {
				methodLinesCovered++
			}
		}
		methodBranchesCovered += lineMetricsStats.branchesCovered
		methodBranchesValid += lineMetricsStats.branchesValid
	}

	if minLine == math.MaxInt32 { // No lines processed
		method.FirstLine = 0
		method.LastLine = 0
	} else {
		method.FirstLine = minLine
		method.LastLine = maxLine
	}

	method.LineRate = 0.0 // Default
	if methodLinesValid > 0 {
		method.LineRate = float64(methodLinesCovered) / float64(methodLinesValid)
	}

	method.BranchRate = 0.0 // Default
	if methodBranchesValid > 0 {
		method.BranchRate = float64(methodBranchesCovered) / float64(methodBranchesValid)
	}
}

// populateStandardMethodMetrics populates the standard set of metrics (Complexity, Coverage, CrapScore)
// for the given method. metricGroupNameForTable is the (potentially cleaned) name to use for this method
// in the metrics table.
func populateStandardMethodMetrics(method *model.Method, metricGroupNameForTable string) {
	method.MethodMetrics = []model.MethodMetric{} // Initialize/clear

	// For MethodMetric.Name, we need a name that's good for display and grouping.
	// The C# `MethodMetric` has `FullName` and `ShortName`.
	// `metricGroupNameForTable` is the `extractedFullName` from `processMethodXML`.
	shortMetricName := utils.GetShortMethodName(metricGroupNameForTable)

	// 1. Cyclomatic Complexity
	if !math.IsNaN(method.Complexity) {
		method.MethodMetrics = append(method.MethodMetrics, model.MethodMetric{
			Name: shortMetricName, // Use the short, clean name for the metric entry
			Line: method.FirstLine,
			Metrics: []model.Metric{
				{Name: "Cyclomatic complexity", Value: method.Complexity, Status: model.StatusOk},
			},
		})
	}

	// 2. Line Coverage (as a metric)
	lineCoveragePercentage := method.LineRate * 100.0
	if !math.IsNaN(lineCoveragePercentage) {
		method.MethodMetrics = append(method.MethodMetrics, model.MethodMetric{
			Name: shortMetricName,
			Line: method.FirstLine,
			Metrics: []model.Metric{
				{Name: "Line coverage", Value: lineCoveragePercentage, Status: model.StatusOk},
			},
		})
	}

	// 3. Branch Coverage (as a metric)
	branchCoveragePercentage := method.BranchRate * 100.0
	if !math.IsNaN(branchCoveragePercentage) {
		method.MethodMetrics = append(method.MethodMetrics, model.MethodMetric{
			Name: shortMetricName,
			Line: method.FirstLine,
			Metrics: []model.Metric{
				{Name: "Branch coverage", Value: branchCoveragePercentage, Status: model.StatusOk},
			},
		})
	}

	// 4. CrapScore
	crapScoreValue := calculateCrapScore(method.LineRate, method.Complexity)
	if !math.IsNaN(crapScoreValue) {
		method.MethodMetrics = append(method.MethodMetrics, model.MethodMetric{
			Name: shortMetricName,
			Line: method.FirstLine,
			Metrics: []model.Metric{
				{Name: "CrapScore", Value: crapScoreValue, Status: model.StatusOk},
			},
		})
	}
}
