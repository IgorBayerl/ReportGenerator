package reporting

import (
	// Adjust import paths based on your actual project structure
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reportconfig"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/settings"
	// You might need other imports like model for HistoricCoverage if you add that field
)

// IReportContext defines the context for report generation.
// Corresponds to C#'s ReportGenerator.Core.Reporting.IReportContext
type IReportContext interface {
	ReportConfiguration() reportconfig.IReportConfiguration
	Settings() *settings.Settings
	// TODO: Add RiskHotspotAnalysisResult and OverallHistoricCoverages if/when implemented
	// RiskHotspotAnalysisResult() *RiskHotspotAnalysisResult (define this struct)
	// OverallHistoricCoverages() []model.HistoricCoverage
}

// ReportContext is a concrete implementation of IReportContext.
// Corresponds to C#'s ReportGenerator.Core.ReportContext
type ReportContext struct {
	Cfg   reportconfig.IReportConfiguration
	Stngs *settings.Settings
	// RiskHotspots *RiskHotspotAnalysisResult
	// Historic     []model.HistoricCoverage
}

// Implement IReportContext methods
func (rc *ReportContext) ReportConfiguration() reportconfig.IReportConfiguration { return rc.Cfg }
func (rc *ReportContext) Settings() *settings.Settings                           { return rc.Stngs }

// func (rc *ReportContext) RiskHotspotAnalysisResult() *RiskHotspotAnalysisResult { return rc.RiskHotspots }
// func (rc *ReportContext) OverallHistoricCoverages() []model.HistoricCoverage    { return rc.Historic }

// NewReportContext creates a new ReportContext.
func NewReportContext(config reportconfig.IReportConfiguration, settings *settings.Settings) *ReportContext {
	return &ReportContext{
		Cfg:   config,
		Stngs: settings,
	}
}
