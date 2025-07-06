package cobertura

import (
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/inputxml"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/utils"
)

// This file contains the core logic for processing Cobertura XML data into the
// generic data model. The design is centered around a 'processingOrchestrator'
// struct which holds the necessary dependencies (like a file reader and configuration)
// to make the logic testable and decoupled from direct file system access.
//
// The main responsibilities are:
// - Iterating through packages (assemblies) and classes defined in the XML.
// - Grouping partial class definitions that may be spread across multiple files.
// - Reading and analyzing source code files to determine line-by-line coverage.
// - Merging coverage data (line hits, branch data) from different XML fragments.
// - Parsing method-specific metrics like cyclomatic complexity.
// - Applying various filters (assembly, class, file) provided in the configuration.
// - Handling C#-specific compiler-generated artifacts like lambdas, async state
//   machines, and nested types, cleaning them up for a more readable report.

// fileProcessingMetrics holds metrics aggregated during the processing of a single file.
type fileProcessingMetrics struct {
	linesCovered    int
	linesValid      int
	branchesCovered int
	branchesValid   int
}

// processingOrchestrator holds dependencies and state for a single parsing operation.
type processingOrchestrator struct {
	fileReader                      FileReader
	config                          parser.ParserConfig
	sourceDirs                      []string
	uniqueFilePathsForGrandTotalLines map[string]int
	processedAssemblyFiles          map[string]struct{}
	supportsBranchCoverage          bool 
}

// newProcessingOrchestrator creates a new orchestrator for processing Cobertura data.
func newProcessingOrchestrator(
	fileReader FileReader,
	config parser.ParserConfig,
	sourceDirs []string,
	supportsBranchCoverage bool, 
) *processingOrchestrator {
	return &processingOrchestrator{
		fileReader:                      fileReader,
		config:                          config,
		sourceDirs:                      sourceDirs,
		uniqueFilePathsForGrandTotalLines: make(map[string]int),
		supportsBranchCoverage:          supportsBranchCoverage,
	}
}


// processPackages is the entry point for the orchestrator.
func (o *processingOrchestrator) processPackages(packages []inputxml.PackageXML) ([]model.Assembly, error) {
	var parsedAssemblies []model.Assembly
	for _, pkgXML := range packages {
		assembly, err := o.processPackage(pkgXML)
		if err != nil {
			slog.Warn("Could not process Cobertura package, skipping.", "package", pkgXML.Name, "error", err)
			continue
		}
		if assembly != nil { // A nil assembly means it was filtered out
			parsedAssemblies = append(parsedAssemblies, *assembly)
		}
	}
	return parsedAssemblies, nil
}

// processPackage transforms a single inputxml.PackageXML to a model.Assembly.
func (o *processingOrchestrator) processPackage(pkgXML inputxml.PackageXML) (*model.Assembly, error) {
	if !o.config.AssemblyFilters().IsElementIncludedInReport(pkgXML.Name) {
		return nil, nil
	}

	assembly := &model.Assembly{
		Name:    pkgXML.Name,
		Classes: []model.Class{},
	}
	o.processedAssemblyFiles = make(map[string]struct{})

	classesXMLGrouped := o.groupClassesByLogicalName(pkgXML.Classes.Class)

	for logicalName, classXMLGroup := range classesXMLGrouped {
		if o.isFilteredRawClassName(logicalName) {
			continue
		}

		classModel, err := o.processClassGroup(classXMLGroup)
		if err != nil {
			slog.Warn("Error processing class group, skipping.", "class", logicalName, "assembly", assembly.Name, "error", err)
			continue
		}
		if classModel != nil {
			assembly.Classes = append(assembly.Classes, *classModel)
		}
	}

	o.aggregateAssemblyMetrics(assembly)
	return assembly, nil
}

