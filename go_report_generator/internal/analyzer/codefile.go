// Path: internal/analyzer/codefile.go
package analyzer

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"strings"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/filereader"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/inputxml"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/utils"
)

// fileProcessingMetrics holds metrics aggregated during the processing of a single <class> XML element's file fragment.
type fileProcessingMetrics struct {
	linesCovered    int
	linesValid      int
	branchesCovered int
	branchesValid   int
}

// findFileInSourceDirs attempts to locate a file, first checking if it's absolute,
// then searching through the provided source directories.
func findFileInSourceDirs(relativePath string, sourceDirs []string) (string, error) {
	if filepath.IsAbs(relativePath) { // This will be false for "PartialClass.cs"
		if _, err := os.Stat(relativePath); err == nil {
			return relativePath, nil
		}
	}

	cleanedRelativePath := filepath.Clean(relativePath) // cleanedRelativePath will still be "PartialClass.cs"

	for _, dir := range sourceDirs { // dir will be "C:\www\ReportGenerator\Testprojects\CSharp\Project_DotNetCore\Test\"
		abs := filepath.Join(filepath.Clean(dir), cleanedRelativePath)
		// abs should become "C:\www\ReportGenerator\Testprojects\CSharp\Project_DotNetCore\Test\PartialClass.cs"
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
	}
	return "", fmt.Errorf("file %q not found in any source directory (%v) or as absolute path", relativePath, sourceDirs) // Log sourceDirs for debugging
}

// processCodeFileFragment processes the file-specific parts of an inputxml.ClassXML.
// It creates a model.CodeFile, populates its lines, and calculates metrics for this fragment.
// It updates uniqueFilePathsForGrandTotalLines with the total line count if the file is processed for the first time.
func processCodeFileFragment(
	classXML inputxml.ClassXML, // Pass the whole classXML
	sourceDirs []string,
	uniqueFilePathsForGrandTotalLines map[string]int,
) (*model.CodeFile, fileProcessingMetrics, error) {

	metrics := fileProcessingMetrics{}
	codeFile := model.CodeFile{Path: classXML.Filename, MethodMetrics: []model.MethodMetric{}, CodeElements: []model.CodeElement{}} // Initialize slices
	var sourceLines []string

	resolvedPath, err := findFileInSourceDirs(classXML.Filename, sourceDirs)
	if err == nil {
		codeFile.Path = resolvedPath
		sLines, readErr := filereader.ReadLinesInFile(resolvedPath)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not read content of source file %s: %v\n", resolvedPath, readErr)
			sourceLines = []string{}
		} else {
			sourceLines = sLines
		}

		// Populate TotalLines for the codeFile and update uniqueFilePathsForGrandTotalLines
		if lineCount, known := uniqueFilePathsForGrandTotalLines[resolvedPath]; known {
			codeFile.TotalLines = lineCount
		} else {
			if n, ferr := filereader.CountLinesInFile(resolvedPath); ferr == nil {
				uniqueFilePathsForGrandTotalLines[resolvedPath] = n
				codeFile.TotalLines = n
			} else {
				fmt.Fprintf(os.Stderr, "Warning: could not count lines in %s: %v\n", resolvedPath, ferr)
				// If we can't count lines, but could read them, use len(sourceLines) as a fallback.
				if readErr == nil {
					uniqueFilePathsForGrandTotalLines[resolvedPath] = len(sourceLines)
					codeFile.TotalLines = len(sourceLines)
				} else {
					codeFile.TotalLines = 0 // Or handle error more explicitly
				}
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "Warning: source file %s not found: %v\n", classXML.Filename, err)
		sourceLines = []string{}
		codeFile.TotalLines = 0 // File not found, so 0 total lines for this fragment's perspective
	}

	var fileFragmentCoveredLines, fileFragmentCoverableLines int
	for _, lineXML := range classXML.Lines.Line {
		lineModel, lineMetricsStats := processLineXML(lineXML, sourceLines)
		codeFile.Lines = append(codeFile.Lines, lineModel)

		// Only count lines with Hits >= 0 as coverable for line rate calculations
		if lineModel.Hits >= 0 {
			fileFragmentCoverableLines++
			metrics.linesValid++ // linesValid for summary usually means coverable lines
			if lineModel.Hits > 0 {
				fileFragmentCoveredLines++
				metrics.linesCovered++
			}
		}
		metrics.branchesCovered += lineMetricsStats.branchesCovered
		metrics.branchesValid += lineMetricsStats.branchesValid
	}
	codeFile.CoveredLines = fileFragmentCoveredLines
	codeFile.CoverableLines = fileFragmentCoverableLines

	// Populate MethodMetrics and CodeElements for the CodeFile
	for _, methodXML := range classXML.Methods.Method {
		methodModel, mErr := processMethodXML(methodXML, sourceLines, classXML.Name)
		if mErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: error processing method %s for file %s: %v\n", methodXML.Name, classXML.Filename, mErr)
			continue
		}

		// 1. Add MethodMetrics to CodeFile's MethodMetrics list
		if methodModel.MethodMetrics != nil {
			codeFile.MethodMetrics = append(codeFile.MethodMetrics, methodModel.MethodMetrics...)
		}

		// 2. Create and add CodeElement to CodeFile's CodeElements list
		elementType := model.MethodElementType
		// Use the cleaned DisplayName for property check and as base for CodeElement names
		cleanedFullNameForElement := methodModel.DisplayName
		if strings.HasPrefix(cleanedFullNameForElement, "get_") || strings.HasPrefix(cleanedFullNameForElement, "set_") {
			elementType = model.PropertyElementType
		}

		var coverageQuotaForElement *float64
		if len(methodModel.Lines) > 0 {
			if !math.IsNaN(methodModel.LineRate) && !math.IsInf(methodModel.LineRate, 0) {
				cq := methodModel.LineRate * 100.0
				coverageQuotaForElement = &cq
			}
		}

		// Determine the short name for display in the CodeElement itself
		var shortNameForElement string
		if elementType == model.PropertyElementType {
			// Properties usually don't have complex signatures that need shortening for CodeElement.Name
			shortNameForElement = cleanedFullNameForElement
		} else {
			shortNameForElement = utils.GetShortMethodName(cleanedFullNameForElement)
		}

		codeElement := model.CodeElement{
			Name:          shortNameForElement,       // Use the shortened version of the cleaned name
			FullName:      cleanedFullNameForElement, // Use the full cleaned name (e.g., MyMethod(System.String))
			Type:          elementType,
			FirstLine:     methodModel.FirstLine,
			LastLine:      methodModel.LastLine,
			CoverageQuota: coverageQuotaForElement,
		}
		codeFile.CodeElements = append(codeFile.CodeElements, codeElement)
	}

	// If CodeElements are structs, utils.SortByLineAndName(codeFile.CodeElements) is fine.
	utils.SortByLineAndName(codeFile.CodeElements)

	return &codeFile, metrics, nil
}
