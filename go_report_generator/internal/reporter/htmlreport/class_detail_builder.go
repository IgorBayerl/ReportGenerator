package htmlreport

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/filereader"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
)

func (b *HtmlReportBuilder) generateClassDetailHTML(classModel *model.Class, classReportFilename string, tag string) error {
	// 1. Build the main ClassViewModelForDetail (server-side rendering focus)
	classVM := b.buildClassViewModelForDetailServer(classModel, tag)

	// 2. Build the AngularClassDetailViewModel (for client-side window.classDetails JSON)
	angularClassDetailForJS, err := b.buildAngularClassDetailForJS(classModel, &classVM)
	if err != nil {
		return fmt.Errorf("failed to build Angular class detail JSON for %s: %w", classModel.DisplayName, err)
	}
	classDetailJSONBytes, err := json.Marshal(angularClassDetailForJS)
	if err != nil {
		return fmt.Errorf("failed to marshal Angular class detail JSON for %s: %w", classModel.DisplayName, err)
	}

	// 3. Prepare overall data for the template
	templateData := b.buildClassDetailPageData(classVM, tag, template.JS(classDetailJSONBytes))

	// 4. Render the template
	return b.renderClassDetailPage(templateData, classReportFilename)

}

func (b *HtmlReportBuilder) buildClassViewModelForDetailServer(classModel *model.Class, tag string) ClassViewModelForDetail {
	cvm := ClassViewModelForDetail{
		Name:         classModel.DisplayName,
		AssemblyName: classModel.Name, // Keep full name for Assembly, DisplayName is for Class title
		IsMultiFile:  len(classModel.Files) > 1,
	}
	if dotIndex := strings.LastIndex(classModel.Name, "."); dotIndex > -1 && dotIndex < len(classModel.Name)-1 {
		// This might be a simplification. C# RG likely gets assembly name from model.Assembly.
		// For now, assuming classModel.Name is like "Namespace.Possibly.AssemblyName.ClassName"
		// and we want "Namespace.Possibly.AssemblyName"
		cvm.AssemblyName = classModel.Name[:dotIndex] // This might be incorrect if class name itself has dots.
		// Let's assume the AssemblyName is derived from the parent model.Assembly.Name when processing.
		// For now, this placeholder uses the full class name which is not ideal.
		// This should be: if classModel has an AssemblyName field, use that.
		// Or, pass assemblyName string to this function.
		// For now, will use classModel.Name as placeholder as in original logic for DisplayName
	}

	cvm.CoveredLines = classModel.LinesCovered
	cvm.CoverableLines = classModel.LinesValid
	cvm.UncoveredLines = cvm.CoverableLines - cvm.CoveredLines
	cvm.TotalLines = classModel.TotalLines

	b.populateLineCoverageMetricsForClassVM(&cvm, classModel)
	b.populateBranchCoverageMetricsForClassVM(&cvm, classModel)
	b.populateMethodCoverageMetricsForClassVM(&cvm, classModel)
	b.populateHistoricCoveragesForClassVM(&cvm, classModel)
	b.populateAggregatedMetricsForClassVM(&cvm, classModel) // Class-level aggregated metrics

	var allMethodMetricsForClass []*model.MethodMetric
	for fileIdx, fileInClass := range classModel.Files {
		fileVM, sourceLines, err := b.buildFileViewModelForServerRender(&fileInClass)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not build file view model for %s: %v\n", fileInClass.Path, err)
			continue // Skip this file
		}

		for i := range fileInClass.MethodMetrics { // Collect all for the class
			allMethodMetricsForClass = append(allMethodMetricsForClass, &fileInClass.MethodMetrics[i])
		}
		for _, codeElem := range fileInClass.CodeElements {
			sidebarElem := b.buildSidebarElementViewModel(&codeElem, fileVM.ShortPath, fileIdx+1, len(classModel.Files) > 1)
			cvm.SidebarElements = append(cvm.SidebarElements, sidebarElem)
		}
		// The original code had a bug where it re-read sourceFileLines for each file,
		// this function now uses the sourceLines returned by buildFileViewModelForServerRender
		cvm.Files = append(cvm.Files, fileVM)
		_ = sourceLines // To use if lines were not part of FileViewModel, but they are
	}

	if len(allMethodMetricsForClass) > 0 {
		cvm.FilesWithMetrics = true
		cvm.MetricsTable = b.buildMetricsTableForClassVM(classModel, allMethodMetricsForClass)
	}

	return cvm

}