// processClassGroup processes all XML fragments for a single logical class.
func (o *processingOrchestrator) processClassGroup(classXMLs []inputxml.ClassXML) (*model.Class, error) {
	if len(classXMLs) == 0 {
		return nil, nil
	}

	logicalName := o.logicalClassName(classXMLs[0].Name)
	if !o.config.ClassFilters().IsElementIncludedInReport(logicalName) {
		return nil, nil
	}

	classModel := &model.Class{
		Name:        logicalName,
		DisplayName: o.formatDisplayName(logicalName),
		Files:       []model.CodeFile{},
		Methods:     []model.Method{},
		Metrics:     make(map[string]float64),
	}
	classProcessedFilePaths := make(map[string]struct{})

	xmlFragmentsByFile := o.groupClassFragmentsByFile(classXMLs)

	for filePath, fragmentsForFile := range xmlFragmentsByFile {
		codeFile, methodsInFile, err := o.processFileForClass(filePath, logicalName, fragmentsForFile)
		if err != nil {
			slog.Warn("Failed to process file for class, skipping file.", "file", filePath, "class", logicalName, "error", err)
			continue
		}

		classModel.Files = append(classModel.Files, *codeFile)
		classModel.Methods = append(classModel.Methods, methodsInFile...)

		o.processedAssemblyFiles[codeFile.Path] = struct{}{}
		classProcessedFilePaths[codeFile.Path] = struct{}{}
	}

	o.aggregateClassMetrics(classModel, classProcessedFilePaths)
	return classModel, nil
}

// processFileForClass processes all XML fragments associated with a single source file.
func (o *processingOrchestrator) processFileForClass(filePath, logicalClassName string, fragments []inputxml.ClassXML) (*model.CodeFile, []model.Method, error) {
	resolvedPath, err := utils.FindFileInSourceDirs(filePath, o.sourceDirs)
	if err != nil {
		slog.Warn("Source file not found, line content will be missing.", "file", filePath, "class", logicalClassName)
		resolvedPath = filePath // Use original path as fallback
	}

	sourceLines, _ := o.fileReader.ReadFile(resolvedPath)
	totalLines := o.getTotalLines(resolvedPath, sourceLines)
	maxLineNumInFile := getMaxLineNumber(fragments)
	mergedLineHits, mergedBranches := o.mergeLineAndBranchData(fragments, maxLineNumInFile)

	methodsInFile, codeElementsInFile, err := o.processMethodsForFile(fragments, sourceLines)
	if err != nil {
		return nil, nil, fmt.Errorf("processing methods for file %s: %w", filePath, err)
	}

	finalLinesForFile, fileMetrics := o.assembleLinesForFile(maxLineNumInFile, sourceLines, mergedLineHits, mergedBranches)

	codeFile := &model.CodeFile{
		Path:           resolvedPath,
		Lines:          finalLinesForFile,
		CoveredLines:   fileMetrics.linesCovered,
		CoverableLines: fileMetrics.linesValid,
		TotalLines:     totalLines,
		CodeElements:   codeElementsInFile,
	}

	for _, method := range methodsInFile {
		if method.MethodMetrics != nil {
			codeFile.MethodMetrics = append(codeFile.MethodMetrics, method.MethodMetrics...)
		}
	}
	codeFile.MethodMetrics = utils.DistinctBy(codeFile.MethodMetrics, func(mm model.MethodMetric) string { return mm.Name + fmt.Sprintf("_%d", mm.Line) })

	return codeFile, methodsInFile, nil
}

// mergeLineAndBranchData combines coverage data from multiple XML fragments for a single file.
func (o *processingOrchestrator) mergeLineAndBranchData(fragments []inputxml.ClassXML, maxLineNum int) (map[int]int, map[int][]model.BranchCoverageDetail) {
	lineHits := make(map[int]int)
	branchDetails := make(map[int][]model.BranchCoverageDetail)

	for _, fragment := range fragments {
		allLines := make([]inputxml.LineXML, len(fragment.Lines.Line))
		copy(allLines, fragment.Lines.Line)
		for _, method := range fragment.Methods.Method {
			allLines = append(allLines, method.Lines.Line...)
		}

		for _, lineXML := range allLines {
			lineNumber, err := strconv.Atoi(lineXML.Number)
			if err != nil || lineNumber <= 0 {
				continue
			}

			if hits, err := strconv.Atoi(lineXML.Hits); err == nil {
				if _, ok := lineHits[lineNumber]; !ok {
					lineHits[lineNumber] = hits
				} else if hits > 0 {
					lineHits[lineNumber] += hits
				}
			}

			if strings.EqualFold(lineXML.Branch, "true") {
				lineModel, _ := o.processLineXML(lineXML)
				if lineModel.IsBranchPoint {
					currentBranches := branchDetails[lineNumber]
					branchDetails[lineNumber] = o.mergeBranches(currentBranches, lineModel.Branch)
				}
			}
		}
	}
	return lineHits, branchDetails
}

