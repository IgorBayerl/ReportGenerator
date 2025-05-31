package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath" // New import
	"strings"
	"time"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/analyzer"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/glob" // New import
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/logging"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reportconfig"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporter/htmlreport"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporter/textsummary"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporting"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/settings"
)

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

	// Changed from -report to -reports to align with C# and common usage for multiple files/patterns
	reportsPatternsStr := flag.String("report", "", "Coverage report file paths or patterns (semicolon-separated, e.g., \"./coverage/*.xml;./more.xml\")")
	outputDir := flag.String("output", "coverage-report", "Output directory for reports")
	reportTypesStr := flag.String("reporttypes", "TextSummary", "Report types to generate (comma-separated: TextSummary,Html)")
	sourceDirsStr := flag.String("sourcedirs", "", "Source directories (comma-separated)")
	tag := flag.String("tag", "", "Optional tag (e.g., build number)")
	title := flag.String("title", "", "Optional report title. Default: 'Coverage Report'")
	verbosityStr := flag.String("verbosity", "Info", "Logging verbosity level (Verbose, Info, Warning, Error, Off)")

	flag.Parse()

	if *reportsPatternsStr == "" {
		// Updated usage message
		fmt.Println("Usage: go_report_generator -reports <file/pattern>[;<file/pattern>...] [-output <dir>] ...")
		fmt.Println("\nReport types:")
		for rt := range supportedReportTypes {
			fmt.Printf("  %s\n", rt)
		}
		fmt.Println("\nVerbosity levels: Verbose, Info, Warning, Error, Off")
		os.Exit(1)
	}

	// Expand report file patterns
	reportFilePatterns := strings.Split(*reportsPatternsStr, ";") // Semicolon separated patterns
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
			// Only add to invalidPatterns if it wasn't an error but just found no files.
			// C# ReportConfiguration stores these separately.
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
					invalidPatterns = append(invalidPatterns, file) // Add specific file that failed stat
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
		fmt.Println("\nSupported report types:")
		for rt := range supportedReportTypes {
			fmt.Printf("  %s\n", rt)
		}
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

	currentSettings := settings.NewSettings()
	actualTitle := *title
	if actualTitle == "" {
		actualTitle = "Coverage Report"
	}

	// Pass the expanded actualReportFiles and any invalidPatterns to the configuration
	reportConfig := reportconfig.NewReportConfiguration(
		actualReportFiles, // Now a list of actual files
		*outputDir,
		sourceDirsList,
		"", // historyDir
		requestedTypes,
		*tag,
		actualTitle,
		verbosity,
		invalidPatterns, // Pass invalid patterns
	)

	reportCtx := reporting.NewReportContext(reportConfig, currentSettings)

	// The rest of the logic will now iterate over `actualReportFiles` if multiple reports need to be merged.
	// For now, assuming CoberturaParser and Analyzer handle one report at a time,
	// or that a higher-level merge strategy is needed (like in C# ReportGenerator).
	// This example will process the first valid report file for simplicity of demonstration.
	// A full multi-report merge is a larger architectural change.

	if len(actualReportFiles) == 0 {
		fmt.Fprintf(os.Stderr, "Error: No report files to process.\n")
		os.Exit(1)
	}

	// For now, process only the first report file found.
	// TODO: Implement merging strategy for multiple report files.
	fmt.Printf("Processing coverage report: %s\n", actualReportFiles[0])
	rawReport, sourceDirsFromParser, err := parser.ParseCoberturaXML(actualReportFiles[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse Cobertura XML from %s: %v\n", actualReportFiles[0], err)
		os.Exit(1)
	}

	// Update reportConfig if sourceDirs came from parser and were not initially set
	// This logic should be outside the loop if you process multiple files and merge configurations.
	if len(reportConfig.SourceDirectories()) == 0 && len(sourceDirsFromParser) > 0 {
		fmt.Printf("Note: Cobertura report specified source directories: %v. Using these as source directories.\n", sourceDirsFromParser)
		reportConfig = reportconfig.NewReportConfiguration( // Recreate or update the config
			actualReportFiles,
			*outputDir,
			sourceDirsFromParser,
			"",
			requestedTypes,
			*tag,
			actualTitle,
			verbosity,
			invalidPatterns,
		)
		// Update reportCtx as well if it's already been used with the old config
		reportCtx = reporting.NewReportContext(reportConfig, currentSettings)
	}

	fmt.Printf("Cobertura XML parsed successfully.\n")
	summaryResult, err := analyzer.Analyze(rawReport, reportConfig.SourceDirectories())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to analyze coverage data: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Coverage data analyzed.\n")
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
	fmt.Printf("\nReport generation completed in %.2f seconds\n", time.Since(start).Seconds())
}
