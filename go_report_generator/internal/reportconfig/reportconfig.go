package reportconfig

import (
	"fmt"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/logging"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser/filtering"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/settings"
)

// IReportConfiguration defines the configuration for report generation.
type IReportConfiguration interface {
	ReportFiles() []string
	TargetDirectory() string
	SourceDirectories() []string
	HistoryDirectory() string
	ReportTypes() []string
	Plugins() []string
	AssemblyFilters() filtering.IFilter            
	ClassFilters() filtering.IFilter               
	FileFilters() filtering.IFilter                
	RiskHotspotAssemblyFilters() filtering.IFilter 
	RiskHotspotClassFilters() filtering.IFilter    
	VerbosityLevel() logging.VerbosityLevel
	Tag() string
	Title() string
	License() string
	InvalidReportFilePatterns() []string
	IsVerbosityLevelValid() bool
	Settings() *settings.Settings 
}

// ReportConfiguration is a concrete implementation of IReportConfiguration.
type ReportConfiguration struct {
	RFiles                        []string
	TDirectory                    string
	SDirectories                  []string
	HDirectory                    string
	RTypes                        []string
	PluginsList                   []string
	AssemblyFilterInstance        filtering.IFilter 
	ClassFilterInstance           filtering.IFilter 
	FileFilterInstance            filtering.IFilter 
	RiskHotspotAssemblyFilterInst filtering.IFilter 
	RiskHotspotClassFilterInst    filtering.IFilter 
	VLevel                        logging.VerbosityLevel
	CfgTag                        string
	CfgTitle                      string
	CfgLicense                    string
	InvalidPatterns               []string
	VLevelValid                   bool
	App                           *settings.Settings 
}

// Implement IReportConfiguration methods
func (rc *ReportConfiguration) ReportFiles() []string              { return rc.RFiles }
func (rc *ReportConfiguration) TargetDirectory() string            { return rc.TDirectory }
func (rc *ReportConfiguration) SourceDirectories() []string        { return rc.SDirectories }
func (rc *ReportConfiguration) HistoryDirectory() string           { return rc.HDirectory }
func (rc *ReportConfiguration) ReportTypes() []string              { return rc.RTypes }
func (rc *ReportConfiguration) Plugins() []string                  { return rc.PluginsList }
func (rc *ReportConfiguration) AssemblyFilters() filtering.IFilter { return rc.AssemblyFilterInstance }
func (rc *ReportConfiguration) ClassFilters() filtering.IFilter    { return rc.ClassFilterInstance }
func (rc *ReportConfiguration) FileFilters() filtering.IFilter     { return rc.FileFilterInstance }
func (rc *ReportConfiguration) RiskHotspotAssemblyFilters() filtering.IFilter {
	return rc.RiskHotspotAssemblyFilterInst
}
func (rc *ReportConfiguration) RiskHotspotClassFilters() filtering.IFilter {
	return rc.RiskHotspotClassFilterInst
}
func (rc *ReportConfiguration) VerbosityLevel() logging.VerbosityLevel { return rc.VLevel }
func (rc *ReportConfiguration) Tag() string                            { return rc.CfgTag }
func (rc *ReportConfiguration) Title() string                          { return rc.CfgTitle }
func (rc *ReportConfiguration) License() string                        { return rc.CfgLicense }
func (rc *ReportConfiguration) InvalidReportFilePatterns() []string    { return rc.InvalidPatterns }
func (rc *ReportConfiguration) IsVerbosityLevelValid() bool            { return rc.VLevelValid }
func (rc *ReportConfiguration) Settings() *settings.Settings           { return rc.App } 

// NewReportConfiguration is a constructor for ReportConfiguration.
func NewReportConfiguration(
	reportFiles []string,
	targetDir string,
	sourceDirs []string,
	historyDir string,
	reportTypes []string,
	tag string,
	title string,
	verbosity logging.VerbosityLevel,
	invalidPatterns []string,
	// Raw filter strings for creating IFilter instances
	assemblyFilterStrings []string,
	classFilterStrings []string,
	fileFilterStrings []string,
	riskHotspotAssemblyFilterStrings []string,
	riskHotspotClassFilterStrings []string,
	appSettings *settings.Settings, 
) (*ReportConfiguration, error) { // Return error for filter creation issues
	if len(reportTypes) == 0 {
		reportTypes = []string{"Html"} // Default
	}

	var err error
	var assemblyFilter, classFilter, fileFilter, rhAssemblyFilter, rhClassFilter filtering.IFilter

	assemblyFilter, err = filtering.NewDefaultFilter(assemblyFilterStrings)
	if err != nil {
		return nil, fmt.Errorf("failed to create assembly filter: %w", err)
	}
	classFilter, err = filtering.NewDefaultFilter(classFilterStrings)
	if err != nil {
		return nil, fmt.Errorf("failed to create class filter: %w", err)
	}
	fileFilter, err = filtering.NewDefaultFilter(fileFilterStrings, true) // true for osIndependantPathSeparator
	if err != nil {
		return nil, fmt.Errorf("failed to create file filter: %w", err)
	}
	rhAssemblyFilter, err = filtering.NewDefaultFilter(riskHotspotAssemblyFilterStrings)
	if err != nil {
		return nil, fmt.Errorf("failed to create risk hotspot assembly filter: %w", err)
	}
	rhClassFilter, err = filtering.NewDefaultFilter(riskHotspotClassFilterStrings)
	if err != nil {
		return nil, fmt.Errorf("failed to create risk hotspot class filter: %w", err)
	}

	return &ReportConfiguration{
		RFiles:                        reportFiles,
		TDirectory:                    targetDir,
		SDirectories:                  sourceDirs,
		HDirectory:                    historyDir,
		RTypes:                        reportTypes,
		CfgTag:                        tag,
		CfgTitle:                      title,
		VLevel:                        verbosity,
		VLevelValid:                   true,
		InvalidPatterns:               invalidPatterns,
		AssemblyFilterInstance:        assemblyFilter,
		ClassFilterInstance:           classFilter,
		FileFilterInstance:            fileFilter,
		RiskHotspotAssemblyFilterInst: rhAssemblyFilter,
		RiskHotspotClassFilterInst:    rhClassFilter,
		PluginsList:                   []string{},  // Initialize if not passed
		App:                           appSettings, // Store settings
	}, nil
}
