package main

import (
	"flag"
	"fmt"
	"net/http" // Added
	"os"
	"strings"
	"time"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/analyzer"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporter/htmlreport"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporter/textsummary"
)

// supportedReportTypes defines the available report formats
var supportedReportTypes = map[string]bool{
	"TextSummary": true,
	"Html":        true,
}

// validateReportTypes checks if all requested report types are supported
func validateReportTypes(types []string) error {
	for _, t := range types {
		if !supportedReportTypes[t] {
			return fmt.Errorf("unsupported report type: %s", t)
		}
	}
	return nil
}

func main() {
	start := time.Now()

	// Parse command line arguments
	reportPath := flag.String("report", "", "Path to Cobertura XML file")
	outputDir := flag.String("output", "coverage-report", "Output directory for reports")
	reportTypes := flag.String("reporttypes", "TextSummary", "Report types to generate (comma-separated: TextSummary,Html)")
	servePort := flag.String("serve", "", "Serve the HTML report on the specified port (e.g., 8080). Disabled by default.") // Added
	flag.Parse()

	// Validate required arguments
	if *reportPath == "" {
		fmt.Println("Usage: go_report_generator -report <cobertura.xml> [-output <dir>] [-reporttypes <types>] [-serve <port>]")
		fmt.Println("\nReport types:")
		fmt.Println("  TextSummary  Generate a text summary report")
		fmt.Println("  Html         Generate an HTML coverage report")
		fmt.Println("\nServer (optional):")
		fmt.Println("  -serve <port> Serve the HTML report on the specified port (e.g., 8080)")
		os.Exit(1)
	}

	// Validate report types
	requestedTypes := strings.Split(*reportTypes, ",")
	if err := validateReportTypes(requestedTypes); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Println("\nSupported report types: TextSummary, Html")
		os.Exit(1)
	}

	htmlReportRequested := false
	for _, rt := range requestedTypes {
		if rt == "Html" {
			htmlReportRequested = true
			break
		}
	}

	if *servePort != "" && !htmlReportRequested {
		fmt.Fprintf(os.Stderr, "Error: -serve flag can only be used when 'Html' is among the requested report types.\n")
		os.Exit(1)
	}

	fmt.Printf("Processing coverage report: %s\n", *reportPath)

	// Step 1: Parse the Cobertura XML into raw input structures
	rawReport, sourceDirs, err := parser.ParseCoberturaXML(*reportPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse Cobertura XML: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Cobertura XML parsed successfully.\n")

	// Step 2: Analyze the raw report to produce the enriched model.SummaryResult
	summaryResult, err := analyzer.Analyze(rawReport, sourceDirs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to analyze coverage data: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Coverage data analyzed (using placeholder analyzer).\n")

	fmt.Printf("Generating reports in: %s\n", *outputDir)

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	// Generate each requested report type
	for _, reportType := range requestedTypes {
		fmt.Printf("Generating %s report...\n", reportType)

		switch reportType {
		case "TextSummary":
			textBuilder := textsummary.NewTextReportBuilder(*outputDir)
			if err := textBuilder.CreateReport(summaryResult); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to generate text report: %v\n", err)
				os.Exit(1)
			}
		case "Html":
			htmlBuilder := htmlreport.NewHTMLReport(*outputDir)
			if err := htmlBuilder.CreateReport(summaryResult); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to generate HTML report: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("HTML report generated successfully in %s\n", *outputDir)
		}
	}

	fmt.Printf("\nReport generation completed in %.2f seconds\n", time.Since(start).Seconds())

	// HTTP Server Logic
	if *servePort != "" {
		if !htmlReportRequested { // Double check, though already handled above
			fmt.Fprintf(os.Stderr, "Warning: -serve flag provided, but 'Html' report type was not generated. Server will not start.\n")
		} else {
			handler := http.FileServer(http.Dir(*outputDir))
			fmt.Printf("Serving HTML report from '%s' on http://localhost:%s\n", *outputDir, *servePort)
			err := http.ListenAndServe(":"+*servePort, handler)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to start HTTP server: %v\n", err)
				os.Exit(1)
			}
		}
	}
}