func (b *HtmlReportBuilder) populateLineCoverageMetricsForClassVM(cvm *ClassViewModelForDetail, classModel *model.Class) {
	if cvm.CoverableLines > 0 {
		lineCoverage := (float64(cvm.CoveredLines) / float64(cvm.CoverableLines)) * 100.0
		cvm.CoveragePercentageForDisplay = fmt.Sprintf("%.1f%%", lineCoverage)
		cvm.CoveragePercentageBarValue = 100 - int(math.Round(lineCoverage))
		cvm.CoverageRatioTextForDisplay = fmt.Sprintf("%d of %d", cvm.CoveredLines, cvm.CoverableLines)
	} else {
		cvm.CoveragePercentageForDisplay = "N/A"
		cvm.CoveragePercentageBarValue = 0 // Avoid NaN issues with Round
		cvm.CoverageRatioTextForDisplay = "-"
	}
}

func (b *HtmlReportBuilder) populateBranchCoverageMetricsForClassVM(cvm *ClassViewModelForDetail, classModel *model.Class) {
	if b.branchCoverageAvailable && classModel.BranchesValid != nil && *classModel.BranchesValid > 0 && classModel.BranchesCovered != nil {
		cvm.CoveredBranches = *classModel.BranchesCovered
		cvm.TotalBranches = *classModel.BranchesValid
		branchCoverage := (float64(cvm.CoveredBranches) / float64(cvm.TotalBranches)) * 100.0
		cvm.BranchCoveragePercentageForDisplay = fmt.Sprintf("%.1f%%", branchCoverage)
		cvm.BranchCoveragePercentageBarValue = 100 - int(math.Round(branchCoverage))
		cvm.BranchCoverageRatioTextForDisplay = fmt.Sprintf("%d of %d", cvm.CoveredBranches, cvm.TotalBranches)
	} else {
		cvm.BranchCoveragePercentageForDisplay = "N/A"
		cvm.BranchCoveragePercentageBarValue = 0
		cvm.BranchCoverageRatioTextForDisplay = "-"
	}
}

func (b *HtmlReportBuilder) populateMethodCoverageMetricsForClassVM(cvm *ClassViewModelForDetail, classModel *model.Class) {
	cvm.TotalMethods = classModel.TotalMethods               // Use pre-calculated from model.Class
	cvm.CoveredMethods = classModel.CoveredMethods           // Use pre-calculated
	cvm.FullyCoveredMethods = classModel.FullyCoveredMethods // Use pre-calculated

	if cvm.TotalMethods > 0 {
		methodCov := (float64(cvm.CoveredMethods) / float64(cvm.TotalMethods)) * 100.0
		fullMethodCovVal := (float64(cvm.FullyCoveredMethods) / float64(cvm.TotalMethods)) * 100.0
		cvm.MethodCoveragePercentageForDisplay = fmt.Sprintf("%.1f%%", methodCov)
		cvm.MethodCoveragePercentageBarValue = 100 - int(math.Round(methodCov))
		cvm.MethodCoverageRatioTextForDisplay = fmt.Sprintf("%d of %d", cvm.CoveredMethods, cvm.TotalMethods)
		cvm.FullMethodCoveragePercentageForDisplay = fmt.Sprintf("%.1f%%", fullMethodCovVal)
		cvm.FullMethodCoverageRatioTextForDisplay = fmt.Sprintf("%d of %d", cvm.FullyCoveredMethods, cvm.TotalMethods)
	} else {
		cvm.MethodCoveragePercentageForDisplay = "N/A"
		cvm.MethodCoveragePercentageBarValue = 0
		cvm.MethodCoverageRatioTextForDisplay = "-"
		cvm.FullMethodCoveragePercentageForDisplay = "N/A"
		cvm.FullMethodCoverageRatioTextForDisplay = "-"
	}

}

