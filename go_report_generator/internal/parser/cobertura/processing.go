package cobertura

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/filereader"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/inputxml"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/utils"
)

// Cobertura-specific Regexes
var (
	// Based on: Palmmedia.ReportGenerator.Core.Parser.CoberturaParser.cs
	// Original C# Regex: private static readonly Regex BranchCoverageRegex = new Regex("\\((?<NumberOfCoveredBranches>\\d+)/(?<NumberOfTotalBranches>\\d+)\\)$", RegexOptions.Compiled);
	conditionCoverageRegexCobertura = regexp.MustCompile(`\((?P<NumberOfCoveredBranches>\d+)/(?P<NumberOfTotalBranches>\d+)\)$`)

	// Based on: Palmmedia.ReportGenerator.Core.Parser.CoberturaParser.cs
	// Original C# Regex: private static readonly Regex LambdaMethodNameRegex = new Regex("<.+>.+__", RegexOptions.Compiled);
	lambdaMethodNameRegexCobertura = regexp.MustCompile(`<.+>.+__`)

	// Based on: Palmmedia.ReportGenerator.Core.Parser.CoberturaParser.cs
	// Original C# Regex: private static readonly Regex CompilerGeneratedMethodNameRegex = new Regex(@"(?<ClassName>.+)(/|\.)<(?<CompilerGeneratedName>.+)>.+__.+MoveNext\(\)$", RegexOptions.Compiled);
	// Go version uses a non-capturing group for the separator: (?:/|\.)
	compilerGeneratedMethodNameRegexCobertura = regexp.MustCompile(`(?P<ClassName>.+)(?:/|\.)<(?P<CompilerGeneratedName>.+)>.+__.+MoveNext\(\)$`)

	// Based on: Palmmedia.ReportGenerator.Core.Parser.CoberturaParser.cs
	// Original C# Regex: private static readonly Regex LocalFunctionMethodNameRegex = new Regex(@"^.*(?<ParentMethodName><.+>).*__(?<NestedMethodName>[^\|]+)\|.*$", RegexOptions.Compiled);
	// Go version is adapted for submatch extraction focusing on NestedMethodName and optionally ParentMethodName.
	localFunctionMethodNameRegexCobertura = regexp.MustCompile(`(?:.*<(?P<ParentMethodName>[^>]+)>g__)?(?P<NestedMethodName>[^|]+)\|`)

	// Based on: Palmmedia.ReportGenerator.Core.Parser.Analysis.Class.cs (GenericClassRegex)
	// Original C# Regex: private static readonly Regex GenericClassRegex = new Regex("^(?<Name>.+)`(?<Number>\\d+)$", RegexOptions.Compiled);
	// Go version uses (?P<Name>...) and (?P<Number>...) for named capture groups.
	genericClassRegexCobertura = regexp.MustCompile("^(?P<Name>.+)`(?P<Number>\\d+)$")

	// This regex is an adaptation of string replacement logic found in C# ReportGenerator (e.g., in OpenCoverParser for FullName).
	// It's used here to normalize nested class separators for display purposes.
	// C# equivalent logic: .Replace('/', '.').Replace('+', '.')
	nestedTypeSeparatorRegexCobertura = regexp.MustCompile(`[+/]`)
)

// fileProcessingMetricsCobertura holds metrics aggregated during the processing of a single <class> XML element's file fragment for Cobertura.
type fileProcessingMetricsCobertura struct {
	linesCovered    int
	linesValid      int
	branchesCovered int
	branchesValid   int
}

