package htmlreport

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/filereader"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/utils"
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
		AssemblyName: classModel.Name, // Placeholder, adjust if AssemblyName is available differently
		IsMultiFile:  len(classModel.Files) > 1,
	}
	if dotIndex := strings.LastIndex(classModel.Name, "."); dotIndex > -1 && dotIndex < len(classModel.Name)-1 {
		cvm.AssemblyName = classModel.Name[:dotIndex]
	}

	cvm.CoveredLines = classModel.LinesCovered
	cvm.CoverableLines = classModel.LinesValid
	cvm.UncoveredLines = cvm.CoverableLines - cvm.CoveredLines
	cvm.TotalLines = classModel.TotalLines

	b.populateLineCoverageMetricsForClassVM(&cvm, classModel)
	b.populateBranchCoverageMetricsForClassVM(&cvm, classModel)
	b.populateMethodCoverageMetricsForClassVM(&cvm, classModel)
	b.populateHistoricCoveragesForClassVM(&cvm, classModel)
	b.populateAggregatedMetricsForClassVM(&cvm, classModel)

	var allMethodMetricsForClass []*model.MethodMetric
	
	// Store file short paths and original index for CodeElements
	type codeElementWithContext struct {
		element       *model.CodeElement
		fileShortPath string
		fileIndexPlus1 int
	}
	var allCodeElementsWithContext []codeElementWithContext


	// Sort files by path for consistent file processing order if needed elsewhere,
	// and for correct fileIndexPlus1 if class is multi-file.
	sortedFiles := make([]model.CodeFile, len(classModel.Files))
	copy(sortedFiles, classModel.Files)
	sort.Slice(sortedFiles, func(i, j int) bool {
		return sortedFiles[i].Path < sortedFiles[j].Path
	})

	// First pass: Build FileViewModels and collect all CodeElements
	for fileIdx, fileInClassValue := range sortedFiles {
		fileInClass := fileInClassValue // Loop variable
		fileVM, _, err := b.buildFileViewModelForServerRender(&fileInClass) // Pass pointer
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not build file view model for %s: %v\n", fileInClass.Path, err)
			continue
		}
		cvm.Files = append(cvm.Files, fileVM) // Add file view model

		for i := range fileInClass.MethodMetrics {
			allMethodMetricsForClass = append(allMethodMetricsForClass, &fileInClass.MethodMetrics[i])
		}
		
		// Collect CodeElements with their file context
		// fileInClass.CodeElements is already sorted by line within this file by the analyzer
		for i := range fileInClass.CodeElements {
			codeElem := &fileInClass.CodeElements[i]
			allCodeElementsWithContext = append(allCodeElementsWithContext, codeElementWithContext{
				element: codeElem,
				fileShortPath: fileVM.ShortPath,
				fileIndexPlus1: fileIdx + 1,
			})
		}
	}

	// Sort ALL collected CodeElements globally for the class by line number
	sort.Slice(allCodeElementsWithContext, func(i, j int) bool {
		elemI := allCodeElementsWithContext[i].element
		elemJ := allCodeElementsWithContext[j].element
		// Use the GetFirstLine and GetSortableName methods for consistency with the sorter interface
		// though direct field access is also fine here since we have the concrete types.
		if elemI.GetFirstLine() == elemJ.GetFirstLine() {
			return elemI.GetSortableName() < elemJ.GetSortableName()
		}
		return elemI.GetFirstLine() < elemJ.GetFirstLine()
	})
	
	// Second pass: Populate sidebar elements from the globally sorted list
	for _, ceCtx := range allCodeElementsWithContext {
		sidebarElem := b.buildSidebarElementViewModel(ceCtx.element, ceCtx.fileShortPath, ceCtx.fileIndexPlus1, len(sortedFiles) > 1)
		cvm.SidebarElements = append(cvm.SidebarElements, sidebarElem)
	}


	if len(allMethodMetricsForClass) > 0 {
		cvm.FilesWithMetrics = true
		cvm.MetricsTable = b.buildMetricsTableForClassVM(classModel, allMethodMetricsForClass) // This already sorts methods globally
	}

	return cvm
}