func (b *HtmlReportBuilder) populateHistoricCoveragesForClassVM(cvm *ClassViewModelForDetail, classModel *model.Class) {
	if classModel.HistoricCoverages == nil {
		return
	}
	for _, hist := range classModel.HistoricCoverages {
		angularHist := b.buildAngularHistoricCoverageViewModel(&hist) // Re-use for consistency
		cvm.HistoricCoverages = append(cvm.HistoricCoverages, angularHist)
		if angularHist.LineCoverageQuota >= 0 {
			cvm.LineCoverageHistory = append(cvm.LineCoverageHistory, angularHist.LineCoverageQuota)
		}
		if angularHist.BranchCoverageQuota >= 0 {
			cvm.BranchCoverageHistory = append(cvm.BranchCoverageHistory, angularHist.BranchCoverageQuota)
		}
	}
}

func (b *HtmlReportBuilder) populateAggregatedMetricsForClassVM(cvm *ClassViewModelForDetail, classModel *model.Class) {
	// Assumes model.Class.Metrics is already populated with aggregated metrics
	cvm.Metrics = make(map[string]float64)
	for name, val := range classModel.Metrics {
		cvm.Metrics[name] = val
	}
}

func (b *HtmlReportBuilder) buildFileViewModelForServerRender(fileInClass *model.CodeFile) (FileViewModelForDetail, []string, error) {
	fileVM := FileViewModelForDetail{
		Path:      fileInClass.Path,
		ShortPath: sanitizeFilenameChars.ReplaceAllString(filepath.Base(fileInClass.Path), "_"),
	}
	sourceLines, err := filereader.ReadLinesInFile(fileInClass.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read source file %s: %v\n", fileInClass.Path, err)
		// Return an empty FileViewModel or handle error appropriately
		// For now, we'll proceed with empty source lines if read fails, lines will be empty.
		sourceLines = []string{}
	}

	coverageLinesMap := make(map[int]*model.Line)
	for i := range fileInClass.Lines { // Make sure to use index to get pointer
		covLine := &fileInClass.Lines[i]
		coverageLinesMap[covLine.Number] = covLine
	}

	for lineNumIdx, lineContent := range sourceLines {
		actualLineNumber := lineNumIdx + 1
		modelCovLine, hasCoverageData := coverageLinesMap[actualLineNumber]
		lineVM := b.buildLineViewModelForServerRender(lineContent, actualLineNumber, modelCovLine, hasCoverageData)
		fileVM.Lines = append(fileVM.Lines, lineVM)
	}
	return fileVM, sourceLines, nil // Return sourceLines in case they are needed by caller

}