// processCoberturaPackageXML transforms inputxml.PackageXML to model.Assembly for Cobertura.
// It applies assembly filters from the config.
func (cp *CoberturaParser) processCoberturaPackageXML(
	pkgXML inputxml.PackageXML,
	sourceDirs []string,
	uniqueFilePathsForGrandTotalLines map[string]int,
	config parser.ParserConfig,
) (*model.Assembly, error) {
	settings := config.Settings()

	if !config.AssemblyFilters().IsElementIncludedInReport(pkgXML.Name) {
		return nil, nil
	}

	assembly := model.Assembly{
		Name:    pkgXML.Name,
		Classes: []model.Class{},
	}
	assemblyProcessedFilePaths := make(map[string]struct{})

	classesXMLGrouped := make(map[string][]inputxml.ClassXML)
	for _, classXML := range pkgXML.Classes.Class {
		logicalName := cp.logicalClassNameCobertura(classXML.Name, settings.RawMode)
		classesXMLGrouped[logicalName] = append(classesXMLGrouped[logicalName], classXML)
	}

	for logicalName, classXMLGroup := range classesXMLGrouped {
		if cp.isFilteredRawClassNameCobertura(logicalName, settings.RawMode) {
			continue
		}
		// Pass 'config' down the call chain
		classModel, err := cp.processCoberturaClassGroup(classXMLGroup, assembly.Name, sourceDirs, uniqueFilePathsForGrandTotalLines, assemblyProcessedFilePaths, config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: CoberturaParser: error processing class group '%s' in assembly '%s': %v\n", logicalName, assembly.Name, err)
			continue
		}
		if classModel != nil {
			assembly.Classes = append(assembly.Classes, *classModel)
		}
	}

	// Aggregation logic remains unchanged...
	var allClassLinesCovered, allClassLinesValid []int
	var allClassBranchesCovered, allClassBranchesValid []int
	hasAsmBranchData := false

	for i := range assembly.Classes {
		cls := &assembly.Classes[i]
		allClassLinesCovered = append(allClassLinesCovered, cls.LinesCovered)
		allClassLinesValid = append(allClassLinesValid, cls.LinesValid)
		if cls.BranchesCovered != nil && cls.BranchesValid != nil {
			hasAsmBranchData = true
			allClassBranchesCovered = append(allClassBranchesCovered, *cls.BranchesCovered)
			allClassBranchesValid = append(allClassBranchesValid, *cls.BranchesValid)
		}
	}
	assembly.LinesCovered = utils.SafeSumInt(allClassLinesCovered)
	assembly.LinesValid = utils.SafeSumInt(allClassLinesValid)
	if hasAsmBranchData {
		bc := utils.SafeSumInt(allClassBranchesCovered)
		bv := utils.SafeSumInt(allClassBranchesValid)
		assembly.BranchesCovered = &bc
		assembly.BranchesValid = &bv
	}
	for path := range assemblyProcessedFilePaths {
		if lineCount, ok := uniqueFilePathsForGrandTotalLines[path]; ok {
			assembly.TotalLines += lineCount
		}
	}

	return &assembly, nil
}