// processMethodsForFile extracts and processes all methods from the given XML fragments.
func (o *processingOrchestrator) processMethodsForFile(fragments []inputxml.ClassXML, sourceLines []string) ([]model.Method, []model.CodeElement, error) {
	var allMethods []model.Method
	var allCodeElements []model.CodeElement

	for _, fragment := range fragments {
		for _, methodXML := range fragment.Methods.Method {
			methodModel, err := o.processMethodXML(methodXML, sourceLines, fragment.Name)
			if err != nil {
				continue // Skip lambdas etc.
			}
			allMethods = append(allMethods, *methodModel)
			allCodeElements = append(allCodeElements, o.createCodeElementFromMethod(methodModel))
		}
	}

	distinctMethods := utils.DistinctBy(allMethods, func(m model.Method) string { return m.DisplayName + fmt.Sprintf("_%d", m.FirstLine) })
	utils.SortByLineAndName(distinctMethods)

	distinctCodeElements := utils.DistinctBy(allCodeElements, func(ce model.CodeElement) string { return ce.FullName + fmt.Sprintf("_%d", ce.FirstLine) })
	utils.SortByLineAndName(distinctCodeElements)

	return distinctMethods, distinctCodeElements, nil
}

// assembleLinesForFile constructs the final array of model.Line for a file.
func (o *processingOrchestrator) assembleLinesForFile(maxLineNum int, sourceLines []string, lineHits map[int]int, branches map[int][]model.BranchCoverageDetail) ([]model.Line, fileProcessingMetrics) {
	var finalLines []model.Line
	metrics := fileProcessingMetrics{}

	for lineNum := 1; lineNum <= maxLineNum; lineNum++ {
		lineContent := ""
		if lineNum > 0 && lineNum <= len(sourceLines) {
			lineContent = sourceLines[lineNum-1]
		}

		hits, hasHits := lineHits[lineNum]
		currentLine := model.Line{
			Number:  lineNum,
			Content: lineContent,
			Hits:    -1, // Default to not coverable
		}
		if hasHits {
			currentLine.Hits = hits
		}

		if branchData, ok := branches[lineNum]; ok {
			currentLine.IsBranchPoint = true
			currentLine.Branch = branchData
			for _, b := range branchData {
				if b.Visits > 0 {
					currentLine.CoveredBranches++
				}
				currentLine.TotalBranches++
			}
		}

		currentLine.LineVisitStatus = determineLineVisitStatus(currentLine.Hits, currentLine.IsBranchPoint, currentLine.CoveredBranches, currentLine.TotalBranches)

		if currentLine.Hits >= 0 {
			metrics.linesValid++
			if currentLine.Hits > 0 {
				metrics.linesCovered++
			}
		}
		metrics.branchesCovered += currentLine.CoveredBranches
		metrics.branchesValid += currentLine.TotalBranches
		finalLines = append(finalLines, currentLine)
	}

	return finalLines, metrics
}

// processMethodXML transforms a single method XML element into a rich model.Method object.
func (o *processingOrchestrator) processMethodXML(methodXML inputxml.MethodXML, sourceLines []string, classNameFromXML string) (*model.Method, error) {
	fullNameFromXML := methodXML.Name + methodXML.Signature

	extractedFullNameForDisplay := o.extractMethodName(fullNameFromXML, classNameFromXML)
	if strings.Contains(extractedFullNameForDisplay, "__") && lambdaMethodNameRegexCobertura.MatchString(extractedFullNameForDisplay) {
		return nil, fmt.Errorf("method '%s' is a lambda and skipped", fullNameFromXML)
	}

	method := &model.Method{
		Name:        methodXML.Name,
		Signature:   methodXML.Signature,
		DisplayName: extractedFullNameForDisplay,
		Complexity:  parseFloat(methodXML.Complexity),
	}

	o.processMethodLines(methodXML, method, sourceLines)
	o.populateStandardMethodMetrics(method)

	return method, nil
}

