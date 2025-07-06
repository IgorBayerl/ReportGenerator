package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/analyzer"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/glob"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/logging"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser"             // Import the main parser package
	_ "github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser/cobertura" // Import for side-effect: registers CoberturaParser
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reportconfig"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporter/htmlreport"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporter/textsummary"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporting"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/settings"
)

// cliFlags holds the parsed command-line flags.
type cliFlags struct {
	reportsPatterns *string
	outputDir       *string
	reportTypes     *string
	sourceDirs      *string
	tag             *string
	title           *string
	verbosity       *string
	logFile         *string // Add logFile flag holder
}

var supportedReportTypes = map[string]bool{
	"TextSummary": true,
	"Html":        true,
}

// setupCliAndLogger parses command-line flags and initializes the structured logger.
func setupCliAndLogger() (*cliFlags, *slog.Logger, logging.VerbosityLevel, io.Closer, error) {
	flags := &cliFlags{
		reportsPatterns: flag.String("report", "", "Coverage report file paths or patterns (semicolon-separated)"),
		outputDir:       flag.String("output", "coverage-report", "Output directory for reports"),
		reportTypes:     flag.String("reporttypes", "TextSummary,Html", "Report types to generate (comma-separated: TextSummary,Html)"),
		sourceDirs:      flag.String("sourcedirs", "", "Source directories (comma-separated)"),
		tag:             flag.String("tag", "", "Optional tag (e.g., build number)"),
		title:           flag.String("title", "", "Optional report title. Default: 'Coverage Report'"),
		verbosity:       flag.String("verbosity", "Info", "Logging verbosity level (Verbose, Info, Warning, Error, Off)"),
		logFile:         flag.String("logfile", "", "Redirect logs to a file instead of the console."), // Define the new flag
	}
	flag.Parse()

	var verbosityLevel logging.VerbosityLevel
	var slogLevel slog.Leveler = slog.LevelInfo // Default

	switch strings.ToLower(*flags.verbosity) {
	case "verbose":
		verbosityLevel = logging.Verbose
		slogLevel = slog.LevelDebug
	case "info":
		verbosityLevel = logging.Info
		slogLevel = slog.LevelInfo
	case "warning":
		verbosityLevel = logging.Warning
		slogLevel = slog.LevelWarn
	case "error":
		verbosityLevel = logging.Error
		slogLevel = slog.LevelError
	case "off":
		verbosityLevel = logging.Off
		slogLevel = slog.Level(slog.LevelError + 42) // A level that will never be used
	default:
		return nil, nil, 0, nil, fmt.Errorf("invalid verbosity level '%s'. Valid levels are Verbose, Info, Warning, Error, Off", *flags.verbosity)
	}

	// Determine the output writer for the logger
	var logOutput io.Writer = os.Stderr
	var logFileCloser io.Closer // To hold the file handle if we open one

	if *flags.logFile != "" {
		f, err := os.OpenFile(*flags.logFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			return nil, nil, 0, nil, fmt.Errorf("failed to open log file %s: %w", *flags.logFile, err)
		}
		logOutput = f
		logFileCloser = f // We need to close this file later
	}

	handlerOptions := &slog.HandlerOptions{Level: slogLevel}
	// Use the determined logOutput (either os.Stderr or the file)
	logger := slog.New(slog.NewTextHandler(logOutput, handlerOptions))

	// Set as global default for convenience in other packages if they don't get a logger passed explicitly.
	slog.SetDefault(logger)

	return flags, logger, verbosityLevel, logFileCloser, nil
}