func (cp *CoberturaParser) processCoberturaClassGroup(
	classXMLs []inputxml.ClassXML, // All XML <class> elements for the same logical class name
	assemblyName string,
	sourceDirs []string,
	uniqueFilePathsForGrandTotalLines map[string]int,
	assemblyProcessedFilePaths map[string]struct{},
	config parser.ParserConfig,
) (*model.Class, error) {
	if len(classXMLs) == 0 {
		return nil, nil
	}
	settings := config.Settings()

	logicalName := cp.logicalClassNameCobertura(classXMLs[0].Name, settings.RawMode)
	if !config.ClassFilters().IsElementIncludedInReport(logicalName) {
		return nil, nil
	}
	displayName := cp.formatDisplayNameCobertura(logicalName, settings.RawMode)

	classModel := model.Class{
		Name:        logicalName,
		DisplayName: displayName,
		Files:       []model.CodeFile{},
		Methods:     []model.Method{},
		Metrics:     make(map[string]float64),
	}
	classProcessedFilePaths := make(map[string]struct{}) // Tracks files processed for *this* class's TotalLines
	var totalClassBranchesCovered, totalClassBranchesValid int
	hasClassBranchData := false

	var allMethodsForClassAcrossFiles []model.Method // Collect all methods for the *entire class* here

	// Group inputxml.ClassXML elements by their Filename attribute
	xmlFragmentsByFile := make(map[string][]inputxml.ClassXML)
	for _, classXML := range classXMLs {
		if classXML.Filename == "" {
			continue
		}
		if !config.FileFilters().IsElementIncludedInReport(classXML.Filename) {
			continue
		}
		xmlFragmentsByFile[classXML.Filename] = append(xmlFragmentsByFile[classXML.Filename], classXML)
	}

	// For each unique physical file path this class touches:
	for filePath, fragmentsForFile := range xmlFragmentsByFile {
		currentCodeFile := model.CodeFile{
			Path:          filePath, // Initial path, will be resolved
			MethodMetrics: []model.MethodMetric{},
			CodeElements:  []model.CodeElement{},
		}
		var sourceLinesForFile []string
		var allCodeElementsForFileFragment []model.CodeElement // Collect CodeElements for this specific file fragment

		resolvedPath, err := utils.FindFileInSourceDirs(filePath, sourceDirs)
		if err == nil {
			currentCodeFile.Path = resolvedPath
			sLines, readErr := filereader.ReadLinesInFile(resolvedPath)
			if readErr == nil {
				sourceLinesForFile = sLines
			}
			if _, known := uniqueFilePathsForGrandTotalLines[resolvedPath]; !known {
				if n, ferr := filereader.CountLinesInFile(resolvedPath); ferr == nil {
					uniqueFilePathsForGrandTotalLines[resolvedPath] = n
				} else if readErr == nil {
					uniqueFilePathsForGrandTotalLines[resolvedPath] = len(sourceLinesForFile)
				}
			}
			if lineCount, ok := uniqueFilePathsForGrandTotalLines[resolvedPath]; ok {
				currentCodeFile.TotalLines = lineCount
			}
		} else {
			fmt.Fprintf(os.Stderr, "Warning: CoberturaParser: source file '%s' for class '%s' not found. Line content will be missing.\n", filePath, logicalName)
		}

		maxLineNumInFile := 0
		for _, fragment := range fragmentsForFile {
			for _, lineXML := range fragment.Lines.Line {
				ln, _ := strconv.Atoi(lineXML.Number)
				if ln > maxLineNumInFile {
					maxLineNumInFile = ln
				}
			}
			for _, methodXML := range fragment.Methods.Method {
				for _, lineXML := range methodXML.Lines.Line {
					ln, _ := strconv.Atoi(lineXML.Number)
					if ln > maxLineNumInFile {
						maxLineNumInFile = ln
					}
				}
			}
		}

		mergedLineHits := make([]int, maxLineNumInFile+1)
		for i := range mergedLineHits {
			mergedLineHits[i] = -1
		}
		mergedBranches := make(map[int][]model.BranchCoverageDetail)

		for _, fragment := range fragmentsForFile {
			for _, lineXML := range fragment.Lines.Line {
				lineModel, _ := cp.processCoberturaLineXML(lineXML, sourceLinesForFile)
				lineNumber := lineModel.Number
				if lineNumber <= 0 || lineNumber > maxLineNumInFile {
					continue
				}
				if mergedLineHits[lineNumber] < 0 {
					mergedLineHits[lineNumber] = lineModel.Hits
				} else if lineModel.Hits > 0 {
					mergedLineHits[lineNumber] += lineModel.Hits
				} else if lineModel.Hits == 0 && mergedLineHits[lineNumber] == -1 { // if no data yet, and this fragment reports 0 hits for a coverable line
					mergedLineHits[lineNumber] = 0
				}

				if lineModel.IsBranchPoint {
					currentBranches, _ := mergedBranches[lineNumber]
					for _, newBranch := range lineModel.Branch {
						found := false
						for idx, existingBranch := range currentBranches {
							if existingBranch.Identifier == newBranch.Identifier {
								currentBranches[idx].Visits += newBranch.Visits
								found = true
								break
							}
						}
						if !found {
							currentBranches = append(currentBranches, newBranch)
						}
					}
					mergedBranches[lineNumber] = currentBranches
				}
			}

			for _, methodXML := range fragment.Methods.Method {
				methodModel, mErr := cp.processCoberturaMethodXML(methodXML, sourceLinesForFile, fragment.Name, config)
				if mErr != nil {
					continue
				}
				allMethodsForClassAcrossFiles = append(allMethodsForClassAcrossFiles, *methodModel)

				if methodModel.MethodMetrics != nil {
					currentCodeFile.MethodMetrics = append(currentCodeFile.MethodMetrics, methodModel.MethodMetrics...)
				}

				elementType := model.MethodElementType
				cleanedFullNameForElement := methodModel.DisplayName
				if strings.HasPrefix(cleanedFullNameForElement, "get_") || strings.HasPrefix(cleanedFullNameForElement, "set_") {
					elementType = model.PropertyElementType
				}
				var coverageQuotaForElement *float64
				if len(methodModel.Lines) > 0 && !math.IsNaN(methodModel.LineRate) && !math.IsInf(methodModel.LineRate, 0) {
					cq := methodModel.LineRate * 100.0
					coverageQuotaForElement = &cq
				}
				var shortNameForElement string
				if elementType == model.PropertyElementType {
					shortNameForElement = cleanedFullNameForElement
				} else {
					shortNameForElement = utils.GetShortMethodName(cleanedFullNameForElement)
				}
				codeElem := model.CodeElement{
					Name:          shortNameForElement,
					FullName:      cleanedFullNameForElement,
					Type:          elementType,
					FirstLine:     methodModel.FirstLine,
					LastLine:      methodModel.LastLine,
					CoverageQuota: coverageQuotaForElement,
				}
				allCodeElementsForFileFragment = append(allCodeElementsForFileFragment, codeElem)
			}
		}

		var finalLinesForFile []model.Line
		var fileCoveredLines, fileCoverableLines, fileBranchesCovered, fileBranchesValid int
		for lineNum := 1; lineNum <= maxLineNumInFile; lineNum++ {
			lineContent := ""
			if lineNum > 0 && lineNum <= len(sourceLinesForFile) {
				lineContent = sourceLinesForFile[lineNum-1]
			}
			currentLine := model.Line{
				Number:                   lineNum,
				Hits:                     mergedLineHits[lineNum],
				Content:                  lineContent,
				LineCoverageByTestMethod: make(map[string]int),
			}
			if branches, ok := mergedBranches[lineNum]; ok && len(branches) > 0 {
				currentLine.IsBranchPoint = true
				currentLine.Branch = branches
				for _, b := range branches {
					if b.Visits > 0 {
						currentLine.CoveredBranches++
					}
					currentLine.TotalBranches++
				}
			}

			if currentLine.Hits < 0 {
				currentLine.LineVisitStatus = model.NotCoverable
			} else if currentLine.IsBranchPoint {
				if currentLine.TotalBranches == 0 {
					currentLine.LineVisitStatus = model.NotCoverable
				} else if currentLine.CoveredBranches == currentLine.TotalBranches {
					currentLine.LineVisitStatus = model.Covered
				} else if currentLine.CoveredBranches > 0 {
					currentLine.LineVisitStatus = model.PartiallyCovered
				} else {
					currentLine.LineVisitStatus = model.NotCovered
				}
			} else if currentLine.Hits > 0 {
				currentLine.LineVisitStatus = model.Covered
			} else {
				currentLine.LineVisitStatus = model.NotCovered
			}

			finalLinesForFile = append(finalLinesForFile, currentLine)
			if currentLine.Hits >= 0 {
				fileCoverableLines++
				if currentLine.Hits > 0 {
					fileCoveredLines++
				}
			}
			fileBranchesCovered += currentLine.CoveredBranches
			fileBranchesValid += currentLine.TotalBranches
		}
		currentCodeFile.Lines = finalLinesForFile
		currentCodeFile.CoveredLines = fileCoveredLines
		currentCodeFile.CoverableLines = fileCoverableLines

		currentCodeFile.MethodMetrics = utils.DistinctBy(currentCodeFile.MethodMetrics, func(mm model.MethodMetric) string { return mm.Name + fmt.Sprintf("_%d", mm.Line) })
		currentCodeFile.CodeElements = utils.DistinctBy(allCodeElementsForFileFragment, func(ce model.CodeElement) string { return ce.FullName + fmt.Sprintf("_%d", ce.FirstLine) })
		utils.SortByLineAndName(currentCodeFile.CodeElements)

		classModel.Files = append(classModel.Files, currentCodeFile)
		assemblyProcessedFilePaths[currentCodeFile.Path] = struct{}{}
		classProcessedFilePaths[currentCodeFile.Path] = struct{}{}

		classModel.LinesCovered += fileCoveredLines
		classModel.LinesValid += fileCoverableLines
		if fileBranchesValid > 0 || fileBranchesCovered > 0 {
			hasClassBranchData = true
			totalClassBranchesCovered += fileBranchesCovered
			totalClassBranchesValid += fileBranchesValid
		}
	}

	if hasClassBranchData {
		classModel.BranchesCovered = &totalClassBranchesCovered
		classModel.BranchesValid = &totalClassBranchesValid
	}

	for path := range classProcessedFilePaths {
		if lineCount, ok := uniqueFilePathsForGrandTotalLines[path]; ok {
			classModel.TotalLines += lineCount
		}
	}

	classModel.Methods = utils.DistinctBy(allMethodsForClassAcrossFiles, func(m model.Method) string { return m.DisplayName + fmt.Sprintf("_%d", m.FirstLine) })
	utils.SortByLineAndName(classModel.Methods)

	var coveredM, fullyCoveredM, totalM int
	if classModel.Methods != nil {
		totalM = len(classModel.Methods)
		for _, method := range classModel.Methods {
			methodHasCoverableLines := false
			methodIsFullyCovered := true
			if len(method.Lines) == 0 {
				methodIsFullyCovered = false
			} else {
				atLeastOneLineCoveredInMethod := false
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
				if !methodHasCoverableLines && totalM > 0 {
					methodIsFullyCovered = false
				}
			}
			if methodIsFullyCovered && methodHasCoverableLines {
				fullyCoveredM++
			} else if !methodHasCoverableLines && len(method.Lines) == 0 {
				fullyCoveredM++
			}
		}
	}
	classModel.CoveredMethods = coveredM
	classModel.FullyCoveredMethods = fullyCoveredM
	classModel.TotalMethods = totalM

	if classModel.Methods != nil {
		for _, method := range classModel.Methods {
			if !math.IsNaN(method.Complexity) && !math.IsInf(method.Complexity, 0) {
				classModel.Metrics["Cyclomatic complexity"] += method.Complexity
			}
			if method.MethodMetrics != nil {
				for _, methodMetric := range method.MethodMetrics {
					for _, metric := range methodMetric.Metrics {
						if metric.Name == "" || metric.Name == "Cyclomatic complexity" {
							continue
						}
						if valFloat, ok := metric.Value.(float64); ok {
							if !math.IsNaN(valFloat) && !math.IsInf(valFloat, 0) {
								classModel.Metrics[metric.Name] += valFloat
							}
						}
					}
				}
			}
		}
	}
	return &classModel, nil
}

