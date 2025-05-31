package reportconfig

import "github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/logging" // Assuming your logging package

// IReportConfiguration defines the configuration for report generation.
// Corresponds to C#'s ReportGenerator.Core.Reporting.IReportConfiguration
type IReportConfiguration interface {
	ReportFiles() []string
	TargetDirectory() string
	SourceDirectories() []string
	HistoryDirectory() string
	ReportTypes() []string
	Plugins() []string // May not be used in Go version initially
	AssemblyFilters() []string
	ClassFilters() []string
	FileFilters() []string
	RiskHotspotAssemblyFilters() []string
	RiskHotspotClassFilters() []string
	VerbosityLevel() logging.VerbosityLevel
	Tag() string
	Title() string
	License() string // May not be used in Go version initially
	InvalidReportFilePatterns() []string
	IsVerbosityLevelValid() bool
}

// ReportConfiguration is a concrete implementation of IReportConfiguration.
// Corresponds to C#'s ReportGenerator.Core.ReportConfiguration
type ReportConfiguration struct {
	RFiles                        []string
	TDirectory                    string
	SDirectories                  []string
	HDirectory                    string
	RTypes                        []string
	PluginsList                   []string
	AssemblyFilterList            []string
	ClassFilterList               []string
	FileFilterList                []string
	RiskHotspotAssemblyFilterList []string
	RiskHotspotClassFilterList    []string
	VLevel                        logging.VerbosityLevel
	CfgTag                        string
	CfgTitle                      string
	CfgLicense                    string
	InvalidPatterns               []string
	VLevelValid                   bool
}

// Implement IReportConfiguration methods
func (rc *ReportConfiguration) ReportFiles() []string       { return rc.RFiles }
func (rc *ReportConfiguration) TargetDirectory() string     { return rc.TDirectory }
func (rc *ReportConfiguration) SourceDirectories() []string { return rc.SDirectories }
func (rc *ReportConfiguration) HistoryDirectory() string    { return rc.HDirectory }
func (rc *ReportConfiguration) ReportTypes() []string       { return rc.RTypes }
func (rc *ReportConfiguration) Plugins() []string           { return rc.PluginsList }
func (rc *ReportConfiguration) AssemblyFilters() []string   { return rc.AssemblyFilterList }
func (rc *ReportConfiguration) ClassFilters() []string      { return rc.ClassFilterList }
func (rc *ReportConfiguration) FileFilters() []string       { return rc.FileFilterList }
func (rc *ReportConfiguration) RiskHotspotAssemblyFilters() []string {
	return rc.RiskHotspotAssemblyFilterList
}
func (rc *ReportConfiguration) RiskHotspotClassFilters() []string {
	return rc.RiskHotspotClassFilterList
}
func (rc *ReportConfiguration) VerbosityLevel() logging.VerbosityLevel { return rc.VLevel }
func (rc *ReportConfiguration) Tag() string                            { return rc.CfgTag }
func (rc *ReportConfiguration) Title() string                          { return rc.CfgTitle }
func (rc *ReportConfiguration) License() string                        { return rc.CfgLicense }
func (rc *ReportConfiguration) InvalidReportFilePatterns() []string    { return rc.InvalidPatterns }
func (rc *ReportConfiguration) IsVerbosityLevelValid() bool            { return rc.VLevelValid }

// NewReportConfiguration is a constructor for ReportConfiguration.
// You'll need to adapt how this is created from your command-line flags (cmd/main.go)
func NewReportConfiguration(
	reportFiles []string,
	targetDir string,
	sourceDirs []string,
	historyDir string,
	reportTypes []string,
	tag string,
	title string,
	verbosity logging.VerbosityLevel,
	// Add other fields like filters as needed
) *ReportConfiguration {
	if len(reportTypes) == 0 {
		reportTypes = []string{"Html"} // Default
	}
	return &ReportConfiguration{
		RFiles:       reportFiles,
		TDirectory:   targetDir,
		SDirectories: sourceDirs,
		HDirectory:   historyDir,
		RTypes:       reportTypes,
		CfgTag:       tag,
		CfgTitle:     title,
		VLevel:       verbosity,
		VLevelValid:  true, // Assume valid for now, add validation if needed
		// Initialize filter lists as empty slices:
		AssemblyFilterList:            []string{},
		ClassFilterList:               []string{},
		FileFilterList:                []string{},
		RiskHotspotAssemblyFilterList: []string{},
		RiskHotspotClassFilterList:    []string{},
		PluginsList:                   []string{},
		InvalidPatterns:               []string{},
	}
}