// processMethodLines processes the <line> elements within a <method> to determine line/branch rates and line ranges.
func (o *processingOrchestrator) processMethodLines(methodXML inputxml.MethodXML, method *model.Method, sourceLines []string) {
	minLine, maxLine := math.MaxInt32, 0
	var methodLinesCovered, methodLinesValid int
	var methodBranchesCovered, methodBranchesValid int

	for _, lineXML := range methodXML.Lines.Line {
		currentLineNum, _ := strconv.Atoi(lineXML.Number)
		if currentLineNum < minLine {
			minLine = currentLineNum
		}
		if currentLineNum > maxLine {
			maxLine = currentLineNum
		}
		lineModel, lineMetricsStats := o.processLineXML(lineXML)
		method.Lines = append(method.Lines, lineModel)
		if lineModel.Hits >= 0 {
			methodLinesValid++
			if lineModel.Hits > 0 {
				methodLinesCovered++
			}
		}
		methodBranchesCovered += lineMetricsStats.branchesCovered
		methodBranchesValid += lineMetricsStats.branchesValid
	}

	if minLine == math.MaxInt32 {
		method.FirstLine = 0
	} else {
		method.FirstLine = minLine
	}
	method.LastLine = maxLine

	// Calculate LineRate
	if methodLinesValid > 0 {
		method.LineRate = float64(methodLinesCovered) / float64(methodLinesValid)
	} else {
		method.LineRate = 0.0
	}

	// === FINAL CORRECTED BRANCH RATE LOGIC ===
	if !o.supportsBranchCoverage {
		// If the entire report format does not support branch coverage, the rate is not applicable.
		method.BranchRate = nil
	} else if methodBranchesValid > 0 {
		// If branches exist, calculate the coverage rate.
		rate := float64(methodBranchesCovered) / float64(methodBranchesValid)
		method.BranchRate = &rate
	} else {
		// If the format supports branches, but this specific method has none, it's 100% covered.
		rate := 1.0
		method.BranchRate = &rate
	}
}


// processLineXML transforms a single line XML element into a rich model.Line object.
func (o *processingOrchestrator) processLineXML(lineXML inputxml.LineXML) (model.Line, fileProcessingMetrics) {
	metrics := fileProcessingMetrics{}
	lineNumber, _ := strconv.Atoi(lineXML.Number)

	line := model.Line{
		Number:        lineNumber,
		Hits:          parseInt(lineXML.Hits),
		IsBranchPoint: strings.EqualFold(lineXML.Branch, "true"),
		Branch:        make([]model.BranchCoverageDetail, 0),
	}

	if line.IsBranchPoint {
		matches := conditionCoverageRegexCobertura.FindStringSubmatch(lineXML.ConditionCoverage)
		if len(matches) > 0 {
			coveredStr := findNamedGroup(conditionCoverageRegexCobertura, matches, "NumberOfCoveredBranches")
			totalStr := findNamedGroup(conditionCoverageRegexCobertura, matches, "NumberOfTotalBranches")

			if coveredStr != "" && totalStr != "" {
				numberOfCoveredBranches, errC := strconv.Atoi(coveredStr)
				numberOfTotalBranches, errT := strconv.Atoi(totalStr)

				if errC == nil && errT == nil && numberOfTotalBranches > 0 {
					line.CoveredBranches = numberOfCoveredBranches
					line.TotalBranches = numberOfTotalBranches
					for i := 0; i < line.TotalBranches; i++ {
						var visits int
						if i < line.CoveredBranches {
							visits = 1
						}

						var identifier string
						if i < len(lineXML.Conditions.Condition) {
							identifier = lineXML.Conditions.Condition[i].Number
						} else {
							identifier = fmt.Sprintf("%d_%d", lineNumber, i)
						}
						line.Branch = append(line.Branch, model.BranchCoverageDetail{Identifier: identifier, Visits: visits})
					}
				}
			}
		} else if len(lineXML.Conditions.Condition) > 0 {
			for _, condition := range lineXML.Conditions.Condition {
				var visits int
				if strings.HasPrefix(condition.Coverage, "100") {
					visits = 1
					line.CoveredBranches++
				}
				line.Branch = append(line.Branch, model.BranchCoverageDetail{Identifier: condition.Number, Visits: visits})
				line.TotalBranches++
			}
		} else {
			o.setFallbackBranchData(&line)
		}
	}

	metrics.branchesCovered = line.CoveredBranches
	metrics.branchesValid = line.TotalBranches
	return line, metrics
}

func (o *processingOrchestrator) setFallbackBranchData(line *model.Line) {
	if line.Hits > 0 {
		line.CoveredBranches = 1
	} else {
		line.CoveredBranches = 0
	}
	line.TotalBranches = 1
	line.Branch = append(line.Branch, model.BranchCoverageDetail{
		Identifier: fmt.Sprintf("%d_0", line.Number),
		Visits:     line.CoveredBranches,
	})
}