func (cp *CoberturaParser) processCoberturaLineXML(lineXML inputxml.LineXML, sourceLines []string) (model.Line, fileProcessingMetricsCobertura) {
	metrics := fileProcessingMetricsCobertura{}
	lineNumber, _ := strconv.Atoi(lineXML.Number)

	line := model.Line{
		Number:            lineNumber,
		Hits:              cp.parseInt(lineXML.Hits),
		IsBranchPoint:     strings.EqualFold(lineXML.Branch, "true"),
		ConditionCoverage: lineXML.ConditionCoverage,
		Branch:            make([]model.BranchCoverageDetail, 0),
	}

	if lineNumber > 0 && lineNumber <= len(sourceLines) {
		line.Content = sourceLines[lineNumber-1]
	} else {
		line.Content = ""
	}

	if line.IsBranchPoint {
		conditionCoverageAttr := lineXML.ConditionCoverage
		matches := conditionCoverageRegexCobertura.FindStringSubmatch(conditionCoverageAttr)

		if len(matches) > 0 {
			groupNames := conditionCoverageRegexCobertura.SubexpNames()
			var coveredStr, totalStr string
			for i, name := range groupNames {
				if i > 0 && name != "" {
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

					for i := 0; i < line.TotalBranches; i++ {
						visits := 0
						if i < line.CoveredBranches {
							visits = 1
						}
						identifier := fmt.Sprintf("%d_%d", lineNumber, i)
						if i < len(lineXML.Conditions.Condition) {
							identifier = lineXML.Conditions.Condition[i].Number
						}
						line.Branch = append(line.Branch, model.BranchCoverageDetail{
							Identifier: identifier, Visits: visits,
						})
					}
				} else {
					cp.setFallbackBranchDataCobertura(&line)
				}
			} else {
				if len(lineXML.Conditions.Condition) > 0 {
					for _, conditionXMLElement := range lineXML.Conditions.Condition {
						branchDetail := model.BranchCoverageDetail{Identifier: conditionXMLElement.Number, Visits: 0}
						if strings.HasPrefix(conditionXMLElement.Coverage, "100") {
							branchDetail.Visits = 1
						}
						line.Branch = append(line.Branch, branchDetail)
						if branchDetail.Visits > 0 {
							line.CoveredBranches++
						}
						line.TotalBranches++
					}
				} else {
					cp.setFallbackBranchDataCobertura(&line)
				}
			}
		} else if len(lineXML.Conditions.Condition) > 0 {
			for _, conditionXMLElement := range lineXML.Conditions.Condition {
				branchDetail := model.BranchCoverageDetail{Identifier: conditionXMLElement.Number, Visits: 0}
				if strings.HasPrefix(conditionXMLElement.Coverage, "100") {
					branchDetail.Visits = 1
				}
				line.Branch = append(line.Branch, branchDetail)
				if branchDetail.Visits > 0 {
					line.CoveredBranches++
				}
				line.TotalBranches++
			}
		} else {
			cp.setFallbackBranchDataCobertura(&line)
		}
	}

	metrics.branchesCovered = line.CoveredBranches
	metrics.branchesValid = line.TotalBranches
	return line, metrics
}

