package parser

import (
	"time"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporting"
)

type ParserResult struct {
	Assemblies             []model.Assembly
	SourceDirectories      []string
	SupportsBranchCoverage bool
	ParserName             string
	MinimumTimeStamp       *time.Time
	MaximumTimeStamp       *time.Time
}

// IParser defines the contract for all coverage report parsers.
type IParser interface {
	Name() string
	SupportsFile(filePath string) bool
	Parse(filePath string, context reporting.IReportContext) (*ParserResult, error)
}
