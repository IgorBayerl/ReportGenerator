package cobertura

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/inputxml"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/utils"
)

// CoberturaParser implements the parser.IParser interface for Cobertura XML reports.
type CoberturaParser struct {
}

// NewCoberturaParser creates a new CoberturaParser.
func NewCoberturaParser() parser.IParser {
	return &CoberturaParser{}
}

func init() {
	parser.RegisterParser(NewCoberturaParser())
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
// It now accepts the lean parser.ParserConfig interface.
func (cp *CoberturaParser) Parse(filePath string, config parser.ParserConfig) (*parser.ParserResult, error) {
	// Load and unmarshal the raw XML data.
	rawReport, sourceDirsFromXML, err := cp.loadAndUnmarshalCoberturaXML(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load/unmarshal Cobertura XML from %s: %w", filePath, err)
	}

	// Determine the effective source directories by combining those from the
	// command line and those specified within the report file itself.
	effectiveSourceDirs := cp.getEffectiveSourceDirs(config, sourceDirsFromXML)

	// Process the raw report data into the common model structure.
	assemblies, err := cp.processPackages(rawReport.Packages.Package, effectiveSourceDirs, config)
	if err != nil {
		return nil, fmt.Errorf("failed to process Cobertura packages: %w", err)
	}

	// Extract the report's timestamp.
	timestamp := cp.getReportTimestamp(rawReport.Timestamp)

	return &parser.ParserResult{
		Assemblies:             assemblies,
		SourceDirectories:      sourceDirsFromXML,
		SupportsBranchCoverage: true,
		ParserName:             cp.Name(),
		MinimumTimeStamp:       timestamp,
		MaximumTimeStamp:       timestamp,
	}, nil
}

// getEffectiveSourceDirs combines source directories from the configuration (CLI)
// and from the XML file's <sources> tag to create a comprehensive list of search paths.
func (cp *CoberturaParser) getEffectiveSourceDirs(config parser.ParserConfig, sourceDirsFromXML []string) []string {
	sourceDirsSet := make(map[string]struct{})

	// Add directories from command-line arguments
	for _, dir := range config.SourceDirectories() {
		if dir != "" {
			sourceDirsSet[dir] = struct{}{}
		}
	}

	// Add directories from the XML report file
	for _, dir := range sourceDirsFromXML {
		if dir != "" {
			sourceDirsSet[dir] = struct{}{}
		}
	}

	// Convert the set back to a slice
	var effectiveSourceDirs []string
	for dir := range sourceDirsSet {
		effectiveSourceDirs = append(effectiveSourceDirs, dir)
	}

	return effectiveSourceDirs
}

// processPackages iterates through all <package> elements from the Cobertura report
// and processes them into a slice of model.Assembly.
func (cp *CoberturaParser) processPackages(packages []inputxml.PackageXML, sourceDirs []string, config parser.ParserConfig) ([]model.Assembly, error) {
	var parsedAssemblies []model.Assembly
	uniqueFilePathsForGrandTotalLines := make(map[string]int)

	for _, pkgXML := range packages {
		assembly, err := cp.processCoberturaPackageXML(pkgXML, sourceDirs, uniqueFilePathsForGrandTotalLines, config)
		if err != nil {
			// In a real application, a logger passed via the config/context would be used.
			// For now, we print a warning and continue, allowing partial results.
			fmt.Fprintf(os.Stderr, "Warning: CoberturaParser: could not process package XML for '%s': %v. Skipping.\n", pkgXML.Name, err)
			continue
		}
		if assembly != nil { // A nil assembly means it was filtered out
			parsedAssemblies = append(parsedAssemblies, *assembly)
		}
	}

	return parsedAssemblies, nil
}

// getReportTimestamp parses the Cobertura timestamp string into a *time.Time object.
func (cp *CoberturaParser) getReportTimestamp(rawTimestamp string) *time.Time {
	if rawTimestamp == "" {
		return nil
	}
	// Cobertura timestamps are typically Unix timestamps (seconds since epoch),
	// but some generators produce milliseconds. We check for this.
	parsedTs, err := strconv.ParseInt(rawTimestamp, 10, 64)
	if err != nil {
		return nil
	}

	// If the timestamp seems to be in milliseconds, convert to seconds.
	if !utils.IsValidUnixSeconds(parsedTs) && utils.IsValidUnixSeconds(parsedTs/1000) {
		parsedTs /= 1000
	}

	if utils.IsValidUnixSeconds(parsedTs) {
		t := time.Unix(parsedTs, 0)
		return &t
	}

	return nil
}

func (cp *CoberturaParser) loadAndUnmarshalCoberturaXML(path string) (*inputxml.CoberturaRoot, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	bytes, err := io.ReadAll(f)
	if err != nil {
		return nil, nil, err
	}
	var rawReport inputxml.CoberturaRoot
	if err := xml.Unmarshal(bytes, &rawReport); err != nil {
		return nil, nil, err
	}
	return &rawReport, rawReport.Sources.Source, nil
}

func (cp *CoberturaParser) parseInt(s string) int {
	val, _ := strconv.Atoi(s)
	return val
}
func (cp *CoberturaParser) parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
func (cp *CoberturaParser) processCoberturaTimestamp(rawTimestamp string) int64 {
	if rawTimestamp == "" {
		return 0
	}
	parsedTs, err := strconv.ParseInt(rawTimestamp, 10, 64)
	if err != nil {
		return 0
	}
	return parsedTs
}