func (b *HtmlReportBuilder) buildLineViewModelForServerRender(lineContent string, actualLineNumber int, modelCovLine *model.Line, hasCoverageData bool) LineViewModelForDetail {
	lineVM := LineViewModelForDetail{LineNumber: actualLineNumber, LineContent: lineContent}
	dataCoverageMap := map[string]map[string]string{"AllTestMethods": {"VC": "", "LVS": "gray"}} // Default

	if hasCoverageData {
		lineVM.Hits = fmt.Sprintf("%d", modelCovLine.Hits)
		status := determineLineVisitStatus(modelCovLine.Hits, modelCovLine.IsBranchPoint, modelCovLine.CoveredBranches, modelCovLine.TotalBranches)
		lineVM.LineVisitStatus = lineVisitStatusToString(status)
		if modelCovLine.IsBranchPoint && modelCovLine.TotalBranches > 0 {
			lineVM.IsBranch = true
			branchCoverageVal := (float64(modelCovLine.CoveredBranches) / float64(modelCovLine.TotalBranches)) * 100.0
			lineVM.BranchBarValue = 100 - int(math.Round(branchCoverageVal))
		}
		dataCoverageMap["AllTestMethods"]["VC"] = fmt.Sprintf("%d", modelCovLine.Hits)
		dataCoverageMap["AllTestMethods"]["LVS"] = lineVM.LineVisitStatus
		tooltipBranchRate := ""
		if lineVM.IsBranch {
			tooltipBranchRate = fmt.Sprintf(", %d of %d branches are covered", modelCovLine.CoveredBranches, modelCovLine.TotalBranches)
		}
		switch status {
		case lineVisitStatusCovered:
			lineVM.Tooltip = fmt.Sprintf("Covered (%d visits%s)", modelCovLine.Hits, tooltipBranchRate)
		case lineVisitStatusNotCovered:
			lineVM.Tooltip = fmt.Sprintf("Not covered (%d visits%s)", modelCovLine.Hits, tooltipBranchRate)
		case lineVisitStatusPartiallyCovered:
			lineVM.Tooltip = fmt.Sprintf("Partially covered (%d visits%s)", modelCovLine.Hits, tooltipBranchRate)
		default: // lineVisitStatusNotCoverable
			lineVM.Tooltip = "Not coverable"
		}
	} else {
		lineVM.LineVisitStatus = lineVisitStatusToString(lineVisitStatusNotCoverable)
		lineVM.Hits = ""
		lineVM.Tooltip = "Not coverable"
	}
	dataCoverageBytes, _ := json.Marshal(dataCoverageMap) // Error handling for marshal can be added
	lineVM.DataCoverage = template.JS(dataCoverageBytes)
	return lineVM

}

func (b *HtmlReportBuilder) buildSidebarElementViewModel(codeElem *model.CodeElement, fileShortPath string, fileIndexPlus1 int, isMultiFile bool) SidebarElementViewModel {
	sidebarElem := SidebarElementViewModel{
		Name:          codeElem.Name,
		FileShortPath: fileShortPath,
		Line:          codeElem.FirstLine,
		Icon:          "cube", // Default to method
	}
	if isMultiFile { // Only set FileIndexPlus1 if it's a multi-file class
		sidebarElem.FileIndexPlus1 = fileIndexPlus1
	}
	if codeElem.Type == model.PropertyElementType {
		sidebarElem.Icon = "wrench"
	}
	if codeElem.CoverageQuota != nil {
		sidebarElem.CoverageBarValue = int(math.Round(*codeElem.CoverageQuota))
		sidebarElem.CoverageTitle = fmt.Sprintf("Line coverage: %.1f%%", *codeElem.CoverageQuota)
	} else {
		sidebarElem.CoverageBarValue = -1 // For N/A items
		sidebarElem.CoverageTitle = "Line coverage: N/A"
	}
	return sidebarElem
}

// getStandardMetricHeaders defines the order and names of metrics for the table.
func (b *HtmlReportBuilder) getStandardMetricHeaders() []AngularMetricDefinitionViewModel {
	standardMetricKeys := []string{ // Use non-translated keys here for logic
		"Branch coverage",
		"CrapScore",
		"Cyclomatic complexity",
		"Line coverage",
	}

	var headers []AngularMetricDefinitionViewModel
	for _, key := range standardMetricKeys {
		translatedName := b.translations[key]
		if translatedName == "" {
			translatedName = key // Fallback to key if translation missing
		}
		headers = append(headers, AngularMetricDefinitionViewModel{
			Name:           translatedName,
			ExplanationURL: b.getMetricExplanationURL(key), // Use key for explanation URL
		})
	}
	return headers
}

