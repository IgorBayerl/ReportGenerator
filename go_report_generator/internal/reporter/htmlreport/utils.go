package htmlreport

import (
	"fmt"
	"strings"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/utils"
)

const (
	lineVisitStatusNotCoverable     = 0
	lineVisitStatusCovered          = 1
	lineVisitStatusNotCovered       = 2
	lineVisitStatusPartiallyCovered = 3
)

const maxFilenameLengthBase = 95

func countTotalClasses(assemblies []model.Assembly) int {
	count := 0
	for _, asm := range assemblies {
		count += len(asm.Classes)
	}
	return count
}

func countUniqueFiles(assemblies []model.Assembly) int {
	if len(assemblies) == 0 {
		return 0
	}

	var allFiles []model.CodeFile
	for _, asm := range assemblies {
		for _, cls := range asm.Classes {
			allFiles = append(allFiles, cls.Files...)
		}
	}

	distinctFiles := utils.DistinctBy(allFiles, func(file model.CodeFile) string {
		return file.Path // Assuming Path is the unique key
	})

	return len(distinctFiles)
}

func (b *HtmlReportBuilder) getClassReportFilename(assemblyShortName, className string, existingFilenames map[string]struct{}) string {
	// The generateUniqueFilename function (from internal/reporter/htmlreport/utils.go)
	// now handles all the logic for processing className, sanitizing, truncating,
	// and ensuring uniqueness with a counter.
	return generateUniqueFilename(assemblyShortName, className, existingFilenames)
}

func determineLineVisitStatus(hits int, isBranchPoint bool, coveredBranches int, totalBranches int) model.LineVisitStatus { // Changed return type
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

func lineVisitStatusToString(status model.LineVisitStatus) string { // Changed parameter type
	switch status {
	case model.Covered: // Use model.Covered
		return "green"
	case model.NotCovered: // Use model.NotCovered
		return "red"
	case model.PartiallyCovered: // Use model.PartiallyCovered
		return "orange"
	default: // model.NotCoverable
		return "gray"
	}
}

// generateUniqueFilename creates a sanitized and unique HTML filename for a class.
// It takes assembly and class names, and a map of existing filenames to ensure uniqueness.
// The existingFilenames map is modified by this function.
func generateUniqueFilename(
	assemblyShortName string,
	className string,
	existingFilenames map[string]struct{},
) string {
	namePart := className
	if lastDot := strings.LastIndex(className, "."); lastDot != -1 {
		namePart = className[lastDot+1:]
	}

	processedClassName := namePart
	if strings.ToLower(namePart) == "js" && strings.HasSuffix(strings.ToLower(className), ".js") {
		if strings.HasSuffix(strings.ToLower(namePart), ".js") {
			processedClassName = namePart[:len(namePart)-3]
		}
	} else if strings.HasSuffix(strings.ToLower(namePart), ".js") {
		processedClassName = namePart[:len(namePart)-3]
	}

	separators := []string{"+", "/", "::"}
	for _, sep := range separators {
		if strings.Contains(processedClassName, sep) {
			parts := strings.Split(processedClassName, sep)
			processedClassName = parts[len(parts)-1]
		}
	}

	baseName := assemblyShortName + processedClassName
	sanitizedName := utils.ReplaceInvalidPathChars(baseName) // Uses the centralized utility

	if len(sanitizedName) > maxFilenameLengthBase {
		if maxFilenameLengthBase > 50 {
			sanitizedName = sanitizedName[:50] + sanitizedName[len(sanitizedName)-(maxFilenameLengthBase-50):]
		} else {
			sanitizedName = sanitizedName[:maxFilenameLengthBase]
		}
	}

	fileName := sanitizedName + ".html"
	counter := 1
	normalizedFileNameToCheck := strings.ToLower(fileName)

	_, exists := existingFilenames[normalizedFileNameToCheck]
	for exists {
		counter++
		fileName = fmt.Sprintf("%s%d.html", sanitizedName, counter)
		normalizedFileNameToCheck = strings.ToLower(fileName)
		_, exists = existingFilenames[normalizedFileNameToCheck]
	}

	existingFilenames[normalizedFileNameToCheck] = struct{}{}
	return fileName
}
