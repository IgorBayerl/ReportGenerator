package parser

import (
	"time"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser/filtering"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/settings"
)

// ParserResult holds the processed data from a single coverage report file.
type ParserResult struct {
	Assemblies             []model.Assembly
	SourceDirectories      []string
	SupportsBranchCoverage bool
	ParserName             string
	MinimumTimeStamp       *time.Time
	MaximumTimeStamp       *time.Time
}

// ParserConfig defines the lean configuration required by a parser.
// This consumer-defined interface decouples parsers from the main report configuration.
type ParserConfig interface {
	SourceDirectories() []string
	AssemblyFilters() filtering.IFilter
	ClassFilters() filtering.IFilter
	FileFilters() filtering.IFilter
	Settings() *settings.Settings
}

// IParser defines the contract for all coverage report parsers.
type IParser interface {
	Name() string
	SupportsFile(filePath string) bool
	// Parse now accepts the lean ParserConfig interface.
	Parse(filePath string, config ParserConfig) (*ParserResult, error)
}
