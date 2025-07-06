package reporting

import (
	"io"
	"log/slog"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reportconfig"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/settings"
)

// IReportContext defines the context for report generation.
type IReportContext interface {
	ReportConfiguration() reportconfig.IReportConfiguration
	Settings() *settings.Settings
	Logger() *slog.Logger
}

type ReportContext struct {
	Cfg   reportconfig.IReportConfiguration
	Stngs *settings.Settings
	L     *slog.Logger
}

func (rc *ReportContext) ReportConfiguration() reportconfig.IReportConfiguration { return rc.Cfg }
func (rc *ReportContext) Settings() *settings.Settings                           { return rc.Stngs }
func (rc *ReportContext) Logger() *slog.Logger                                   { return rc.L }

// NewReportContext creates a new ReportContext.
// Update the constructor to accept a logger.
func NewReportContext(config reportconfig.IReportConfiguration, settings *settings.Settings, logger *slog.Logger) *ReportContext {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}

	return &ReportContext{
		Cfg:   config,
		Stngs: settings,
		L:     logger,
	}
}
