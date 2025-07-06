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
	rawReport, sourceDirsFromXML, err := cp.loadAndUnmarshalCoberturaXML(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load/unmarshal Cobertura XML from %s: %w", filePath, err)
	}

	effectiveSourceDirs := config.SourceDirectories()
	if len(effectiveSourceDirs) == 0 && len(sourceDirsFromXML) > 0 {
		effectiveSourceDirs = sourceDirsFromXML
	}

	var parsedAssemblies []model.Assembly
	uniqueFilePathsForGrandTotalLines := make(map[string]int)

	for _, pkgXML := range rawReport.Packages.Package {
		// Pass the lean 'config' directly to processing functions.
		assembly, err := cp.processCoberturaPackageXML(pkgXML, effectiveSourceDirs, uniqueFilePathsForGrandTotalLines, config)
		if err != nil {
			// In a real app with logging, this would use the logger.
			// For now, printing to stderr is a placeholder.
			fmt.Fprintf(os.Stderr, "Warning: CoberturaParser: could not process package XML for '%s': %v. Skipping.\n", pkgXML.Name, err)
			continue
		}
		if assembly != nil { // Ensure assembly is not nil if filter excluded it
			parsedAssemblies = append(parsedAssemblies, *assembly)
		}
	}

	var timestamp *time.Time
	if rawReport.Timestamp != "" {
		tsVal := cp.processCoberturaTimestamp(rawReport.Timestamp)
		if tsVal > 0 {
			t := time.Unix(tsVal, 0)
			timestamp = &t
		}
	}

	return &parser.ParserResult{
		Assemblies:             parsedAssemblies,
		SourceDirectories:      sourceDirsFromXML,
		SupportsBranchCoverage: true,
		ParserName:             cp.Name(),
		MinimumTimeStamp:       timestamp,
		MaximumTimeStamp:       timestamp,
	}, nil
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