// resolveAndValidateInputs processes input patterns and validates them.
func resolveAndValidateInputs(logger *slog.Logger, flags *cliFlags) ([]string, []string, error) {
	if *flags.reportsPatterns == "" {
		return nil, nil, fmt.Errorf("missing required -report flag. Usage: go_report_generator -reports <file/pattern>...")
	}

	reportFilePatterns := strings.Split(*flags.reportsPatterns, ";")
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
			logger.Warn("Error expanding report file pattern", "pattern", trimmedPattern, "error", err)
			invalidPatterns = append(invalidPatterns, trimmedPattern)
			continue
		}
		if len(expandedFiles) == 0 {
			logger.Warn("No files found for report pattern", "pattern", trimmedPattern)
			invalidPatterns = append(invalidPatterns, trimmedPattern)
		}
		for _, file := range expandedFiles {
			absFile, _ := filepath.Abs(file)
			if _, exists := seenFiles[absFile]; !exists {
				if stat, err := os.Stat(absFile); err == nil && !stat.IsDir() {
					actualReportFiles = append(actualReportFiles, absFile)
					seenFiles[absFile] = struct{}{}
				} else if err != nil {
					logger.Warn("Could not stat file from pattern", "pattern", trimmedPattern, "file", absFile, "error", err)
					invalidPatterns = append(invalidPatterns, file)
				}
			}
		}
	}

	if len(actualReportFiles) == 0 {
		return nil, invalidPatterns, fmt.Errorf("no valid report files found after expanding patterns")
	}

	logger.Info("Found report files", "count", len(actualReportFiles))
	logger.Debug("Report file list", "files", strings.Join(actualReportFiles, ", "))
	return actualReportFiles, invalidPatterns, nil
}

// createReportConfiguration assembles the main configuration object for the generator.
func createReportConfiguration(flags *cliFlags, verbosity logging.VerbosityLevel, actualReportFiles, invalidPatterns []string) (*reportconfig.ReportConfiguration, error) {
	requestedTypes := strings.Split(*flags.reportTypes, ",")
	for _, t := range requestedTypes {
		trimmedType := strings.TrimSpace(t)
		if !supportedReportTypes[trimmedType] {
			return nil, fmt.Errorf("unsupported report type: %s", trimmedType)
		}
	}

	var sourceDirsList []string
	if *flags.sourceDirs != "" {
		sourceDirsList = strings.Split(*flags.sourceDirs, ",")
		for i, dir := range sourceDirsList {
			sourceDirsList[i] = strings.TrimSpace(dir)
		}
	}

	actualTitle := *flags.title
	if actualTitle == "" {
		actualTitle = "Coverage Report"
	}

	return reportconfig.NewReportConfiguration(
		actualReportFiles,
		*flags.outputDir,
		sourceDirsList,
		"", // historyDir
		requestedTypes,
		*flags.tag,
		actualTitle,
		verbosity,
		invalidPatterns,
		[]string{}, []string{}, []string{}, []string{}, []string{}, // empty filters for now
		settings.NewSettings(),
	)
}