func (o *processingOrchestrator) populateStandardMethodMetrics(method *model.Method) {
	method.MethodMetrics = []model.MethodMetric{}
	shortMetricName := utils.GetShortMethodName(method.DisplayName)

	if !math.IsNaN(method.Complexity) {
		method.MethodMetrics = append(method.MethodMetrics, model.MethodMetric{
			Name: shortMetricName, Line: method.FirstLine,
			Metrics: []model.Metric{{Name: "Cyclomatic complexity", Value: method.Complexity, Status: model.StatusOk}},
		})
	}

	lineCoveragePercentage := method.LineRate * 100.0
	if !math.IsNaN(lineCoveragePercentage) {
		method.MethodMetrics = append(method.MethodMetrics, model.MethodMetric{
			Name: shortMetricName, Line: method.FirstLine,
			Metrics: []model.Metric{{Name: "Line coverage", Value: lineCoveragePercentage, Status: model.StatusOk}},
		})
	}

	// Correctly handle nullable BranchRate
	if method.BranchRate != nil {
		branchCoveragePercentage := *method.BranchRate * 100.0
		if !math.IsNaN(branchCoveragePercentage) {
			method.MethodMetrics = append(method.MethodMetrics, model.MethodMetric{
				Name: shortMetricName, Line: method.FirstLine,
				Metrics: []model.Metric{{Name: "Branch coverage", Value: branchCoveragePercentage, Status: model.StatusOk}},
			})
		}
	}

	// CrapScore depends on coverage. Use branch coverage if available, otherwise line coverage.
	var coverageForCrapScore float64
	if method.BranchRate != nil {
		coverageForCrapScore = *method.BranchRate
	} else {
		// Fallback to line coverage for CrapScore if branch coverage is not applicable.
		coverageForCrapScore = method.LineRate
	}

	crapScoreValue := o.calculateCrapScore(coverageForCrapScore, method.Complexity)
	if !math.IsNaN(crapScoreValue) {
		method.MethodMetrics = append(method.MethodMetrics, model.MethodMetric{
			Name: shortMetricName, Line: method.FirstLine,
			Metrics: []model.Metric{{Name: "CrapScore", Value: crapScoreValue, Status: model.StatusOk}},
		})
	}
}

func (o *processingOrchestrator) calculateCrapScore(coverage float64, complexity float64) float64 {
	if math.IsNaN(coverage) || math.IsInf(coverage, 0) || coverage < 0 || coverage > 1 {
		coverage = 0
	}
	if math.IsNaN(complexity) || math.IsInf(complexity, 0) || complexity < 0 {
		return math.NaN()
	}
	uncoveredRatio := 1.0 - coverage
	return (math.Pow(complexity, 2) * math.Pow(uncoveredRatio, 3)) + complexity
}

func (o *processingOrchestrator) extractMethodName(methodNamePlusSignature, classNameFromXML string) string {
	rawMode := o.config.Settings().RawMode
	if rawMode {
		return methodNamePlusSignature
	}

	combinedNameForContext := classNameFromXML + methodNamePlusSignature
	if strings.Contains(methodNamePlusSignature, "|") && (strings.Contains(classNameFromXML, ">g__") || strings.Contains(methodNamePlusSignature, ">g__")) {
		if match := localFunctionMethodNameRegexCobertura.FindStringSubmatch(combinedNameForContext); match != nil {
			if nestedName := findNamedGroup(localFunctionMethodNameRegexCobertura, match, "NestedMethodName"); nestedName != "" {
				return nestedName + "()"
			}
		}
	}
	if strings.HasSuffix(methodNamePlusSignature, "MoveNext()") {
		if match := compilerGeneratedMethodNameRegexCobertura.FindStringSubmatch(combinedNameForContext); match != nil {
			if compilerGenName := findNamedGroup(compilerGeneratedMethodNameRegexCobertura, match, "CompilerGeneratedName"); compilerGenName != "" {
				return compilerGenName + "()"
			}
		}
	}
	return methodNamePlusSignature
}

