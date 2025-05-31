package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/analyzer"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/glob"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/logging"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser"             // Import the main parser package
	_ "github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser/cobertura" // Import for side-effect: registers CoberturaParser
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reportconfig"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporter/htmlreport"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporter/textsummary"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporting"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/settings"
)

// ... (validateReportTypes and supportedReportTypes remain the same) ...
// supportedReportTypes defines the available report formats
var supportedReportTypes = map[string]bool{
	"TextSummary": true,
	"Html":        true,
}

// validateReportTypes checks if all requested report types are supported
func validateReportTypes(types []string) error {
	for _, t := range types {
		trimmedType := strings.TrimSpace(t)
		if !supportedReportTypes[trimmedType] {
			return fmt.Errorf("unsupported report type: %s", trimmedType)
		}
	}
	return nil
}

func main() {
	start := time.Now()

	reportsPatternsStr := flag.String("report", "", "Coverage report file paths or patterns (semicolon-separated, e.g., \"./coverage/*.xml;./more.xml\")")
	outputDir := flag.String("output", "coverage-report", "Output directory for reports")
	reportTypesStr := flag.String("reporttypes", "TextSummary", "Report types to generate (comma-separated: TextSummary,Html)")
	sourceDirsStr := flag.String("sourcedirs", "", "Source directories (comma-separated)")
	tag := flag.String("tag", "", "Optional tag (e.g., build number)")
	title := flag.String("title", "", "Optional report title. Default: 'Coverage Report'")
	verbosityStr := flag.String("verbosity", "Info", "Logging verbosity level (Verbose, Info, Warning, Error, Off)")

	flag.Parse()

	// ... (CLI parsing and report file globbing remain mostly the same) ...
	if *reportsPatternsStr == "" {
		fmt.Println("Usage: go_report_generator -reports <file/pattern>[;<file/pattern>...] [-output <dir>] ...")
		fmt.Println("\nReport types:")
		for rt := range supportedReportTypes {
			fmt.Printf("  %s\n", rt)
		}
		fmt.Println("\nVerbosity levels: Verbose, Info, Warning, Error, Off")
		os.Exit(1)
	}

	reportFilePatterns := strings.Split(*reportsPatternsStr, ";")
	var actualReportFiles []string
	var invalidPatterns []string
	seenFiles := make(map[string]struct{})

	for _, pattern := range reportFilePatterns {
		trimmedPattern := strings.TrimSpace(pattern)
		if trimmedPattern == "" {
			continue
		}
		expandedFiles, err := glob.GetFiles(trimmedPattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Error expanding report file pattern '%s': %v\n", trimmedPattern, err)
			invalidPatterns = append(invalidPatterns, trimmedPattern)
			continue
		}
		if len(expandedFiles) == 0 {
			fmt.Fprintf(os.Stderr, "Warning: No files found for report pattern '%s'\n", trimmedPattern)
			invalidPatterns = append(invalidPatterns, trimmedPattern)
		}
		for _, file := range expandedFiles {
			absFile, _ := filepath.Abs(file)
			if _, found := seenFiles[absFile]; !found {
				if stat, err := os.Stat(absFile); err == nil && !stat.IsDir() {
					actualReportFiles = append(actualReportFiles, absFile)
					seenFiles[absFile] = struct{}{}
				} else if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Could not stat file from pattern '%s': %s - %v\n", trimmedPattern, absFile, err)
					invalidPatterns = append(invalidPatterns, file)
				}
			}
		}
	}

	if len(actualReportFiles) == 0 {
		fmt.Fprintf(os.Stderr, "Error: No valid report files found after expanding patterns.\n")
		if len(invalidPatterns) > 0 {
			fmt.Fprintf(os.Stderr, "Patterns that yielded no files or errors: %s\n", strings.Join(invalidPatterns, ", "))
		}
		os.Exit(1)
	}

	requestedTypes := strings.Split(*reportTypesStr, ",")
	if err := validateReportTypes(requestedTypes); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var sourceDirsList []string
	if *sourceDirsStr != "" {
		sourceDirsList = strings.Split(*sourceDirsStr, ",")
		for i, dir := range sourceDirsList {
			sourceDirsList[i] = strings.TrimSpace(dir)
		}
	}

	var verbosity logging.VerbosityLevel
	// ... (verbosity parsing remains the same) ...
	switch strings.ToLower(*verbosityStr) {
	case "verbose":
		verbosity = logging.Verbose
	case "info":
		verbosity = logging.Info
	case "warning":
		verbosity = logging.Warning
	case "error":
		verbosity = logging.Error
	case "off":
		verbosity = logging.Off
	default:
		fmt.Fprintf(os.Stderr, "Error: Invalid verbosity level '%s'. Valid levels are Verbose, Info, Warning, Error, Off.\n", *verbosityStr)
		os.Exit(1)
	}


	currentSettings := settings.NewSettings() // Create settings instance
	actualTitle := *title
	if actualTitle == "" {
		actualTitle = "Coverage Report"
	}

    // Collect raw filter strings from CLI (or config file if you add that later)
    // For now, assuming they are not yet CLI flags, so passing empty slices.
    // If you add CLI flags for filters, parse them here.
    assemblyFilterStrings := []string{} // Placeholder: populate from CLI flags if available
    classFilterStrings := []string{}    // Placeholder
    fileFilterStrings := []string{}     // Placeholder
    rhAssemblyFilterStrings := []string{}// Placeholder
    rhClassFilterStrings := []string{}  // Placeholder


	reportConfig, err := reportconfig.NewReportConfiguration( // Updated call
		actualReportFiles,
		*outputDir,
		sourceDirsList,
		"", // historyDir
		requestedTypes,
		*tag,
		actualTitle,
		verbosity,
		invalidPatterns,
        assemblyFilterStrings,
        classFilterStrings,
        fileFilterStrings,
        rhAssemblyFilterStrings,
        rhClassFilterStrings,
        currentSettings, // Pass settings
	)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error creating report configuration: %v\n", err)
        os.Exit(1)
    }

	// Create ReportContext once, after initial config and settings are ready.
	reportCtx := reporting.NewReportContext(reportConfig, currentSettings)


	var parserResults []*parser.ParserResult
	processedAllFilesSuccessfully := true

	for _, reportFile := range actualReportFiles {
		fmt.Printf("Attempting to parse report file: %s\n", reportFile)
		parserInstance, err := parser.FindParserForFile(reportFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v for file %s. Skipping.\n", err, reportFile)
			processedAllFilesSuccessfully = false
			continue
		}

		fmt.Printf("Using parser: %s for file %s\n", parserInstance.Name(), reportFile)
		// Pass reportCtx to the parser's Parse method
		result, err := parserInstance.Parse(reportFile, reportCtx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing file %s with %s: %v. Skipping.\n", reportFile, parserInstance.Name(), err)
			processedAllFilesSuccessfully = false
			continue
		}
		parserResults = append(parserResults, result)
		fmt.Printf("Successfully parsed: %s\n", reportFile)

		// Update reportConfig's source directories if new ones are found by the parser
		// This part needs careful thought: if reportConfig is immutable or if this update is safe.
		// For now, this assumes reportConfig's source dirs might be updated.
		// A better approach might be to collect all source dirs from all parserResults
		// and then update the single reportCtx or final SummaryResult.
		currentConfig := reportCtx.ReportConfiguration()
		if len(currentConfig.SourceDirectories()) == 0 && len(result.SourceDirectories) > 0 {
			fmt.Printf("Note: Report '%s' specified source directories: %v. Updating configuration for context.\n", reportFile, result.SourceDirectories)
			
            // Re-create or update the config and then the context
            // This is a bit clumsy; ideally, ReportConfiguration would be mutable or source dirs handled centrally.
			updatedConfig, confErr := reportconfig.NewReportConfiguration(
				actualReportFiles, *outputDir, result.SourceDirectories, "",
				requestedTypes, *tag, actualTitle, verbosity, invalidPatterns,
                assemblyFilterStrings, classFilterStrings, fileFilterStrings,
                rhAssemblyFilterStrings, rhClassFilterStrings, currentSettings,
			)
            if confErr != nil {
                 fmt.Fprintf(os.Stderr, "Error updating report configuration with new source dirs: %v\n", confErr)
            } else {
			    reportCtx = reporting.NewReportContext(updatedConfig, currentSettings) // Update the context for subsequent operations
            }
		}
	}

	if len(parserResults) == 0 {
		fmt.Fprintf(os.Stderr, "Error: No coverage reports could be parsed successfully.\n")
		os.Exit(1)
	}
	
	fmt.Printf("Merging %d parsed report(s)...\n", len(parserResults))
	// Pass reportCtx to MergeParserResults as well, it might need config/settings.
	summaryResult, err := analyzer.MergeParserResults(parserResults, reportCtx.ReportConfiguration())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to merge parser results: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Coverage data merged and analyzed.\n")

	// ... (Report Generation part remains the same, using reportCtx where needed for HtmlReportBuilder) ...
    fmt.Printf("Generating reports in: %s\n", *outputDir)
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	for _, reportType := range requestedTypes {
		fmt.Printf("Generating %s report...\n", reportType)
		switch strings.TrimSpace(reportType) {
		case "TextSummary":
			textBuilder := textsummary.NewTextReportBuilder(*outputDir)
			if err := textBuilder.CreateReport(summaryResult); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to generate text report: %v\n", err)
			}
		case "Html":
			htmlBuilder := htmlreport.NewHtmlReportBuilder(*outputDir, reportCtx) 
			if err := htmlBuilder.CreateReport(summaryResult); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to generate HTML report: %v\n", err)
			}
		}
	}

	if !processedAllFilesSuccessfully {
		fmt.Println("\nWarning: Some report files could not be processed. See messages above.")
	}
	fmt.Printf("\nReport generation completed in %.2f seconds\n", time.Since(start).Seconds())

}