func (cp *CoberturaParser) setFallbackBranchDataCobertura(line *model.Line) {
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

// processCoberturaMethodXML now accepts parser.ParserConfig.
func (cp *CoberturaParser) processCoberturaMethodXML(
	methodXML inputxml.MethodXML,
	sourceLines []string,
	classNameFromXML string,
	config parser.ParserConfig,
) (*model.Method, error) {
	rawMethodName := methodXML.Name
	rawSignature := methodXML.Signature
	fullNameFromXML := rawMethodName + rawSignature

	extractedFullNameForDisplay := cp.extractMethodNameCobertura(fullNameFromXML, classNameFromXML, config.Settings().RawMode)

	if strings.Contains(extractedFullNameForDisplay, "__") && lambdaMethodNameRegexCobertura.MatchString(extractedFullNameForDisplay) {
		return nil, fmt.Errorf("method '%s' (extracted: '%s') is a lambda and skipped", fullNameFromXML, extractedFullNameForDisplay)
	}

	method := model.Method{
		Name:        rawMethodName,
		Signature:   rawSignature,
		DisplayName: extractedFullNameForDisplay,
		Complexity:  cp.parseFloat(methodXML.Complexity),
	}

	cp.processMethodLinesCobertura(methodXML, &method, sourceLines)
	cp.populateStandardMethodMetricsCobertura(&method, extractedFullNameForDisplay)

	return &method, nil
}

func (cp *CoberturaParser) processMethodLinesCobertura(methodXML inputxml.MethodXML, method *model.Method, sourceLines []string) {
	minLine := math.MaxInt32
	maxLine := 0
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
		lineModel, lineMetricsStats := cp.processCoberturaLineXML(lineXML, sourceLines)
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
		method.FirstLine, method.LastLine = 0, 0
	} else {
		method.FirstLine, method.LastLine = minLine, maxLine
	}
	method.LineRate = 0.0
	if methodLinesValid > 0 {
		method.LineRate = float64(methodLinesCovered) / float64(methodLinesValid)
	}
	method.BranchRate = 0.0
	if methodBranchesValid > 0 {
		method.BranchRate = float64(methodBranchesCovered) / float64(methodBranchesValid)
	}
}