// findCorrespondingCodeElement links a model.Method to its model.CodeElement.
// This is crucial for getting display names, proper links, and coverage quotas.
func findCorrespondingCodeElement(method *model.Method, classModel *model.Class) (*model.CodeElement, string, int) {
	for fIdx, f := range classModel.Files {
		for _, ce := range f.CodeElements {
			// Matching criteria:
			// 1. FirstLine must match.
			// 2. The CodeElement's FullName should ideally be method.Name + method.Signature
			//    or CodeElement.Name is method.Name (if signature is not part of ce.Name).
			//    The C# version has various ways these names are constructed.
			//    A robust match might require ce.FullName == (method.Name + method.Signature)
			//    For now, matching FirstLine and the base name (method.Name)
			if ce.FirstLine == method.FirstLine && (ce.Name == method.Name || strings.HasPrefix(ce.FullName, method.Name+method.Signature)) {
				fileShortPath := sanitizeFilenameChars.ReplaceAllString(filepath.Base(f.Path), "_")
				return &ce, fileShortPath, fIdx + 1
			}
		}
	}
	return nil, "", 0
}

// buildSingleMetricRow creates one AngularMethodMetricsViewModel for a method.
func (b *HtmlReportBuilder) buildSingleMetricRow(
	method *model.Method,
	correspondingCE *model.CodeElement,
	fileShortPath string,
	fileIndexPlus1 int,
	headers []AngularMetricDefinitionViewModel,
) AngularMethodMetricsViewModel {

	var displayName, fullNameForTitle string
	var lineToLink int
	var isProperty bool
	var coverageQuota *float64

	if correspondingCE != nil {
		// Use CodeElement's Name for display, which is typically the method name without full signature.
		// The original report seems to use the CodeElement's name and then appends the signature
		// if the signature is not "()" and not already part of the CodeElement's name.
		displayName = correspondingCE.Name
		if method.Signature != "" && method.Signature != "()" {
			// Check if signature is already somewhat represented in displayName
			// This is tricky because ce.Name might be "MyMethod" or "MyMethod()"
			// and method.Signature might be "(System.String)"
			if !strings.Contains(displayName, "(") { // If no parens in ce.Name, append full signature
				displayName += method.Signature
			} else if strings.HasSuffix(displayName, "()") { // If ce.Name is "MyMethod()", replace "()" with actual signature
				displayName = strings.TrimSuffix(displayName, "()") + method.Signature
			}
			// If ce.Name already contains a signature-like part, we might leave it,
			// or try to be smarter. For now, this is a common case.
		}

		fullNameForTitle = correspondingCE.FullName // This should be the most complete name
		lineToLink = correspondingCE.FirstLine
		isProperty = (correspondingCE.Type == model.PropertyElementType)
		coverageQuota = correspondingCE.CoverageQuota
	} else {
		// Fallback if CodeElement wasn't found
		displayName = method.Name // Raw XML name
		if method.Signature != "" && method.Signature != "()" {
			displayName += method.Signature
		}
		fullNameForTitle = displayName // Best guess for full name
		lineToLink = method.FirstLine
		isProperty = strings.HasPrefix(method.Name, "get_") || strings.HasPrefix(method.Name, "set_")
	}

	row := AngularMethodMetricsViewModel{
		Name:           displayName,
		FullName:       fullNameForTitle,
		FileIndexPlus1: fileIndexPlus1,
		Line:           lineToLink,
		FileShortPath:  fileShortPath,
		IsProperty:     isProperty,
		CoverageQuota:  coverageQuota,
		MetricValues:   make([]string, len(headers)),
	}

	// Populate metric values (logic remains the same as previous correct version)
	methodMetricsMap := make(map[string]model.Metric)
	for _, mm := range method.MethodMetrics {
		for _, m := range mm.Metrics {
			methodMetricsMap[m.Name] = m
		}
	}

	for i, headerVM := range headers {
		var originalMetricKey string
		// Simplified reverse lookup for known standard keys (assuming Option A for viewmodel is not yet implemented)
		switch headerVM.Name {
		case b.translations["Branch coverage"]:
			originalMetricKey = "Branch coverage"
		case b.translations["CrapScore"]:
			originalMetricKey = "CrapScore"
		case b.translations["Cyclomatic complexity"]:
			originalMetricKey = "Cyclomatic complexity"
		case b.translations["Line coverage"]:
			originalMetricKey = "Line coverage"
		default: // Fallback if not one of the standard translated ones, or translation matches key
			originalMetricKey = headerVM.Name // This assumes headerVM.Name is the key if not found in above cases
			// A more robust reverse lookup might still be needed if translations are complex
			// or if you implement Option A for AngularMetricDefinitionViewModel.OriginalKey
			if _, ok := methodMetricsMap[originalMetricKey]; !ok { // If direct match fails, iterate translations
				for key, translatedVal := range b.translations {
					if translatedVal == headerVM.Name && (key == "Branch coverage" || key == "CrapScore" || key == "Cyclomatic complexity" || key == "Line coverage" || key == "Complexity") {
						originalMetricKey = key
						break
					}
				}
			}
		}

		if metric, ok := methodMetricsMap[originalMetricKey]; ok {
			row.MetricValues[i] = b.formatMetricValue(metric)
		} else if originalMetricKey == "Cyclomatic complexity" {
			if cxMetric, cxOk := methodMetricsMap["Complexity"]; cxOk {
				row.MetricValues[i] = b.formatMetricValue(cxMetric)
			} else {
				row.MetricValues[i] = "-"
			}
		} else {
			row.MetricValues[i] = "-"
		}
	}
	return row
}