func (b *HtmlReportBuilder) populateLineCoverageMetricsForClassVM(cvm *ClassViewModelForDetail, classModel *model.Class) {
	decimalPlaces := b.maximumDecimalPlacesForCoverageQuotas
	lineCoverage := utils.CalculatePercentage(cvm.CoveredLines, cvm.CoverableLines, decimalPlaces)
	cvm.CoveragePercentageForDisplay = utils.FormatPercentage(lineCoverage, decimalPlaces)

	if !math.IsNaN(lineCoverage) {
		cvm.CoveragePercentageBarValue = 100 - int(math.Round(lineCoverage))
		cvm.CoverageRatioTextForDisplay = fmt.Sprintf("%d of %d", cvm.CoveredLines, cvm.CoverableLines)
	} else {
		cvm.CoveragePercentageBarValue = 0
		cvm.CoverageRatioTextForDisplay = "-"
	}
}

func (b *HtmlReportBuilder) populateBranchCoverageMetricsForClassVM(cvm *ClassViewModelForDetail, classModel *model.Class) {
	// Ensure b.branchCoverageAvailable is correctly set in the HtmlReportBuilder
	if b.branchCoverageAvailable && classModel.BranchesValid != nil && *classModel.BranchesValid > 0 && classModel.BranchesCovered != nil {
		cvm.CoveredBranches = *classModel.BranchesCovered
		cvm.TotalBranches = *classModel.BranchesValid
		// Use utils.CalculatePercentage and utils.FormatPercentage
		decimalPlaces := b.maximumDecimalPlacesForCoverageQuotas
		branchCoverage := utils.CalculatePercentage(*classModel.BranchesCovered, *classModel.BranchesValid, decimalPlaces)
		cvm.BranchCoveragePercentageForDisplay = utils.FormatPercentage(branchCoverage, decimalPlaces)

		if !math.IsNaN(branchCoverage) {
			cvm.BranchCoveragePercentageBarValue = 100 - int(math.Round(branchCoverage))
			cvm.BranchCoverageRatioTextForDisplay = fmt.Sprintf("%d of %d", cvm.CoveredBranches, cvm.TotalBranches)
		} else {
			cvm.BranchCoveragePercentageBarValue = 0
			cvm.BranchCoverageRatioTextForDisplay = "-"
		}
	} else {
		cvm.BranchCoveragePercentageForDisplay = "N/A" // From translations ideally
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
		ShortPath: utils.ReplaceInvalidPathChars(filepath.Base(fileInClass.Path)),
	}
	sourceLines, err := filereader.ReadLinesInFile(fileInClass.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read source file %s: %v\n", fileInClass.Path, err)
		sourceLines = []string{}
	}

	coverageLinesMap := make(map[int]*model.Line)
	for i := range fileInClass.Lines {
		covLine := &fileInClass.Lines[i]
		coverageLinesMap[covLine.Number] = covLine
	}

	for lineNumIdx, lineContent := range sourceLines {
		actualLineNumber := lineNumIdx + 1
		modelCovLine, hasCoverageData := coverageLinesMap[actualLineNumber]
		lineVM := b.buildLineViewModelForServerRender(lineContent, actualLineNumber, modelCovLine, hasCoverageData)
		fileVM.Lines = append(fileVM.Lines, lineVM)
	}
	return fileVM, sourceLines, nil
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
		Name:          codeElem.Name,     // This is now the short, display-friendly name
		FullName:      codeElem.FullName, // This is now the full, cleaned name
		FileShortPath: fileShortPath,
		Line:          codeElem.FirstLine,
		Icon:          "cube",
	}
	if isMultiFile {
		sidebarElem.FileIndexPlus1 = fileIndexPlus1
	}
	if codeElem.Type == model.PropertyElementType {
		sidebarElem.Icon = "wrench"
	}

	var coverageTitleText string
	if codeElem.CoverageQuota != nil {
		sidebarElem.CoverageBarValue = int(math.Round(*codeElem.CoverageQuota))
		coverageTitleText = fmt.Sprintf("Line coverage: %.1f%%", *codeElem.CoverageQuota)
	} else {
		sidebarElem.CoverageBarValue = -1
		coverageTitleText = "Line coverage: N/A"
	}
	// Use FullName for the detailed part of the title
	sidebarElem.CoverageTitle = fmt.Sprintf("%s - %s", coverageTitleText, codeElem.FullName)
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
	// Iterate over sorted files for deterministic search if files are not already sorted
	// but classModel.Files used here should be the same as used for sidebar generation
	sortedFiles := make([]model.CodeFile, len(classModel.Files))
	copy(sortedFiles, classModel.Files)
	sort.Slice(sortedFiles, func(i, j int) bool {
		return sortedFiles[i].Path < sortedFiles[j].Path
	})

	for fIdx, fValue := range sortedFiles {
		f := fValue // loop variable
		// CodeElements within f are already sorted by line by the analyzer
		for i := range f.CodeElements {
			ce := &f.CodeElements[i] // Use pointer
			// Match based on FirstLine and the method's cleaned DisplayName against CE's FullName
			if ce.FirstLine == method.FirstLine && ce.FullName == method.DisplayName {
				fileShortPath := utils.ReplaceInvalidPathChars(filepath.Base(f.Path))
				return ce, fileShortPath, fIdx + 1
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

	var fullNameForTitle string // This will be the cleaned full name
	var lineToLink int
	var isProperty bool
	var coverageQuota *float64

	// Use the pre-cleaned DisplayName from the method model
	cleanedFullName := method.DisplayName
	fullNameForTitle = cleanedFullName

	// Construct the full name first, which might be from CodeElement or model.Method
	if correspondingCE != nil {
		lineToLink = correspondingCE.FirstLine
		isProperty = (correspondingCE.Type == model.PropertyElementType)
		coverageQuota = correspondingCE.CoverageQuota
	} else {
		// Fallback if CodeElement wasn't found
		lineToLink = method.FirstLine
		// Determine if it's a property based on the cleaned name
		isProperty = strings.HasPrefix(cleanedFullName, "get_") || strings.HasPrefix(cleanedFullName, "set_")
	}

	var shortDisplayNameForTable string
	if isProperty {
		shortDisplayNameForTable = cleanedFullName
	} else {
		shortDisplayNameForTable = utils.GetShortMethodName(cleanedFullName)
	}

	row := AngularMethodMetricsViewModel{
		Name:           shortDisplayNameForTable, // Use the "short" version for table display
		FullName:       fullNameForTitle,         // Use the fuller version for tooltips
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

	// Create a mutable copy of methods to sort
	// model.Method is a struct, so we need a slice of model.Method values.
	sortedMethods := make([]model.Method, len(classModel.Methods))
	copy(sortedMethods, classModel.Methods)

	// Sort methods using the generic sorter
	utils.SortByLineAndName(sortedMethods) // This should now work

	for i := range sortedMethods {
		methodPtr := &sortedMethods[i] // Get a pointer to the method in the sorted slice

		correspondingCE, fileShortPath, fileIndexPlus1 := findCorrespondingCodeElement(methodPtr, classModel)

		if correspondingCE == nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not find corresponding CodeElement for method %s (line %d) in class %s for metrics table. Using fallback naming.\n", methodPtr.DisplayName, methodPtr.FirstLine, classModel.DisplayName)
		}

		row := b.buildSingleMetricRow(methodPtr, correspondingCE, fileShortPath, fileIndexPlus1, metricsTable.Headers)
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

	decimalPlaces := b.maximumDecimalPlacesForCoverageQuotas // Use from builder

	switch metric.Name {
	case "Line coverage", "Branch coverage": // These are already percentages
		return utils.FormatPercentage(valFloat, decimalPlaces)
	case "CrapScore":
		return fmt.Sprintf("%.2f", valFloat)
	case "Cyclomatic complexity", "Complexity":
		return fmt.Sprintf("%.0f", valFloat)
	default:
		return fmt.Sprintf(fmt.Sprintf("%%.%df", decimalPlaces), valFloat)
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
