package htmlreport

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporting"
)

type HtmlReportBuilder struct {
	OutputDir     string
	ReportContext reporting.IReportContext

	// Cached data for reuse across page generations
	angularCssFile                     string
	angularRuntimeJsFile               string
	angularPolyfillsJsFile             string
	angularMainJsFile                  string
	assembliesJSON                     template.JS
	riskHotspotsJSON                   template.JS
	metricsJSON                        template.JS
	riskHotspotMetricsJSON             template.JS
	historicCoverageExecutionTimesJSON template.JS
	translationsJSON                   template.JS

	// Settings derived from context
	branchCoverageAvailable                  bool
	methodCoverageAvailable                  bool
	maximumDecimalPlacesForCoverageQuotas    int
	maximumDecimalPlacesForPercentageDisplay int
	parserName                               string
	reportTimestamp                          int64
	reportTitle                              string
	tag                                      string
	translations                             map[string]string
	onlySummary                              bool // In C#, this is based on report types. For now, assume false.

}

func NewHtmlReportBuilder(outputDir string, reportCtx reporting.IReportContext) *HtmlReportBuilder {
	return &HtmlReportBuilder{
		OutputDir:     outputDir,
		ReportContext: reportCtx,
	}
}

func (b *HtmlReportBuilder) ReportType() string {
	return "Html"
}

func (b *HtmlReportBuilder) CreateReport(report *model.SummaryResult) error {
	if err := b.validateContext(); err != nil {
		return err
	}
	if err := b.prepareOutputDirectory(); err != nil {
		return err
	}
	if err := b.initializeAssets(); err != nil {
		return err
	}

	b.initializeBuilderProperties(report)
	if err := b.prepareGlobalJSONData(report); err != nil {
		return err
	}

	summaryPageAssemblyFilenames := make(map[string]struct{})
	angularAssemblies, err := b.buildAngularAssemblyViewModelsForSummary(report, summaryPageAssemblyFilenames)
	if err != nil {
		return fmt.Errorf("failed to build angular assembly view models for summary: %w", err)
	}

	// For now, risk hotspots are empty for the summary page JSON.
	// This would be populated if risk hotspot analysis was performed.
	var angularRiskHotspots []AngularRiskHotspotViewModel
	if err := b.setRiskHotspotsJSON(angularRiskHotspots); err != nil {
		return err
	}

	summaryData, err := b.buildSummaryPageData(report, angularAssemblies, angularRiskHotspots)
	if err != nil {
		return fmt.Errorf("failed to build summary page data: %w", err)
	}
	if err := b.renderSummaryPage(summaryData); err != nil {
		return fmt.Errorf("failed to render summary page: %w", err)
	}

	if !b.onlySummary {
		if err := b.renderClassDetailPages(report, angularAssemblies); err != nil {
			return fmt.Errorf("failed to render class detail pages: %w", err)
		}
	}
	return nil

}

// --- CreateReport helper methods ---

func (b *HtmlReportBuilder) validateContext() error {
	if b.ReportContext == nil {
		return fmt.Errorf("HtmlReportBuilder.ReportContext is not set; it's required for configuration and settings")
	}
	return nil
}

func (b *HtmlReportBuilder) prepareOutputDirectory() error {
	return os.MkdirAll(b.OutputDir, 0755)
}

func (b *HtmlReportBuilder) initializeBuilderProperties(report *model.SummaryResult) {
	reportConfig := b.ReportContext.ReportConfiguration()
	settings := b.ReportContext.Settings()

	b.reportTitle = reportConfig.Title()
	if b.reportTitle == "" {
		b.reportTitle = "Summary" // Default for summary page
	}
	b.parserName = report.ParserName
	b.reportTimestamp = report.Timestamp
	b.tag = reportConfig.Tag()
	b.branchCoverageAvailable = report.BranchesValid != nil && *report.BranchesValid > 0
	b.methodCoverageAvailable = true // Per original C# behavior
	b.maximumDecimalPlacesForCoverageQuotas = settings.MaximumDecimalPlacesForCoverageQuotas
	b.maximumDecimalPlacesForPercentageDisplay = settings.MaximumDecimalPlacesForPercentageDisplay
	b.translations = GetTranslations()
	// b.onlySummary determination could be more complex based on ReportTypes in reportConfig

}

func (b *HtmlReportBuilder) renderSummaryPage(data SummaryPageData) error {
	outputIndexPath := filepath.Join(b.OutputDir, "index.html")
	summaryFile, err := os.Create(outputIndexPath)
	if err != nil {
		return fmt.Errorf("failed to create index.html: %w", err)
	}
	defer summaryFile.Close()
	return summaryPageTpl.Execute(summaryFile, data)
}

func (b *HtmlReportBuilder) renderClassDetailPages(report *model.SummaryResult, angularAssembliesForSummary []AngularAssemblyViewModel) error {
	classPageFilenames := make(map[string]struct{}) // Use a new map for class page filenames

	for _, assemblyModel := range report.Assemblies {
		for _, classModel := range assemblyModel.Classes {
			classReportFilename := b.determineClassReportFilename(&assemblyModel, &classModel, angularAssembliesForSummary, classPageFilenames)
			if classReportFilename == "" {
				// This case should ideally not happen if logic is correct
				fmt.Fprintf(os.Stderr, "Warning: Could not determine report filename for class %s. Skipping detail page.\n", classModel.DisplayName)
				continue
			}

			err := b.generateClassDetailHTML(&classModel, classReportFilename, b.tag)
			if err != nil {
				// Log or collect errors, but continue if possible, or return immediately
				fmt.Fprintf(os.Stderr, "Failed to generate detail page for class '%s' (file: %s): %v\n", classModel.DisplayName, classReportFilename, err)
				// Depending on desired behavior, you might return err here to stop all generation
			}
		}
	}
	return nil

}

func (b *HtmlReportBuilder) determineClassReportFilename(
	assemblyModel *model.Assembly,
	classModel *model.Class,
	angularAssembliesForSummary []AngularAssemblyViewModel,
	classPageFilenames map[string]struct{}, // Map for filenames generated for class detail pages
) string {
	var classReportFilename string
	foundFilenameInSummary := false

	// 1. Check if this class's report path was already defined in the summary Angular VMs
	for _, asmView := range angularAssembliesForSummary {
		if asmView.Name == assemblyModel.Name {
			for _, classView := range asmView.Classes {
				if classView.Name == classModel.DisplayName {
					classReportFilename = classView.ReportPath
					if classReportFilename != "" {
						// "Reserve" this filename in the current context's map
						classPageFilenames[strings.ToLower(classReportFilename)] = struct{}{}
						foundFilenameInSummary = true
					}
					break
				}
			}
		}
		if foundFilenameInSummary {
			break
		}
	}

	// 2. If not found in summary VMs or path was empty, generate a new unique filename.
	if !foundFilenameInSummary || classReportFilename == "" {
		// Call the utility function from the htmlreport package (or wherever utils.go is placed)
		// It uses the package-level sanitizeFilenameChars from its own file.
		classReportFilename = generateUniqueFilename(assemblyModel.Name, classModel.Name, classPageFilenames)
	}

	return classReportFilename
}

func (b *HtmlReportBuilder) renderClassDetailPage(data ClassDetailData, classReportFilename string) error {
	outputFilePath := filepath.Join(b.OutputDir, classReportFilename)
	fileWriter, err := os.Create(outputFilePath)
	if err != nil {
		return fmt.Errorf("failed to create class report file %s: %w", outputFilePath, err)
	}
	defer fileWriter.Close()
	return classDetailTpl.Execute(fileWriter, data)
}