func (b *HtmlReportBuilder) buildMetricsTableForClassVM(classModel *model.Class, _ []*model.MethodMetric) MetricsTableViewModel {
	metricsTable := MetricsTableViewModel{}
	metricsTable.Headers = b.getStandardMetricHeaders()

	if len(classModel.Methods) == 0 {
		return metricsTable
	}

	// Create a temporary slice of methods with their CodeElement and file context
	// to preserve the original order from classModel.Methods if CodeElements are correctly ordered.
	// The order of methods in classModel.Methods should ideally reflect source order.
	// The order of CodeElements in classModel.Files[...].CodeElements should also reflect source order.

	// The critical part is that `classModel.Methods` should already be in source order.
	// If they are, then iterating through them directly will maintain that order.
	// The challenge is ensuring the `correspondingCE` is found reliably.

	for _, method := range classModel.Methods {
		correspondingCE, fileShortPath, fileIndexPlus1 := findCorrespondingCodeElement(&method, classModel)

		if correspondingCE == nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not find corresponding CodeElement for method %s (line %d) in class %s for metrics table. Using fallback naming.\n", method.Name+method.Signature, method.FirstLine, classModel.DisplayName)
			// If CE is not found, it might affect linking and precise display name,
			// but we still want to show the metrics for the method.
		}

		row := b.buildSingleMetricRow(&method, correspondingCE, fileShortPath, fileIndexPlus1, metricsTable.Headers)
		metricsTable.Rows = append(metricsTable.Rows, row)
	}

	return metricsTable
}

// getMetricExplanationURL (ensure keys match those in getStandardMetricHeaders)
func (b *HtmlReportBuilder) getMetricExplanationURL(metricKey string) string {
	switch metricKey { // Use the non-translated key
	case "Cyclomatic complexity", "Complexity":
		return "https://en.wikipedia.org/wiki/Cyclomatic_complexity"
	case "CrapScore":
		return "https://testing.googleblog.com/2011/02/this-code-is-crap.html"
	case "Line coverage", "Branch coverage":
		return "https://en.wikipedia.org/wiki/Code_coverage"
	default:
		return ""
	}
}