func (o *processingOrchestrator) formatDisplayName(rawClassName string) string {
	if o.config.Settings().RawMode {
		return rawClassName
	}
	nameForDisplay := nestedTypeSeparatorRegexCobertura.ReplaceAllString(rawClassName, ".")
	match := genericClassRegexCobertura.FindStringSubmatch(nameForDisplay)
	if match == nil {
		return nameForDisplay
	}

	baseDisplayName := findNamedGroup(genericClassRegexCobertura, match, "Name")
	numberStr := findNamedGroup(genericClassRegexCobertura, match, "Number")
	argCount, _ := strconv.Atoi(numberStr)

	if argCount > 0 {
		var sb strings.Builder
		sb.WriteString("<")
		for i := 1; i <= argCount; i++ {
			if i > 1 {
				sb.WriteString(", ")
			}
			sb.WriteString("T")
			if argCount > 1 {
				sb.WriteString(strconv.Itoa(i))
			}
		}
		sb.WriteString(">")
		return baseDisplayName + sb.String()
	}
	return baseDisplayName
}

func (o *processingOrchestrator) logicalClassName(raw string) string {
	if o.config.Settings().RawMode {
		return raw
	}
	if i := strings.IndexAny(raw, "/$+"); i != -1 {
		return raw[:i]
	}
	return raw
}

func (o *processingOrchestrator) isFilteredRawClassName(rawName string) bool {
	if o.config.Settings().RawMode {
		return false
	}
	if strings.Contains(rawName, ">d__") ||
		strings.Contains(rawName, "/<>c") || strings.Contains(rawName, "+<>c") ||
		strings.HasPrefix(rawName, "<>c") ||
		strings.Contains(rawName, ">e__") ||
		(strings.Contains(rawName, "|") && strings.Contains(rawName, ">g__")) {
		return true
	}
	if idx := strings.LastIndexAny(rawName, "/+"); idx != -1 {
		nestedPart := rawName[idx+1:]
		if (strings.HasPrefix(nestedPart, "<") && (strings.Contains(nestedPart, ">d__") || strings.Contains(nestedPart, ">e__") || strings.Contains(nestedPart, ">g__"))) || strings.HasPrefix(nestedPart, "<>c") {
			return true
		}
	}
	return false
}

func (o *processingOrchestrator) groupClassesByLogicalName(classes []inputxml.ClassXML) map[string][]inputxml.ClassXML {
	grouped := make(map[string][]inputxml.ClassXML)
	for _, classXML := range classes {
		logicalName := o.logicalClassName(classXML.Name)
		grouped[logicalName] = append(grouped[logicalName], classXML)
	}
	return grouped
}

func (o *processingOrchestrator) groupClassFragmentsByFile(classXMLs []inputxml.ClassXML) map[string][]inputxml.ClassXML {
	grouped := make(map[string][]inputxml.ClassXML)
	for _, classXML := range classXMLs {
		if classXML.Filename == "" || !o.config.FileFilters().IsElementIncludedInReport(classXML.Filename) {
			continue
		}
		grouped[classXML.Filename] = append(grouped[classXML.Filename], classXML)
	}
	return grouped
}

func (o *processingOrchestrator) aggregateAssemblyMetrics(assembly *model.Assembly) {
	var linesCovered, linesValid, branchesCovered, branchesValid, totalLines int
	hasBranchData := false

	for _, cls := range assembly.Classes {
		linesCovered += cls.LinesCovered
		linesValid += cls.LinesValid
		if cls.BranchesCovered != nil && cls.BranchesValid != nil {
			hasBranchData = true
			branchesCovered += *cls.BranchesCovered
			branchesValid += *cls.BranchesValid
		}
	}
	for path := range o.processedAssemblyFiles {
		if lineCount, ok := o.uniqueFilePathsForGrandTotalLines[path]; ok {
			totalLines += lineCount
		}
	}
	assembly.LinesCovered = linesCovered
	assembly.LinesValid = linesValid
	assembly.TotalLines = totalLines
	if hasBranchData {
		assembly.BranchesCovered = &branchesCovered
		assembly.BranchesValid = &branchesValid
	}
}

