package parser

import (
	"time"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser/filtering"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/settings"
)

// ParserResult holds the processed data from a single coverage report.
type ParserResult struct {
	Assemblies             []model.Assembly
	SourceDirectories      []string
	SupportsBranchCoverage bool
	ParserName             string
	MinimumTimeStamp       *time.Time
	MaximumTimeStamp       *time.Time
}

type ParserConfig interface {
	SourceDirectories() []string
	AssemblyFilters() filtering.IFilter
	ClassFilters() filtering.IFilter
	FileFilters() filtering.IFilter
	Settings() *settings.Settings
}

type IParser interface {
	Name() string
	SupportsFile(filePath string) bool
	Parse(filePath string, config ParserConfig) (*ParserResult, error)
}