func (cp *CoberturaParser) populateStandardMethodMetricsCobertura(method *model.Method, metricGroupNameForTable string) {
	method.MethodMetrics = []model.MethodMetric{}
	shortMetricName := utils.GetShortMethodName(metricGroupNameForTable)
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
	branchCoveragePercentage := method.BranchRate * 100.0
	if !math.IsNaN(branchCoveragePercentage) {
		method.MethodMetrics = append(method.MethodMetrics, model.MethodMetric{
			Name: shortMetricName, Line: method.FirstLine,
			Metrics: []model.Metric{{Name: "Branch coverage", Value: branchCoveragePercentage, Status: model.StatusOk}},
		})
	}
	crapScoreValue := cp.calculateCrapScoreCobertura(method.LineRate, method.Complexity)
	if !math.IsNaN(crapScoreValue) {
		method.MethodMetrics = append(method.MethodMetrics, model.MethodMetric{
			Name: shortMetricName, Line: method.FirstLine,
			Metrics: []model.Metric{{Name: "CrapScore", Value: crapScoreValue, Status: model.StatusOk}},
		})
	}
}

func (cp *CoberturaParser) calculateCrapScoreCobertura(coverage float64, complexity float64) float64 {
	if math.IsNaN(coverage) || math.IsInf(coverage, 0) || coverage < 0 || coverage > 1 {
		coverage = 0
	}
	if math.IsNaN(complexity) || math.IsInf(complexity, 0) || complexity < 0 {
		return math.NaN()
	}
	uncoveredRatio := 1.0 - coverage
	return (math.Pow(complexity, 2) * math.Pow(uncoveredRatio, 3)) + complexity
}