// formatMetricValue (ensure metric.Name matches the keys used in method.MethodMetrics)
func (b *HtmlReportBuilder) formatMetricValue(metric model.Metric) string {
	if metric.Value == nil {
		return "-"
	}
	// ... (rest of your formatMetricValue function, it looks mostly okay)
	// Ensure it handles the precision from settings for coverage values correctly.
	valFloat, isFloat := metric.Value.(float64)
	if !isFloat {
		if valInt, isInt := metric.Value.(int); isInt {
			return fmt.Sprintf("%d", valInt)
		}
		return fmt.Sprintf("%v", metric.Value)
	}

	if math.IsNaN(valFloat) {
		return "NaN"
	}
	if math.IsInf(valFloat, 0) {
		return "Inf"
	}

	precision := b.ReportContext.Settings().MaximumDecimalPlacesForCoverageQuotas
	formatString := fmt.Sprintf("%%.%df", precision)

	switch metric.Name { // Use the non-translated key
	case "Line coverage", "Branch coverage":
		return fmt.Sprintf(formatString+"%%", valFloat)
	case "CrapScore":
		return fmt.Sprintf("%.2f", valFloat) // CrapScore often displayed with 2 decimal places
	case "Cyclomatic complexity", "Complexity":
		return fmt.Sprintf("%.0f", valFloat) // Typically an integer
	default:
		return fmt.Sprintf(formatString, valFloat)
	}
}

func (b *HtmlReportBuilder) buildAngularClassDetailForJS(classModel *model.Class, classVMServer *ClassViewModelForDetail) (AngularClassDetailViewModel, error) {
	// Basic class info from server view model or re-calculate if necessary
	angularClassVMForJS := AngularClassViewModel{
		Name:                  classModel.DisplayName,
		CoveredLines:          classModel.LinesCovered,
		UncoveredLines:        classModel.LinesValid - classModel.LinesCovered,
		CoverableLines:        classModel.LinesValid,
		TotalLines:            classModel.TotalLines,
		CoveredMethods:        classVMServer.CoveredMethods,
		FullyCoveredMethods:   classVMServer.FullyCoveredMethods,
		TotalMethods:          classVMServer.TotalMethods,
		HistoricCoverages:     classVMServer.HistoricCoverages, // Re-use from server VM
		LineCoverageHistory:   classVMServer.LineCoverageHistory,
		BranchCoverageHistory: classVMServer.BranchCoverageHistory,
		Metrics:               classVMServer.Metrics, // Re-use aggregated metrics
	}
	if classModel.BranchesCovered != nil {
		angularClassVMForJS.CoveredBranches = *classModel.BranchesCovered
	}
	if classModel.BranchesValid != nil {
		angularClassVMForJS.TotalBranches = *classModel.BranchesValid
	}

	detailVM := AngularClassDetailViewModel{Class: angularClassVMForJS, Files: []AngularCodeFileViewModel{}}
	if classModel.Files == nil {
		return detailVM, nil
	}

	for _, fileInClass := range classModel.Files {
		angularFileForJS, err := b.buildAngularFileViewModelForJS(&fileInClass)
		if err != nil {
			// Log error, but try to continue
			fmt.Fprintf(os.Stderr, "Error building Angular file view model for JS (%s): %v\n", fileInClass.Path, err)
			continue
		}
		detailVM.Files = append(detailVM.Files, angularFileForJS)
	}
	return detailVM, nil

}

