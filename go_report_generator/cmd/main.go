package main

import (
	"errors"
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
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser"
	_ "github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser/cobertura"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reportconfig"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporter/htmlreport"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporter/textsummary"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporting"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/settings"
)

type cliFlags struct {
	reportsPatterns *string
	outputDir       *string
	reportTypes     *string
	sourceDirs      *string
	tag             *string
	title           *string
	verbosity       *string
	logFile         *string
	assemblyFilters   *string
	classFilters      *string
	fileFilters       *string
	rhAssemblyFilters *string
	rhClassFilters    *string
}

// setupCliAndLogger parses command-line flags and initializes the structured logger.
func setupCliAndLogger() (*cliFlags, logging.VerbosityLevel, io.Closer, error) {
	flags := &cliFlags{
		reportsPatterns:   flag.String("report", "", "Coverage report file paths or patterns (semicolon-separated)"),
		outputDir:         flag.String("output", "coverage-report", "Output directory for reports"),
		reportTypes:       flag.String("reporttypes", "TextSummary,Html", "Report types to generate (comma-separated)"),
		sourceDirs:        flag.String("sourcedirs", "", "Source directories (comma-separated)"),
		tag:               flag.String("tag", "", "Optional tag (e.g., build number)"),
		title:             flag.String("title", "", "Optional report title. Default: 'Coverage Report'"),
		verbosity:         flag.String("verbosity", "Info", "Logging verbosity level (Verbose, Info, Warning, Error, Off)"),
		logFile:           flag.String("logfile", "", "Redirect logs to a file instead of the console."),
		assemblyFilters:   flag.String("assemblyfilters", "", "Assembly filters (e.g., +MyProject;-MyProject.Tests)"),
		classFilters:      flag.String("classfilters", "", "Class filters"),
		fileFilters:       flag.String("filefilters", "", "File filters"),
		rhAssemblyFilters: flag.String("riskhotspotassemblyfilters", "", "Risk hotspot assembly filters"),
		rhClassFilters:    flag.String("riskhotspotclassfilters", "", "Risk hotspot class filters"),
	}
	flag.Parse()

	var verbosityLevel logging.VerbosityLevel
	var slogLevel slog.Leveler

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
		slogLevel = slog.Level(slog.LevelError + 42) // A level that will effectively silence the logger.
	default:
		return nil, 0, nil, fmt.Errorf("invalid verbosity level '%s'", *flags.verbosity)
	}

	var logOutput io.Writer = os.Stderr
	var logFileCloser io.Closer

	if *flags.logFile != "" {
		f, err := os.OpenFile(*flags.logFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			return nil, 0, nil, fmt.Errorf("failed to open log file %s: %w", *flags.logFile, err)
		}
		logOutput = f
		logFileCloser = f
	}

	handlerOptions := &slog.HandlerOptions{Level: slogLevel}
	logger := slog.New(slog.NewTextHandler(logOutput, handlerOptions))
	slog.SetDefault(logger)

	return flags, verbosityLevel, logFileCloser, nil
}

