package htmlreport

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
)

var (
	sanitizeFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
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
	uniqueFiles := make(map[string]bool)
	for _, asm := range assemblies {
		for _, cls := range asm.Classes {
			for _, f := range cls.Files {
				uniqueFiles[f.Path] = true
			}
		}
	}
	return len(uniqueFiles)
}



func (b *HtmlReportBuilder) getClassReportFilename(assemblyShortName, className string, existingFilenames map[string]struct{}) string {
	processedClassName := className
	if lastDot := strings.LastIndex(className, "."); lastDot != -1 {
		processedClassName = className[lastDot+1:]
	}
	if strings.HasSuffix(strings.ToLower(processedClassName), ".js") {
		processedClassName = processedClassName[:len(processedClassName)-3]
	}
	baseName := assemblyShortName + "" + processedClassName
	sanitizedName := sanitizeFilenameChars.ReplaceAllString(baseName, "")
	maxLengthBase := 95
	if len(sanitizedName) > maxLengthBase {
		if maxLengthBase > 50 {
			sanitizedName = sanitizedName[:50] + sanitizedName[len(sanitizedName)-(maxLengthBase-50):]
		} else {
			sanitizedName = sanitizedName[:maxLengthBase]
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




// --- Helper functions for determining line status and simple counts ---
func determineLineVisitStatus(hits int, isBranchPoint bool, coveredBranches int, totalBranches int) int {
	if hits < 0 {
		return lineVisitStatusNotCoverable
	}
	if isBranchPoint {
		if totalBranches == 0 {
			return lineVisitStatusNotCoverable
		}
		if coveredBranches == totalBranches {
			return lineVisitStatusCovered
		}
		if coveredBranches > 0 {
			return lineVisitStatusPartiallyCovered
		}
		return lineVisitStatusNotCovered
	}
	if hits > 0 {
		return lineVisitStatusCovered
	}
	return lineVisitStatusNotCovered
}

func lineVisitStatusToString(status int) string {
	switch status {
	case lineVisitStatusCovered:
		return "green"
	case lineVisitStatusNotCovered:
		return "red"
	case lineVisitStatusPartiallyCovered:
		return "orange"
	default: // lineVisitStatusNotCoverable
		return "gray"
	}
}


// generateUniqueFilename creates a sanitized and unique HTML filename for a class.
// It takes assembly and class names, and a map of existing filenames to ensure uniqueness.
// The existingFilenames map is modified by this function.
func generateUniqueFilename( // Renamed to lowercase
	assemblyShortName string,
	className string,
	existingFilenames map[string]struct{},
) string {
	// 1. Determine the effective class name part (after last namespace separator)
	namePart := className
	if lastDot := strings.LastIndex(className, "."); lastDot != -1 {
		namePart = className[lastDot+1:]
	}

	// 2. Handle specific ".js" suffix if it's the entirety of namePart
	processedClassName := namePart
	if strings.ToLower(namePart) == "js" && strings.HasSuffix(strings.ToLower(className), ".js") {
	    // This case handles "Namespace.js" -> "" or "js.js" -> "js"
        // If the original full class name ended with ".js" AND the part after the last dot is just "js",
        // then we effectively treat the class name part as empty or strip the .js from the segment.
        // Let's be more direct: if namePart is "js", just use it.
        // The original C# logic might be more nuanced for specific tools that output ".js" classes.
        // For "MyClass.js" -> namePart is "MyClass.js".
        // For "MyNamespace.MyClass.js" -> namePart is "MyClass.js".
        if strings.HasSuffix(strings.ToLower(namePart), ".js") {
             processedClassName = namePart[:len(namePart)-3]
        }

	} else if strings.HasSuffix(strings.ToLower(namePart), ".js") {
        // General case: if namePart ends with .js (e.g. "SomeFile.js"), strip it.
		processedClassName = namePart[:len(namePart)-3]
	}


	// 3. Further simplify processedClassName by taking the segment after common C# nested type separators
    // This helps with "SomeClass::Sub/Inner" -> "Inner"
    separators := []string{"+", "/", "::"} // Order might matter if they can be combined
    for _, sep := range separators {
        if strings.Contains(processedClassName, sep) {
            parts := strings.Split(processedClassName, sep)
            processedClassName = parts[len(parts)-1]
        }
    }

	baseName := assemblyShortName + processedClassName
	sanitizedName := sanitizeFilenameChars.ReplaceAllString(baseName, "")

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
		fileName = fmt.Sprintf("%s%d.html", sanitizedName, counter) // Use the original-case sanitizedName for the actual filename
		normalizedFileNameToCheck = strings.ToLower(fileName)
		_, exists = existingFilenames[normalizedFileNameToCheck]
	}

	existingFilenames[normalizedFileNameToCheck] = struct{}{} // Store lowercase for consistent checking
	return fileName
}