func (o *processingOrchestrator) aggregateClassMetrics(class *model.Class, processedFiles map[string]struct{}) {
	var totalClassLines, totalClassBranchesCovered, totalClassBranchesValid int
	var coveredM, fullyCoveredM, totalM int
	hasClassBranchData := false

	for _, f := range class.Files {
		class.LinesCovered += f.CoveredLines
		class.LinesValid += f.CoverableLines
		for _, line := range f.Lines {
			if line.IsBranchPoint {
				hasClassBranchData = true
				totalClassBranchesValid += line.TotalBranches
				totalClassBranchesCovered += line.CoveredBranches
			}
		}
	}
	if hasClassBranchData {
		class.BranchesCovered = &totalClassBranchesCovered
		class.BranchesValid = &totalClassBranchesValid
	}

	for path := range processedFiles {
		if lineCount, ok := o.uniqueFilePathsForGrandTotalLines[path]; ok {
			totalClassLines += lineCount
		}
	}
	class.TotalLines = totalClassLines

	if len(class.Methods) > 0 {
		totalM = len(class.Methods)
		for _, method := range class.Methods {
			atLeastOneLineCoveredInMethod := false
			methodIsFullyCovered := true
			methodHasCoverableLines := false
			for _, line := range method.Lines {
				if line.Hits >= 0 {
					methodHasCoverableLines = true
					if line.Hits > 0 {
						atLeastOneLineCoveredInMethod = true
					} else {
						methodIsFullyCovered = false
					}
				}
			}
			if atLeastOneLineCoveredInMethod {
				coveredM++
			}
			if methodHasCoverableLines && methodIsFullyCovered {
				fullyCoveredM++
			} else if !methodHasCoverableLines && len(method.Lines) == 0 {
				fullyCoveredM++
			}
		}
	}
	class.CoveredMethods = coveredM
	class.FullyCoveredMethods = fullyCoveredM
	class.TotalMethods = totalM

	for _, method := range class.Methods {
		if !math.IsNaN(method.Complexity) {
			class.Metrics["Cyclomatic complexity"] += method.Complexity
		}
		for _, methodMetric := range method.MethodMetrics {
			for _, metric := range methodMetric.Metrics {
				if valFloat, ok := metric.Value.(float64); ok && !math.IsNaN(valFloat) && metric.Name != "Cyclomatic complexity" {
					class.Metrics[metric.Name] += valFloat
				}
			}
		}
	}
}

func (o *processingOrchestrator) getTotalLines(path string, sourceLines []string) int {
	if count, ok := o.uniqueFilePathsForGrandTotalLines[path]; ok {
		return count
	}
	if lineCount, err := o.fileReader.CountLines(path); err == nil {
		o.uniqueFilePathsForGrandTotalLines[path] = lineCount
		return lineCount
	}
	if sourceLines != nil {
		o.uniqueFilePathsForGrandTotalLines[path] = len(sourceLines)
		return len(sourceLines)
	}
	return 0
}

func getMaxLineNumber(fragments []inputxml.ClassXML) int {
	maxLine := 0
	for _, fragment := range fragments {
		allLines := fragment.Lines.Line
		for _, method := range fragment.Methods.Method {
			allLines = append(allLines, method.Lines.Line...)
		}
		for _, lineXML := range allLines {
			if ln, _ := strconv.Atoi(lineXML.Number); ln > maxLine {
				maxLine = ln
			}
		}
	}
	return maxLine
}

func (o *processingOrchestrator) createCodeElementFromMethod(method *model.Method) model.CodeElement {
	elementType := model.MethodElementType
	if strings.HasPrefix(method.DisplayName, "get_") || strings.HasPrefix(method.DisplayName, "set_") {
		elementType = model.PropertyElementType
	}

	var coverageQuota *float64
	if len(method.Lines) > 0 && !math.IsNaN(method.LineRate) {
		cq := method.LineRate * 100.0
		coverageQuota = &cq
	}

	var shortName string
	if elementType == model.PropertyElementType {
		shortName = method.DisplayName
	} else {
		shortName = utils.GetShortMethodName(method.DisplayName)
	}

	return model.CodeElement{
		Name:          shortName,
		FullName:      method.DisplayName,
		Type:          elementType,
		FirstLine:     method.FirstLine,
		LastLine:      method.LastLine,
		CoverageQuota: coverageQuota,
	}
}

func (o *processingOrchestrator) mergeBranches(existing, new []model.BranchCoverageDetail) []model.BranchCoverageDetail {
	if existing == nil {
		return new
	}
	for _, newBranch := range new {
		found := false
		for i, existingBranch := range existing {
			if existingBranch.Identifier == newBranch.Identifier {
				existing[i].Visits += newBranch.Visits
				found = true
				break
			}
		}
		if !found {
			existing = append(existing, newBranch)
		}
	}
	return existing
}

// findNamedGroup safely retrieves a captured group's value from a regex match slice.
func findNamedGroup(re *regexp.Regexp, match []string, groupName string) string {
	for i, name := range re.SubexpNames() {
		if i > 0 && i < len(match) && name == groupName {
			return match[i]
		}
	}
	return ""
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

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