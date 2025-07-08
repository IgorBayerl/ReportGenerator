package cobertura

import (
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/filereader"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/inputxml"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/utils"
)

// CoberturaParser implements the parser.IParser interface for Cobertura XML reports.
// It now includes a FileReader dependency to enable testability.
type CoberturaParser struct {
	fileReader FileReader // Injected dependency
}

// DefaultFileReader is the production implementation of the FileReader interface,
// using the real filesystem.
type DefaultFileReader struct{}

// ReadFile implements the FileReader interface for production.
func (dfr *DefaultFileReader) ReadFile(path string) ([]string, error) {
	return filereader.ReadLinesInFile(path)
}

// CountLines implements the FileReader interface for production.
func (dfr *DefaultFileReader) CountLines(path string) (int, error) {
	return filereader.CountLinesInFile(path)
}

// NewCoberturaParser creates a new CoberturaParser with the given FileReader.
// This constructor is designed for dependency injection.
func NewCoberturaParser(fileReader FileReader) parser.IParser {
	return &CoberturaParser{
		fileReader: fileReader,
	}
}

// init registers the CoberturaParser with the central parser factory.
// It uses the DefaultFileReader for production use.
func init() {
	parser.RegisterParser(NewCoberturaParser(&DefaultFileReader{}))
}

// Name returns the name of the parser.
func (cp *CoberturaParser) Name() string {
	return "Cobertura"
}

// SupportsFile checks if the given file is likely a Cobertura XML report.
func (cp *CoberturaParser) SupportsFile(filePath string) bool {
	if !strings.HasSuffix(strings.ToLower(filePath), ".xml") {
		return false
	}
	f, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer f.Close()
	decoder := xml.NewDecoder(f)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false
		}
		if se, ok := token.(xml.StartElement); ok {
			return se.Name.Local == "coverage"
		}
	}
	return false
}

// Parse processes the Cobertura XML file and transforms it into a common ParserResult.
func (cp *CoberturaParser) Parse(filePath string, config parser.ParserConfig) (*parser.ParserResult, error) {
	rawReport, sourceDirsFromXML, err := cp.loadAndUnmarshalCoberturaXML(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load/unmarshal Cobertura XML from %s: %w", filePath, err)
	}

	effectiveSourceDirs := cp.getEffectiveSourceDirs(config, sourceDirsFromXML)

	// Create the orchestrator, passing true for supportsBranchCoverage
	orchestrator := newProcessingOrchestrator(cp.fileReader, config, effectiveSourceDirs, true)

	assemblies, err := orchestrator.processPackages(rawReport.Packages.Package)
	if err != nil {
		return nil, fmt.Errorf("failed to process Cobertura packages: %w", err)
	}

	timestamp := cp.getReportTimestamp(rawReport.Timestamp)

	return &parser.ParserResult{
		Assemblies:             assemblies,
		SourceDirectories:      sourceDirsFromXML,
		SupportsBranchCoverage: true, // This parser always supports it
		ParserName:             cp.Name(),
		MinimumTimeStamp:       timestamp,
		MaximumTimeStamp:       timestamp,
	}, nil
}

// getEffectiveSourceDirs combines source directories from the configuration (CLI)
// and from the XML file's <sources> tag to create a comprehensive list of search paths.
func (cp *CoberturaParser) getEffectiveSourceDirs(config parser.ParserConfig, sourceDirsFromXML []string) []string {
	sourceDirsSet := make(map[string]struct{})

	for _, dir := range config.SourceDirectories() {
		if dir != "" {
			sourceDirsSet[dir] = struct{}{}
		}
	}

	for _, dir := range sourceDirsFromXML {
		if dir != "" {
			sourceDirsSet[dir] = struct{}{}
		}
	}

	var effectiveSourceDirs []string
	for dir := range sourceDirsSet {
		effectiveSourceDirs = append(effectiveSourceDirs, dir)
	}

	return effectiveSourceDirs
}

// getReportTimestamp parses the Cobertura timestamp string into a *time.Time object.
func (cp *CoberturaParser) getReportTimestamp(rawTimestamp string) *time.Time {
	if rawTimestamp == "" {
		return nil
	}
	parsedTs, err := strconv.ParseInt(rawTimestamp, 10, 64)
	if err != nil {
		slog.Warn("Failed to parse Cobertura timestamp", "timestamp", rawTimestamp, "error", err)
		return nil
	}

	// Handle timestamps in milliseconds vs. seconds
	if !utils.IsValidUnixSeconds(parsedTs) && utils.IsValidUnixSeconds(parsedTs/1000) {
		parsedTs /= 1000
	}

	if utils.IsValidUnixSeconds(parsedTs) {
		t := time.Unix(parsedTs, 0)
		return &t
	}

	slog.Warn("Cobertura timestamp is outside the valid range", "timestamp", rawTimestamp)
	return nil
}

func (cp *CoberturaParser) loadAndUnmarshalCoberturaXML(path string) (*inputxml.CoberturaRoot, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	bytes, err := io.ReadAll(f)
	if err != nil {
		return nil, nil, fmt.Errorf("read file: %w", err)
	}

	var rawReport inputxml.CoberturaRoot
	if err := xml.Unmarshal(bytes, &rawReport); err != nil {
		return nil, nil, fmt.Errorf("unmarshal xml: %w", err)
	}
	return &rawReport, rawReport.Sources.Source, nil
}