func (cp *CoberturaParser) extractMethodNameCobertura(methodNamePlusSignature, classNameFromXML string, rawMode bool) string {
	combinedNameForContext := classNameFromXML + methodNamePlusSignature
	if strings.Contains(methodNamePlusSignature, "|") && (strings.Contains(classNameFromXML, ">g__") || strings.Contains(methodNamePlusSignature, ">g__")) {
		match := localFunctionMethodNameRegexCobertura.FindStringSubmatch(combinedNameForContext)
		nameIndex := localFunctionMethodNameRegexCobertura.SubexpIndex("NestedMethodName")
		if len(match) > nameIndex && match[nameIndex] != "" {
			if nestedName := match[nameIndex]; nestedName != "" {
				return nestedName + "()"
			}
		}
	}
	if strings.HasSuffix(methodNamePlusSignature, "MoveNext()") {
		match := compilerGeneratedMethodNameRegexCobertura.FindStringSubmatch(combinedNameForContext)
		nameIndex := compilerGeneratedMethodNameRegexCobertura.SubexpIndex("CompilerGeneratedName")
		if len(match) > nameIndex && match[nameIndex] != "" {
			return match[nameIndex] + "()"
		}
	}
	return methodNamePlusSignature
}

func (cp *CoberturaParser) formatDisplayNameCobertura(rawCoberturaClassName string, rawMode bool) string {
	if rawMode {
		return rawCoberturaClassName
	}
	nameForDisplay := nestedTypeSeparatorRegexCobertura.ReplaceAllString(rawCoberturaClassName, ".")
	match := genericClassRegexCobertura.FindStringSubmatch(nameForDisplay)
	baseDisplayName, genericSuffix := nameForDisplay, ""
	if match != nil {
		baseDisplayName = match[1]
		if argCount, _ := strconv.Atoi(match[2]); argCount > 0 {
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
			genericSuffix = sb.String()
		}
	}
	return baseDisplayName + genericSuffix
}

func (cp *CoberturaParser) logicalClassNameCobertura(raw string, rawMode bool) string {
	if rawMode {
		return raw
	}
	if i := strings.IndexAny(raw, "/$+"); i != -1 {
		return raw[:i]
	}
	return raw
}

func (cp *CoberturaParser) isFilteredRawClassNameCobertura(rawName string, rawMode bool) bool {
	if rawMode {
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
