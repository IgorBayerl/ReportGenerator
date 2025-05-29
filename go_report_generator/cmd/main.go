package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/analyzer"
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
	// Add other types like "Xml", "JsonSummary", "CsvSummary" etc. as you implement them
}

// validateReportTypes checks if all requested report types are supported
func validateReportTypes(types []string) error {
	for _, t := range types {
		// Trim whitespace in case of " Html "
		trimmedType := strings.TrimSpace(t)
		if !supportedReportTypes[trimmedType] {
			return fmt.Errorf("unsupported report type: %s", trimmedType)
		}
	}
	return nil
}

func main() {
	start := time.Now()

	reportPath := flag.String("report", "", "Path to Cobertura XML file")
	outputDir := flag.String("output", "coverage-report", "Output directory for reports")
	reportTypesStr := flag.String("reporttypes", "TextSummary", "Report types to generate (comma-separated: TextSummary,Html)")
	sourceDirsStr := flag.String("sourcedirs", "", "Source directories (comma-separated)")
	tag := flag.String("tag", "", "Optional tag (e.g., build number)")
	title := flag.String("title", "", "Optional report title. Default: 'Coverage Report'") // Default set in NewReportConfiguration
	verbosityStr := flag.String("verbosity", "Info", "Logging verbosity level (Verbose, Info, Warning, Error, Off)")

	flag.Parse()

	if *reportPath == "" {
		fmt.Println("Usage: go_report_generator -report <cobertura.xml> [-output <dir>] [-reporttypes <types>] [-sourcedirs <dirs>] [-tag <tag>] [-title <title>] [-verbosity <level>]")
		fmt.Println("\nReport types:")
		for rt := range supportedReportTypes {
			fmt.Printf("  %s\n", rt)
		}
		fmt.Println("\nVerbosity levels: Verbose, Info, Warning, Error, Off")
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
		for i, dir := range sourceDirsList { // Trim spaces from each source directory
			sourceDirsList[i] = strings.TrimSpace(dir)
		}
	}
	
	// Parse verbosity level
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
	// TODO: Set this verbosity level in a global logger factory if you implement one like in C#

	// 1. Create Settings
	currentSettings := settings.NewSettings()
	// Example: override from flags if you add them
	// currentSettings.MaximumDecimalPlacesForCoverageQuotas = *maxDecimalPlacesFlag

	// 2. Create IReportConfiguration
	actualTitle := *title
	if actualTitle == "" { // Default title if not provided
		actualTitle = "Coverage Report"
	}

	reportConfig := reportconfig.NewReportConfiguration(
		[]string{*reportPath},
		*outputDir,
		sourceDirsList,
		"", // historyDir
		requestedTypes,
		*tag,
		actualTitle,
		verbosity,
	)
	// You might want to populate filters here too if you add flags for them.
	// E.g., reportConfig.AssemblyFilterList = parseFilters(*assemblyFiltersFlag)

	// 3. Create IReportContext
	reportCtx := reporting.NewReportContext(reportConfig, currentSettings)

	fmt.Printf("Processing coverage report: %s\n", *reportPath)
	rawReport, sourceDirsFromParser, err := parser.ParseCoberturaXML(*reportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse Cobertura XML: %v\n", err)
		os.Exit(1)
	}
	if len(reportConfig.SourceDirectories()) == 0 && len(sourceDirsFromParser) > 0 {
		// If command line didn't specify source dirs, but Cobertura XML did, we might want to use them.
		// This requires ReportConfiguration to be mutable or to re-create it.
		// For now, we log this. The analyzer will use what's in reportConfig.
		fmt.Printf("Note: Cobertura report specified source directories: %v. Consider using -sourcedirs if needed.\n", sourceDirsFromParser)
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
		switch strings.TrimSpace(reportType) { // Trim space for robust matching
		case "TextSummary":
			textBuilder := textsummary.NewTextReportBuilder(*outputDir)
			if err := textBuilder.CreateReport(summaryResult); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to generate text report: %v\n", err)
			}
		case "Html":
			// Pass reportCtx to NewHtmlReportBuilder
			htmlBuilder := htmlreport.NewHtmlReportBuilder(*outputDir, reportCtx)
			if err := htmlBuilder.CreateReport(summaryResult); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to generate HTML report: %v\n", err)
			}
		}
	}
	fmt.Printf("\nReport generation completed in %.2f seconds\n", time.Since(start).Seconds())
}