// resolveAndValidateInputs processes input patterns and validates them.
func resolveAndValidateInputs(logger *slog.Logger, flags *cliFlags) ([]string, []string, error) {
	if *flags.reportsPatterns == "" {
		return nil, nil, fmt.Errorf("missing required -report flag")
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

// createReportConfiguration assembles the main configuration object for the generator using the Functional Options Pattern.
func createReportConfiguration(flags *cliFlags, verbosity logging.VerbosityLevel, actualReportFiles, invalidPatterns []string) (*reportconfig.ReportConfiguration, error) {
	// Collect all raw string inputs from flags
	reportTypes := strings.Split(*flags.reportTypes, ",")
	sourceDirsList := strings.Split(*flags.sourceDirs, ",")
	assemblyFilterStrings := strings.Split(*flags.assemblyFilters, ";")
	classFilterStrings := strings.Split(*flags.classFilters, ";")
	fileFilterStrings := strings.Split(*flags.fileFilters, ";")
	rhAssemblyFilterStrings := strings.Split(*flags.rhAssemblyFilters, ";")
	rhClassFilterStrings := strings.Split(*flags.rhClassFilters, ";")

	// Build the list of options to apply
	opts := []reportconfig.Option{
		reportconfig.WithVerbosity(verbosity),
		reportconfig.WithInvalidPatterns(invalidPatterns),
		reportconfig.WithTitle(*flags.title),
		reportconfig.WithTag(*flags.tag),
		reportconfig.WithSourceDirectories(sourceDirsList),
		reportconfig.WithReportTypes(reportTypes),
		reportconfig.WithFilters(
			assemblyFilterStrings,
			classFilterStrings,
			fileFilterStrings,
			rhAssemblyFilterStrings,
			rhClassFilterStrings,
		),
	}

	// Create the configuration
	return reportconfig.NewReportConfiguration(
		actualReportFiles,
		*flags.outputDir,
		opts...,
	)
}

// parseAndMergeReports handles the parsing of all found report files and merges them into a single result.
func parseAndMergeReports(logger *slog.Logger, reportConfig *reportconfig.ReportConfiguration) (*model.SummaryResult, error) {
	var parserResults []*parser.ParserResult
	var parserErrors []string

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
		result, err := parserInstance.Parse(reportFile, reportConfig)
		if err != nil {
			msg := fmt.Sprintf("error parsing file %s with %s: %v", reportFile, parserInstance.Name(), err)
			parserErrors = append(parserErrors, msg)
			logger.Error(msg)
			continue
		}
		parserResults = append(parserResults, result)
		logger.Info("Successfully parsed file", "file", reportFile)

		// This dynamic update logic can be simplified if we decide to
		// collect all source dirs first. But for now, it's kept.
		if len(reportConfig.SourceDirectories()) == 0 && len(result.SourceDirectories) > 0 {
			logger.Info("Report specified source directories, updating configuration", "file", reportFile, "dirs", result.SourceDirectories)

			// Recreate the config options with the new source directories
			opts := []reportconfig.Option{
				reportconfig.WithSourceDirectories(result.SourceDirectories),
				// Re-apply other options as well
				reportconfig.WithVerbosity(reportConfig.VerbosityLevel()),
				reportconfig.WithInvalidPatterns(reportConfig.InvalidReportFilePatterns()),
				reportconfig.WithTitle(reportConfig.Title()),
				reportconfig.WithTag(reportConfig.Tag()),
				reportconfig.WithReportTypes(reportConfig.ReportTypes()),
				// Note: getting raw filter strings back from the config is tricky.
				// This side-effect logic makes things complicated. A better approach
				// would be to gather all source dirs from all reports *before* parsing.
				// For now, we just update the source dirs.
			}

			// In-place update (not ideal, but works with current structure)
			for _, opt := range opts {
				if err := opt(reportConfig); err != nil {
					logger.Warn("Failed to apply report configuration option", "error", err)
				}
			}
		}
	}

	if len(parserResults) == 0 {
		errorMsg := "no coverage reports could be parsed successfully"
		if len(parserErrors) > 0 {
			errorMsg = fmt.Sprintf("%s. Errors:\n- %s", errorMsg, strings.Join(parserErrors, "\n- "))
		}
		return nil, errors.New(errorMsg)
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
func generateReports(reportCtx reporting.IReportContext, summaryResult *model.SummaryResult) error {
	logger := reportCtx.Logger()
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
	flags, verbosity, logFileCloser, err := setupCliAndLogger()
	if err != nil {
		return err
	}
	if logFileCloser != nil {
		defer logFileCloser.Close()
	}

	logger := slog.Default()

	actualReportFiles, invalidPatterns, err := resolveAndValidateInputs(logger, flags)
	if err != nil {
		if len(invalidPatterns) > 0 {
			return fmt.Errorf("%w; invalid patterns: %s", err, strings.Join(invalidPatterns, ", "))
		}
		return err
	}

	reportConfig, err := createReportConfiguration(flags, verbosity, actualReportFiles, invalidPatterns)
	if err != nil {
		return err
	}

	summaryResult, err := parseAndMergeReports(logger, reportConfig)
	if err != nil {
		return err
	}

	reportCtx := reporting.NewReportContext(reportConfig, settings.NewSettings(), logger)

	return generateReports(reportCtx, summaryResult)
}

func main() {
	start := time.Now()

	if err := run(); err != nil {
		slog.Error("An error occurred during report generation", "error", err)
		os.Exit(1)
	}

	slog.Info("Report generation completed successfully", "duration", time.Since(start).Round(time.Millisecond))
}