// parseAndMergeReports handles the parsing of all found report files and merges them into a single result.
func parseAndMergeReports(logger *slog.Logger, reportCtx reporting.IReportContext) (*model.SummaryResult, error) {
	var parserResults []*parser.ParserResult
	var parserErrors []string

	reportConfig := reportCtx.ReportConfiguration()
	actualReportFiles := reportConfig.ReportFiles()

	for _, reportFile := range actualReportFiles {
		logger.Info("Attempting to parse report file", "file", reportFile)
		parserInstance, err := parser.FindParserForFile(reportFile)
		if err != nil {
			msg := fmt.Sprintf("no suitable parser found for file %s: %v", reportFile, err)
			parserErrors = append(parserErrors, msg)
			logger.Warn(msg)
			continue
		}

		logger.Info("Using parser for file", "parser", parserInstance.Name(), "file", reportFile)
		result, err := parserInstance.Parse(reportFile, reportCtx)
		if err != nil {
			msg := fmt.Sprintf("error parsing file %s with %s: %v", reportFile, parserInstance.Name(), err)
			parserErrors = append(parserErrors, msg)
			logger.Error(msg)
			continue
		}
		parserResults = append(parserResults, result)
		logger.Info("Successfully parsed file", "file", reportFile)

		// This logic dynamically updates the context if a report file specifies source directories.
		// It's a bit of a side effect but kept for feature parity.
		if len(reportConfig.SourceDirectories()) == 0 && len(result.SourceDirectories) > 0 {
			logger.Info("Report specified source directories, updating configuration for context", "file", reportFile, "dirs", result.SourceDirectories)
			updatedConfig, confErr := reportconfig.NewReportConfiguration(
				reportConfig.ReportFiles(), reportConfig.TargetDirectory(), result.SourceDirectories, reportConfig.HistoryDirectory(),
				reportConfig.ReportTypes(), reportConfig.Tag(), reportConfig.Title(), reportConfig.VerbosityLevel(), reportConfig.InvalidReportFilePatterns(),
				[]string{}, []string{}, []string{}, []string{}, []string{}, reportCtx.Settings(),
			)
			if confErr != nil {
				return nil, fmt.Errorf("error updating report configuration with new source dirs: %w", confErr)
			}
			reportCtx = reporting.NewReportContext(updatedConfig, reportCtx.Settings(), logger)
			reportConfig = updatedConfig // Update local variable for next loop iteration
		}
	}

	if len(parserResults) == 0 {
		errorMsg := "no coverage reports could be parsed successfully"
		if len(parserErrors) > 0 {
			errorMsg = fmt.Sprintf("%s. Errors:\n- %s", errorMsg, strings.Join(parserErrors, "\n- "))
		}
		return nil, fmt.Errorf(errorMsg)
	}

	logger.Info("Merging parsed reports", "count", len(parserResults))
	summaryResult, err := analyzer.MergeParserResults(parserResults, reportConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to merge parser results: %w", err)
	}
	logger.Info("Coverage data merged and analyzed")

	return summaryResult, nil
}

// generateReports creates the final report files based on the summary result.
func generateReports(logger *slog.Logger, reportCtx reporting.IReportContext, summaryResult *model.SummaryResult) error {
	reportConfig := reportCtx.ReportConfiguration()
	outputDir := reportConfig.TargetDirectory()

	logger.Info("Generating reports", "directory", outputDir)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	for _, reportType := range reportConfig.ReportTypes() {
		trimmedType := strings.TrimSpace(reportType)
		logger.Info("Generating report", "type", trimmedType)

		switch trimmedType {
		case "TextSummary":
			textBuilder := textsummary.NewTextReportBuilder(outputDir, logger)
			if err := textBuilder.CreateReport(summaryResult); err != nil {
				return fmt.Errorf("failed to generate text report: %w", err)
			}
		case "Html":
			htmlBuilder := htmlreport.NewHtmlReportBuilder(outputDir, reportCtx)
			if err := htmlBuilder.CreateReport(summaryResult); err != nil {
				return fmt.Errorf("failed to generate HTML report: %w", err)
			}
		}
	}
	return nil
}

// run is the main application logic.
func run() error {
	flags, logger, verbosity, logFileCloser, err := setupCliAndLogger()
	if err != nil {
		return err
	}
	// If a log file was opened, ensure it gets closed when this function exits.
	if logFileCloser != nil {
		defer logFileCloser.Close()
	}

	actualReportFiles, invalidPatterns, err := resolveAndValidateInputs(logger, flags)
	if err != nil {
		if len(invalidPatterns) > 0 {
			return fmt.Errorf("%w. Invalid patterns: %s", err, strings.Join(invalidPatterns, ", "))
		}
		return err
	}

	reportConfig, err := createReportConfiguration(flags, verbosity, actualReportFiles, invalidPatterns)
	if err != nil {
		return err
	}

	reportCtx := reporting.NewReportContext(reportConfig, settings.NewSettings(), logger)

	summaryResult, err := parseAndMergeReports(logger, reportCtx)
	if err != nil {
		return err
	}

	return generateReports(logger, reportCtx, summaryResult)
}

func main() {
	start := time.Now()

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// We can't use the logger from `run` here as it's out of scope.
	// But since `slog.SetDefault` was called, we can use the global logger
	// for the final success message.
	slog.Info("Report generation completed successfully", "duration", time.Since(start).Round(time.Millisecond))
}