func (b *HtmlReportBuilder) buildAngularFileViewModelForJS(fileInClass *model.CodeFile) (AngularCodeFileViewModel, error) {
	angularFile := AngularCodeFileViewModel{
		Path:           fileInClass.Path,
		CoveredLines:   fileInClass.CoveredLines,
		CoverableLines: fileInClass.CoverableLines,
		TotalLines:     fileInClass.TotalLines,
		Lines:          []AngularLineAnalysisViewModel{},
	}
	sourceLines, err := filereader.ReadLinesInFile(fileInClass.Path)
	if err != nil {
		// If source can't be read, the lines array will be empty.
		// This is consistent with C# RG behavior (no lines if file not found).
		fmt.Fprintf(os.Stderr, "Warning: could not read source file %s for JS Angular VM: %v\n", fileInClass.Path, err)
		return angularFile, nil // Return with empty lines, not a fatal error for this part.
	}

	coverageLinesMap := make(map[int]*model.Line)
	if fileInClass.Lines != nil {
		for i := range fileInClass.Lines { // Use index for pointer
			covLine := &fileInClass.Lines[i]
			coverageLinesMap[covLine.Number] = covLine
		}
	}

	for i, content := range sourceLines {
		actualLineNumber := i + 1
		modelCovLine, hasCoverageData := coverageLinesMap[actualLineNumber]
		angularLine := b.buildAngularLineViewModelForJS(content, actualLineNumber, modelCovLine, hasCoverageData)
		angularFile.Lines = append(angularFile.Lines, angularLine)
	}
	return angularFile, nil

}

func (b *HtmlReportBuilder) buildAngularLineViewModelForJS(content string, actualLineNumber int, modelCovLine *model.Line, hasCoverageData bool) AngularLineAnalysisViewModel {
	lineVM := AngularLineAnalysisViewModel{
		LineNumber:  actualLineNumber,
		LineContent: content, // For JS, we send the raw content
	}
	if hasCoverageData {
		lineVM.Hits = modelCovLine.Hits
		lineVM.CoveredBranches = modelCovLine.CoveredBranches
		lineVM.TotalBranches = modelCovLine.TotalBranches
		lineVM.LineVisitStatus = lineVisitStatusToString(determineLineVisitStatus(modelCovLine.Hits, modelCovLine.IsBranchPoint, modelCovLine.CoveredBranches, modelCovLine.TotalBranches))
	} else {
		lineVM.LineVisitStatus = lineVisitStatusToString(lineVisitStatusNotCoverable)
	}
	return lineVM
}

func (b *HtmlReportBuilder) buildClassDetailPageData(classVM ClassViewModelForDetail, tag string, classDetailJS template.JS) ClassDetailData {
	appVersion := "0.0.1" // Placeholder, same as summary
	if b.ReportContext.ReportConfiguration() != nil {
		appVersion = "0.0.1"
		// Logic to get actual app version if available
	}
	return ClassDetailData{
		ReportTitle:                           b.reportTitle,
		AppVersion:                            appVersion,
		CurrentDateTime:                       time.Now().Format("02/01/2006 - 15:04:05"),
		Class:                                 classVM,
		BranchCoverageAvailable:               b.branchCoverageAvailable,
		MethodCoverageAvailable:               b.methodCoverageAvailable,
		Tag:                                   tag,
		Translations:                          b.translations,
		MaximumDecimalPlacesForCoverageQuotas: b.maximumDecimalPlacesForCoverageQuotas,
		AngularCssFile:                        b.angularCssFile,
		AngularRuntimeJsFile:                  b.angularRuntimeJsFile,
		AngularPolyfillsJsFile:                b.angularPolyfillsJsFile,
		AngularMainJsFile:                     b.angularMainJsFile,
		AssembliesJSON:                        b.assembliesJSON,
		RiskHotspotsJSON:                      b.riskHotspotsJSON,
		MetricsJSON:                           b.metricsJSON,
		RiskHotspotMetricsJSON:                b.riskHotspotMetricsJSON,
		HistoricCoverageExecutionTimesJSON:    b.historicCoverageExecutionTimesJSON,
		TranslationsJSON:                      b.translationsJSON,
		ClassDetailJSON:                       classDetailJS,
	}
}
