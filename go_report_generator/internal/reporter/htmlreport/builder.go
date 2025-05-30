package htmlreport

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/filereader"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporting"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/utils"
	"golang.org/x/net/html"
)

var (
	assetsDir             = filepath.Join(utils.ProjectRoot(), "assets", "htmlreport")
	angularDistSourcePath = filepath.Join(utils.ProjectRoot(), "angular_frontend_spa", "dist")
	sanitizeFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
)

const (
	lineVisitStatusNotCoverable     = 0
	lineVisitStatusCovered          = 1
	lineVisitStatusNotCovered       = 2
	lineVisitStatusPartiallyCovered = 3
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
	branchCoverageAvailable               bool
	methodCoverageAvailable               bool
	maximumDecimalPlacesForCoverageQuotas int
	parserName                            string
	reportTimestamp                       int64
	reportTitle                           string
	tag                                   string
	translations                          map[string]string
	onlySummary                           bool // In C#, this is based on report types. For now, assume false.

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

func (b *HtmlReportBuilder) initializeAssets() error {
	if err := b.copyStaticAssets(); err != nil {
		return fmt.Errorf("failed to copy static assets: %w", err)
	}
	if err := b.copyAngularAssets(b.OutputDir); err != nil {
		return fmt.Errorf("failed to copy angular assets: %w", err)
	}

	angularIndexHTMLPath := filepath.Join(angularDistSourcePath, "index.html")
	cssFile, runtimeJs, polyfillsJs, mainJs, err := b.parseAngularIndexHTML(angularIndexHTMLPath)
	if err != nil {
		return fmt.Errorf("failed to parse Angular index.html: %w", err)
	}
	if cssFile == "" || runtimeJs == "" || mainJs == "" {
		fmt.Fprintf(os.Stderr, "Warning: One or more Angular assets might be missing (css: %s, runtime: %s, polyfills: %s, main: %s)\n", cssFile, runtimeJs, polyfillsJs, mainJs)
		if cssFile == "" || runtimeJs == "" || mainJs == "" { // Check critical ones
			return fmt.Errorf("missing critical Angular assets from index.html (css: '%s', runtime: '%s', main: '%s')", cssFile, runtimeJs, mainJs)
		}
	}
	b.angularCssFile = cssFile
	b.angularRuntimeJsFile = runtimeJs
	b.angularPolyfillsJsFile = polyfillsJs
	b.angularMainJsFile = mainJs
	return nil

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
	b.translations = GetTranslations()
	// b.onlySummary determination could be more complex based on ReportTypes in reportConfig

}

func (b *HtmlReportBuilder) prepareGlobalJSONData(report *model.SummaryResult) error {
	translationsJSONBytes, err := json.Marshal(b.translations)
	if err != nil {
		b.translationsJSON = template.JS("({})") // Fallback
	} else {
		b.translationsJSON = template.JS(translationsJSONBytes)
	}

	availableMetrics := []AngularMetricViewModel{
		{Name: "NPath complexity", Abbreviation: "npath", ExplanationURL: "https://modess.io/npath-complexity-cyclomatic-complexity-explained/"},
		{Name: "CrapScore", Abbreviation: "crap", ExplanationURL: "https://testing.googleblog.com/2011/02/this-code-is-crap.html"},
	}
	metricsJSONBytes, err := json.Marshal(availableMetrics)
	if err != nil {
		b.metricsJSON = template.JS("([])")
	} else {
		b.metricsJSON = template.JS(metricsJSONBytes)
	}

	riskHotspotMetricHeaders := []AngularRiskHotspotMetricHeaderViewModel{
		{Name: "Cyclomatic complexity", Abbreviation: "cyclomatic", ExplanationURL: "https://www.ndepend.com/docs/code-metrics#CC"},
		{Name: "CrapScore", Abbreviation: "crap", ExplanationURL: "https://testing.googleblog.com/2011/02/this-code-is-crap.html"},
		{Name: "NPath complexity", Abbreviation: "npath", ExplanationURL: "https://modess.io/npath-complexity-cyclomatic-complexity-explained/"},
	}
	riskHotspotMetricsJSONBytes, err := json.Marshal(riskHotspotMetricHeaders)
	if err != nil {
		b.riskHotspotMetricsJSON = template.JS("([])")
	} else {
		b.riskHotspotMetricsJSON = template.JS(riskHotspotMetricsJSONBytes)
	}

	executionTimes := b.collectHistoricExecutionTimes(report)
	historicExecTimesJSONBytes, err := json.Marshal(executionTimes)
	if err != nil {
		b.historicCoverageExecutionTimesJSON = template.JS("([])")
	} else {
		b.historicCoverageExecutionTimesJSON = template.JS(historicExecTimesJSONBytes)
	}
	return nil

}

func (b *HtmlReportBuilder) collectHistoricExecutionTimes(report *model.SummaryResult) []string {
	uniqueExecutionTimestamps := make(map[int64]bool)
	if report.Assemblies != nil {
		for _, assembly := range report.Assemblies {
			for _, class := range assembly.Classes {
				for _, hist := range class.HistoricCoverages {
					if _, exists := uniqueExecutionTimestamps[hist.ExecutionTime]; !exists {
						uniqueExecutionTimestamps[hist.ExecutionTime] = true
					}
				}
			}
		}
	}
	var executionTimes []string
	for ts := range uniqueExecutionTimestamps {
		executionTimes = append(executionTimes, time.Unix(ts, 0).Format("2006-01-02 15:04:05"))
	}
	sort.Strings(executionTimes) // Ensure consistent order
	return executionTimes
}

func (b *HtmlReportBuilder) buildAngularAssemblyViewModelsForSummary(report *model.SummaryResult, summaryPageAssemblyFilenames map[string]struct{}) ([]AngularAssemblyViewModel, error) {
	var angularAssemblies []AngularAssemblyViewModel
	if report.Assemblies == nil {
		b.assembliesJSON = template.JS("([])")
		return angularAssemblies, nil
	}

	for _, assembly := range report.Assemblies {
		assemblyShortName := assembly.Name
		angularAssembly := AngularAssemblyViewModel{Name: assembly.Name, Classes: []AngularClassViewModel{}}
		for _, class := range assembly.Classes {
			classReportFilename := b.getClassReportFilename(assemblyShortName, class.Name, summaryPageAssemblyFilenames)
			angularClass := b.buildAngularClassViewModelForSummary(&class, classReportFilename)
			angularAssembly.Classes = append(angularAssembly.Classes, angularClass)
		}
		angularAssemblies = append(angularAssemblies, angularAssembly)
	}

	assembliesJSONBytes, err := json.Marshal(angularAssemblies)
	if err != nil {
		b.assembliesJSON = template.JS("([])") // Fallback
		return nil, fmt.Errorf("failed to marshal angular assemblies: %w", err)
	}
	b.assembliesJSON = template.JS(assembliesJSONBytes)
	return angularAssemblies, nil

}

func (b *HtmlReportBuilder) buildAngularClassViewModelForSummary(class *model.Class, reportPath string) AngularClassViewModel {
	angularClass := AngularClassViewModel{
		Name:                class.DisplayName,
		ReportPath:          reportPath,
		CoveredLines:        class.LinesCovered,
		CoverableLines:      class.LinesValid,
		UncoveredLines:      class.LinesValid - class.LinesCovered,
		TotalLines:          class.TotalLines,
		Metrics:             make(map[string]float64),
		HistoricCoverages:   []AngularHistoricCoverageViewModel{},
		LineCoverageHistory: []float64{}, BranchCoverageHistory: []float64{},
	}

	angularClass.TotalMethods = class.TotalMethods
	angularClass.CoveredMethods = class.CoveredMethods
	angularClass.FullyCoveredMethods = class.FullyCoveredMethods

	if class.BranchesCovered != nil {
		angularClass.CoveredBranches = *class.BranchesCovered
	}
	if class.BranchesValid != nil {
		angularClass.TotalBranches = *class.BranchesValid
	}

	for _, hist := range class.HistoricCoverages {
		angularHist := b.buildAngularHistoricCoverageViewModel(&hist)
		angularClass.HistoricCoverages = append(angularClass.HistoricCoverages, angularHist)
		if angularHist.LineCoverageQuota >= 0 {
			angularClass.LineCoverageHistory = append(angularClass.LineCoverageHistory, angularHist.LineCoverageQuota)
		}
		if angularHist.BranchCoverageQuota >= 0 {
			angularClass.BranchCoverageHistory = append(angularClass.BranchCoverageHistory, angularHist.BranchCoverageQuota)
		}
	}

	// Populate class.Metrics (aggregated from methods)
	// This logic assumes metrics are already aggregated on model.Class.Metrics
	for name, val := range class.Metrics {
		angularClass.Metrics[name] = val
	}
	return angularClass

}

func (b *HtmlReportBuilder) buildAngularHistoricCoverageViewModel(hist *model.HistoricCoverage) AngularHistoricCoverageViewModel {
	angularHist := AngularHistoricCoverageViewModel{
		ExecutionTime:   time.Unix(hist.ExecutionTime, 0).Format("2006-01-02"), // Simplified format for summary
		CoveredLines:    hist.CoveredLines,
		CoverableLines:  hist.CoverableLines,
		TotalLines:      hist.TotalLines,
		CoveredBranches: hist.CoveredBranches,
		TotalBranches:   hist.TotalBranches,
	}
	if hist.CoverableLines > 0 {
		angularHist.LineCoverageQuota = (float64(hist.CoveredLines) / float64(hist.CoverableLines)) * 100.0
	}
	if hist.TotalBranches > 0 {
		angularHist.BranchCoverageQuota = (float64(hist.CoveredBranches) / float64(hist.TotalBranches)) * 100.0
	}
	// Method coverage history can be added if model.HistoricCoverage includes method counts
	return angularHist
}

func (b *HtmlReportBuilder) setRiskHotspotsJSON(angularRiskHotspots []AngularRiskHotspotViewModel) error {
	riskHotspotsJSONBytes, err := json.Marshal(angularRiskHotspots)
	if err != nil {
		b.riskHotspotsJSON = template.JS("([])") // Fallback
		return fmt.Errorf("failed to marshal angular risk hotspots: %w", err)
	}
	b.riskHotspotsJSON = template.JS(riskHotspotsJSONBytes)
	return nil
}

func (b *HtmlReportBuilder) buildSummaryPageData(report *model.SummaryResult, _ []AngularAssemblyViewModel, angularRiskHotspots []AngularRiskHotspotViewModel) (SummaryPageData, error) {
	data := SummaryPageData{
		ReportTitle:                           b.reportTitle,
		AppVersion:                            "0.0.1", // Placeholder
		CurrentDateTime:                       time.Now().Format("02/01/2006 - 15:04:05"),
		Translations:                          b.translations,
		HasRiskHotspots:                       len(angularRiskHotspots) > 0,
		HasAssemblies:                         len(report.Assemblies) > 0,
		AssembliesJSON:                        b.assembliesJSON,
		RiskHotspotsJSON:                      b.riskHotspotsJSON,
		MetricsJSON:                           b.metricsJSON,
		RiskHotspotMetricsJSON:                b.riskHotspotMetricsJSON,
		HistoricCoverageExecutionTimesJSON:    b.historicCoverageExecutionTimesJSON,
		TranslationsJSON:                      b.translationsJSON,
		AngularCssFile:                        b.angularCssFile,
		AngularRuntimeJsFile:                  b.angularRuntimeJsFile,
		AngularPolyfillsJsFile:                b.angularPolyfillsJsFile,
		AngularMainJsFile:                     b.angularMainJsFile,
		BranchCoverageAvailable:               b.branchCoverageAvailable,
		MethodCoverageAvailable:               b.methodCoverageAvailable,
		MaximumDecimalPlacesForCoverageQuotas: b.maximumDecimalPlacesForCoverageQuotas,
		SummaryCards:                          b.buildSummaryCards(report),
		OverallHistoryChartData:               HistoryChartDataViewModel{Series: false}, // Simplified
	}
	return data, nil
}

func (b *HtmlReportBuilder) buildSummaryCards(report *model.SummaryResult) []CardViewModel {
	var cards []CardViewModel

	// Information Card
	infoCardRows := []CardRowViewModel{
		{Header: b.translations["Parser"], Text: report.ParserName},
		{Header: b.translations["Assemblies2"], Text: fmt.Sprintf("%d", len(report.Assemblies)), Alignment: "right"},
		{Header: b.translations["Classes"], Text: fmt.Sprintf("%d", countTotalClasses(report.Assemblies)), Alignment: "right"},
		{Header: b.translations["Files2"], Text: fmt.Sprintf("%d", countUniqueFiles(report.Assemblies)), Alignment: "right"},
	}
	if report.Timestamp > 0 {
		infoCardRows = append(infoCardRows, CardRowViewModel{Header: b.translations["CoverageDate"], Text: time.Unix(report.Timestamp, 0).Format("02/01/2006 - 15:04:05")})
	}
	if b.tag != "" {
		infoCardRows = append(infoCardRows, CardRowViewModel{Header: b.translations["Tag"], Text: b.tag})
	}
	cards = append(cards, CardViewModel{Title: b.translations["Information"], Rows: infoCardRows})

	// Line Coverage Card
	lineCovQuota, lineCovText, lineCovTooltip, lineCovBar := 0.0, "N/A", "-", 0
	if report.LinesValid > 0 {
		lineCovQuota = (float64(report.LinesCovered) / float64(report.LinesValid)) * 100.0
		lineCovText = fmt.Sprintf("%.1f%%", lineCovQuota)
		lineCovTooltip = fmt.Sprintf("%d of %d", report.LinesCovered, report.LinesValid)
		lineCovBar = 100 - int(math.Round(lineCovQuota))
	}
	cards = append(cards, CardViewModel{Title: b.translations["LineCoverage"], SubTitle: lineCovText, SubTitlePercentageBarValue: lineCovBar, Rows: []CardRowViewModel{
		{Header: b.translations["CoveredLines"], Text: fmt.Sprintf("%d", report.LinesCovered), Alignment: "right"},
		{Header: b.translations["UncoveredLines"], Text: fmt.Sprintf("%d", report.LinesValid-report.LinesCovered), Alignment: "right"},
		{Header: b.translations["CoverableLines"], Text: fmt.Sprintf("%d", report.LinesValid), Alignment: "right"},
		{Header: b.translations["TotalLines"], Text: fmt.Sprintf("%d", report.TotalLines), Alignment: "right"},
		{Header: b.translations["LineCoverage"], Text: lineCovText, Tooltip: lineCovTooltip, Alignment: "right"},
	}})

	// Branch Coverage Card (Conditional)
	if b.branchCoverageAvailable && report.BranchesCovered != nil && report.BranchesValid != nil {
		branchCovQuota, branchCovText, branchCovTooltip, branchCovBar := 0.0, "N/A", "-", 0
		if *report.BranchesValid > 0 {
			branchCovQuota = (float64(*report.BranchesCovered) / float64(*report.BranchesValid)) * 100.0
			branchCovText = fmt.Sprintf("%.1f%%", branchCovQuota)
			branchCovTooltip = fmt.Sprintf("%d of %d", *report.BranchesCovered, *report.BranchesValid)
			branchCovBar = 100 - int(math.Round(branchCovQuota))
		}
		cards = append(cards, CardViewModel{Title: b.translations["BranchCoverage"], SubTitle: branchCovText, SubTitlePercentageBarValue: branchCovBar, Rows: []CardRowViewModel{
			{Header: b.translations["CoveredBranches2"], Text: fmt.Sprintf("%d", *report.BranchesCovered), Alignment: "right"},
			{Header: b.translations["TotalBranches"], Text: fmt.Sprintf("%d", *report.BranchesValid), Alignment: "right"},
			{Header: b.translations["BranchCoverage"], Text: branchCovText, Tooltip: branchCovTooltip, Alignment: "right"},
		}})
	}

	// Method Coverage Card
	var totalMethods, coveredMethods, fullyCoveredMethods int
	for _, asm := range report.Assemblies {
		for _, cls := range asm.Classes {
			totalMethods += cls.TotalMethods
			coveredMethods += cls.CoveredMethods
			fullyCoveredMethods += cls.FullyCoveredMethods
		}
	}
	methodCovText, methodCovBar, methodCovTooltip := "N/A", 0, "-"
	fullMethodCovText, fullMethodCovTooltip := "N/A", "-"
	if totalMethods > 0 {
		methodRate := (float64(coveredMethods) / float64(totalMethods)) * 100.0
		methodCovText = fmt.Sprintf("%.1f%%", methodRate)
		methodCovBar = 100 - int(math.Round(methodRate))
		methodCovTooltip = fmt.Sprintf("%d of %d", coveredMethods, totalMethods)

		fullMethodRate := (float64(fullyCoveredMethods) / float64(totalMethods)) * 100.0
		fullMethodCovText = fmt.Sprintf("%.1f%%", fullMethodRate)
		fullMethodCovTooltip = fmt.Sprintf("%d of %d", fullyCoveredMethods, totalMethods)
	}
	cards = append(cards, CardViewModel{
		Title: b.translations["MethodCoverage"], ProRequired: !b.methodCoverageAvailable, SubTitle: methodCovText, SubTitlePercentageBarValue: methodCovBar,
		Rows: []CardRowViewModel{
			{Header: b.translations["CoveredCodeElements"], Text: fmt.Sprintf("%d", coveredMethods), Alignment: "right"},
			{Header: b.translations["FullCoveredCodeElements"], Text: fmt.Sprintf("%d", fullyCoveredMethods), Alignment: "right"},
			{Header: b.translations["TotalCodeElements"], Text: fmt.Sprintf("%d", totalMethods), Alignment: "right"},
			{Header: b.translations["CodeElementCoverageQuota2"], Text: methodCovText, Tooltip: methodCovTooltip, Alignment: "right"},
			{Header: b.translations["FullCodeElementCoverageQuota2"], Text: fullMethodCovText, Tooltip: fullMethodCovTooltip, Alignment: "right"},
		},
	})
	return cards

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
	classPageFilenames map[string]struct{}) string {

	var classReportFilename string
	foundFilename := false
	// Try to get from summary-generated view models first for consistency
	for _, asmView := range angularAssembliesForSummary {
		if asmView.Name == assemblyModel.Name {
			for _, classView := range asmView.Classes {
				if classView.Name == classModel.DisplayName {
					classReportFilename = classView.ReportPath
					// Add to classPageFilenames to "reserve" it, assuming it's unique from summary context
					classPageFilenames[strings.ToLower(classReportFilename)] = struct{}{}
					foundFilename = true
					break
				}
			}
		}
		if foundFilename {
			break
		}
	}

	if !foundFilename || classReportFilename == "" {
		// Fallback: generate filename directly for this context if not found or empty
		classReportFilename = b.getClassReportFilename(assemblyModel.Name, classModel.Name, classPageFilenames)
	}
	return classReportFilename

}

// --- generateClassDetailHTML and its helper methods ---

func (b *HtmlReportBuilder) generateClassDetailHTML(classModel *model.Class, classReportFilename string, tag string) error {
	// 1. Build the main ClassViewModelForDetail (server-side rendering focus)
	classVM := b.buildClassViewModelForDetailServer(classModel, tag)

	// 2. Build the AngularClassDetailViewModel (for client-side window.classDetails JSON)
	angularClassDetailForJS, err := b.buildAngularClassDetailForJS(classModel, &classVM)
	if err != nil {
		return fmt.Errorf("failed to build Angular class detail JSON for %s: %w", classModel.DisplayName, err)
	}
	classDetailJSONBytes, err := json.Marshal(angularClassDetailForJS)
	if err != nil {
		return fmt.Errorf("failed to marshal Angular class detail JSON for %s: %w", classModel.DisplayName, err)
	}

	// 3. Prepare overall data for the template
	templateData := b.buildClassDetailPageData(classVM, tag, template.JS(classDetailJSONBytes))

	// 4. Render the template
	return b.renderClassDetailPage(templateData, classReportFilename)

}

func (b *HtmlReportBuilder) buildClassViewModelForDetailServer(classModel *model.Class, tag string) ClassViewModelForDetail {
	cvm := ClassViewModelForDetail{
		Name:         classModel.DisplayName,
		AssemblyName: classModel.Name, // Keep full name for Assembly, DisplayName is for Class title
		IsMultiFile:  len(classModel.Files) > 1,
	}
	if dotIndex := strings.LastIndex(classModel.Name, "."); dotIndex > -1 && dotIndex < len(classModel.Name)-1 {
		// This might be a simplification. C# RG likely gets assembly name from model.Assembly.
		// For now, assuming classModel.Name is like "Namespace.Possibly.AssemblyName.ClassName"
		// and we want "Namespace.Possibly.AssemblyName"
		cvm.AssemblyName = classModel.Name[:dotIndex] // This might be incorrect if class name itself has dots.
		// Let's assume the AssemblyName is derived from the parent model.Assembly.Name when processing.
		// For now, this placeholder uses the full class name which is not ideal.
		// This should be: if classModel has an AssemblyName field, use that.
		// Or, pass assemblyName string to this function.
		// For now, will use classModel.Name as placeholder as in original logic for DisplayName
	}

	cvm.CoveredLines = classModel.LinesCovered
	cvm.CoverableLines = classModel.LinesValid
	cvm.UncoveredLines = cvm.CoverableLines - cvm.CoveredLines
	cvm.TotalLines = classModel.TotalLines

	b.populateLineCoverageMetricsForClassVM(&cvm, classModel)
	b.populateBranchCoverageMetricsForClassVM(&cvm, classModel)
	b.populateMethodCoverageMetricsForClassVM(&cvm, classModel)
	b.populateHistoricCoveragesForClassVM(&cvm, classModel)
	b.populateAggregatedMetricsForClassVM(&cvm, classModel) // Class-level aggregated metrics

	var allMethodMetricsForClass []*model.MethodMetric
	for fileIdx, fileInClass := range classModel.Files {
		fileVM, sourceLines, err := b.buildFileViewModelForServerRender(&fileInClass)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not build file view model for %s: %v\n", fileInClass.Path, err)
			continue // Skip this file
		}

		for i := range fileInClass.MethodMetrics { // Collect all for the class
			allMethodMetricsForClass = append(allMethodMetricsForClass, &fileInClass.MethodMetrics[i])
		}
		for _, codeElem := range fileInClass.CodeElements {
			sidebarElem := b.buildSidebarElementViewModel(&codeElem, fileVM.ShortPath, fileIdx+1, len(classModel.Files) > 1)
			cvm.SidebarElements = append(cvm.SidebarElements, sidebarElem)
		}
		// The original code had a bug where it re-read sourceFileLines for each file,
		// this function now uses the sourceLines returned by buildFileViewModelForServerRender
		cvm.Files = append(cvm.Files, fileVM)
		_ = sourceLines // To use if lines were not part of FileViewModel, but they are
	}

	if len(allMethodMetricsForClass) > 0 {
		cvm.FilesWithMetrics = true
		cvm.MetricsTable = b.buildMetricsTableForClassVM(classModel, allMethodMetricsForClass)
	}

	return cvm

}

func (b *HtmlReportBuilder) populateLineCoverageMetricsForClassVM(cvm *ClassViewModelForDetail, classModel *model.Class) {
	if cvm.CoverableLines > 0 {
		lineCoverage := (float64(cvm.CoveredLines) / float64(cvm.CoverableLines)) * 100.0
		cvm.CoveragePercentageForDisplay = fmt.Sprintf("%.1f%%", lineCoverage)
		cvm.CoveragePercentageBarValue = 100 - int(math.Round(lineCoverage))
		cvm.CoverageRatioTextForDisplay = fmt.Sprintf("%d of %d", cvm.CoveredLines, cvm.CoverableLines)
	} else {
		cvm.CoveragePercentageForDisplay = "N/A"
		cvm.CoveragePercentageBarValue = 0 // Avoid NaN issues with Round
		cvm.CoverageRatioTextForDisplay = "-"
	}
}

func (b *HtmlReportBuilder) populateBranchCoverageMetricsForClassVM(cvm *ClassViewModelForDetail, classModel *model.Class) {
	if b.branchCoverageAvailable && classModel.BranchesValid != nil && *classModel.BranchesValid > 0 && classModel.BranchesCovered != nil {
		cvm.CoveredBranches = *classModel.BranchesCovered
		cvm.TotalBranches = *classModel.BranchesValid
		branchCoverage := (float64(cvm.CoveredBranches) / float64(cvm.TotalBranches)) * 100.0
		cvm.BranchCoveragePercentageForDisplay = fmt.Sprintf("%.1f%%", branchCoverage)
		cvm.BranchCoveragePercentageBarValue = 100 - int(math.Round(branchCoverage))
		cvm.BranchCoverageRatioTextForDisplay = fmt.Sprintf("%d of %d", cvm.CoveredBranches, cvm.TotalBranches)
	} else {
		cvm.BranchCoveragePercentageForDisplay = "N/A"
		cvm.BranchCoveragePercentageBarValue = 0
		cvm.BranchCoverageRatioTextForDisplay = "-"
	}
}

func (b *HtmlReportBuilder) populateMethodCoverageMetricsForClassVM(cvm *ClassViewModelForDetail, classModel *model.Class) {
	cvm.TotalMethods = classModel.TotalMethods               // Use pre-calculated from model.Class
	cvm.CoveredMethods = classModel.CoveredMethods           // Use pre-calculated
	cvm.FullyCoveredMethods = classModel.FullyCoveredMethods // Use pre-calculated

	if cvm.TotalMethods > 0 {
		methodCov := (float64(cvm.CoveredMethods) / float64(cvm.TotalMethods)) * 100.0
		fullMethodCovVal := (float64(cvm.FullyCoveredMethods) / float64(cvm.TotalMethods)) * 100.0
		cvm.MethodCoveragePercentageForDisplay = fmt.Sprintf("%.1f%%", methodCov)
		cvm.MethodCoveragePercentageBarValue = 100 - int(math.Round(methodCov))
		cvm.MethodCoverageRatioTextForDisplay = fmt.Sprintf("%d of %d", cvm.CoveredMethods, cvm.TotalMethods)
		cvm.FullMethodCoveragePercentageForDisplay = fmt.Sprintf("%.1f%%", fullMethodCovVal)
		cvm.FullMethodCoverageRatioTextForDisplay = fmt.Sprintf("%d of %d", cvm.FullyCoveredMethods, cvm.TotalMethods)
	} else {
		cvm.MethodCoveragePercentageForDisplay = "N/A"
		cvm.MethodCoveragePercentageBarValue = 0
		cvm.MethodCoverageRatioTextForDisplay = "-"
		cvm.FullMethodCoveragePercentageForDisplay = "N/A"
		cvm.FullMethodCoverageRatioTextForDisplay = "-"
	}

}

func (b *HtmlReportBuilder) populateHistoricCoveragesForClassVM(cvm *ClassViewModelForDetail, classModel *model.Class) {
	if classModel.HistoricCoverages == nil {
		return
	}
	for _, hist := range classModel.HistoricCoverages {
		angularHist := b.buildAngularHistoricCoverageViewModel(&hist) // Re-use for consistency
		cvm.HistoricCoverages = append(cvm.HistoricCoverages, angularHist)
		if angularHist.LineCoverageQuota >= 0 {
			cvm.LineCoverageHistory = append(cvm.LineCoverageHistory, angularHist.LineCoverageQuota)
		}
		if angularHist.BranchCoverageQuota >= 0 {
			cvm.BranchCoverageHistory = append(cvm.BranchCoverageHistory, angularHist.BranchCoverageQuota)
		}
	}
}

func (b *HtmlReportBuilder) populateAggregatedMetricsForClassVM(cvm *ClassViewModelForDetail, classModel *model.Class) {
	// Assumes model.Class.Metrics is already populated with aggregated metrics
	cvm.Metrics = make(map[string]float64)
	for name, val := range classModel.Metrics {
		cvm.Metrics[name] = val
	}
}

func (b *HtmlReportBuilder) buildFileViewModelForServerRender(fileInClass *model.CodeFile) (FileViewModelForDetail, []string, error) {
	fileVM := FileViewModelForDetail{
		Path:      fileInClass.Path,
		ShortPath: sanitizeFilenameChars.ReplaceAllString(filepath.Base(fileInClass.Path), "_"),
	}
	sourceLines, err := filereader.ReadLinesInFile(fileInClass.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read source file %s: %v\n", fileInClass.Path, err)
		// Return an empty FileViewModel or handle error appropriately
		// For now, we'll proceed with empty source lines if read fails, lines will be empty.
		sourceLines = []string{}
	}

	coverageLinesMap := make(map[int]*model.Line)
	for i := range fileInClass.Lines { // Make sure to use index to get pointer
		covLine := &fileInClass.Lines[i]
		coverageLinesMap[covLine.Number] = covLine
	}

	for lineNumIdx, lineContent := range sourceLines {
		actualLineNumber := lineNumIdx + 1
		modelCovLine, hasCoverageData := coverageLinesMap[actualLineNumber]
		lineVM := b.buildLineViewModelForServerRender(lineContent, actualLineNumber, modelCovLine, hasCoverageData)
		fileVM.Lines = append(fileVM.Lines, lineVM)
	}
	return fileVM, sourceLines, nil // Return sourceLines in case they are needed by caller

}

func (b *HtmlReportBuilder) buildLineViewModelForServerRender(lineContent string, actualLineNumber int, modelCovLine *model.Line, hasCoverageData bool) LineViewModelForDetail {
	lineVM := LineViewModelForDetail{LineNumber: actualLineNumber, LineContent: lineContent}
	dataCoverageMap := map[string]map[string]string{"AllTestMethods": {"VC": "", "LVS": "gray"}} // Default

	if hasCoverageData {
		lineVM.Hits = fmt.Sprintf("%d", modelCovLine.Hits)
		status := determineLineVisitStatus(modelCovLine.Hits, modelCovLine.IsBranchPoint, modelCovLine.CoveredBranches, modelCovLine.TotalBranches)
		lineVM.LineVisitStatus = lineVisitStatusToString(status)
		if modelCovLine.IsBranchPoint && modelCovLine.TotalBranches > 0 {
			lineVM.IsBranch = true
			branchCoverageVal := (float64(modelCovLine.CoveredBranches) / float64(modelCovLine.TotalBranches)) * 100.0
			lineVM.BranchBarValue = 100 - int(math.Round(branchCoverageVal))
		}
		dataCoverageMap["AllTestMethods"]["VC"] = fmt.Sprintf("%d", modelCovLine.Hits)
		dataCoverageMap["AllTestMethods"]["LVS"] = lineVM.LineVisitStatus
		tooltipBranchRate := ""
		if lineVM.IsBranch {
			tooltipBranchRate = fmt.Sprintf(", %d of %d branches are covered", modelCovLine.CoveredBranches, modelCovLine.TotalBranches)
		}
		switch status {
		case lineVisitStatusCovered:
			lineVM.Tooltip = fmt.Sprintf("Covered (%d visits%s)", modelCovLine.Hits, tooltipBranchRate)
		case lineVisitStatusNotCovered:
			lineVM.Tooltip = fmt.Sprintf("Not covered (%d visits%s)", modelCovLine.Hits, tooltipBranchRate)
		case lineVisitStatusPartiallyCovered:
			lineVM.Tooltip = fmt.Sprintf("Partially covered (%d visits%s)", modelCovLine.Hits, tooltipBranchRate)
		default: // lineVisitStatusNotCoverable
			lineVM.Tooltip = "Not coverable"
		}
	} else {
		lineVM.LineVisitStatus = lineVisitStatusToString(lineVisitStatusNotCoverable)
		lineVM.Hits = ""
		lineVM.Tooltip = "Not coverable"
	}
	dataCoverageBytes, _ := json.Marshal(dataCoverageMap) // Error handling for marshal can be added
	lineVM.DataCoverage = template.JS(dataCoverageBytes)
	return lineVM

}

func (b *HtmlReportBuilder) buildSidebarElementViewModel(codeElem *model.CodeElement, fileShortPath string, fileIndexPlus1 int, isMultiFile bool) SidebarElementViewModel {
	sidebarElem := SidebarElementViewModel{
		Name:          codeElem.Name,
		FileShortPath: fileShortPath,
		Line:          codeElem.FirstLine,
		Icon:          "cube", // Default to method
	}
	if isMultiFile { // Only set FileIndexPlus1 if it's a multi-file class
		sidebarElem.FileIndexPlus1 = fileIndexPlus1
	}
	if codeElem.Type == model.PropertyElementType {
		sidebarElem.Icon = "wrench"
	}
	if codeElem.CoverageQuota != nil {
		sidebarElem.CoverageBarValue = 100 - int(math.Round(*codeElem.CoverageQuota))
		sidebarElem.CoverageTitle = fmt.Sprintf("Line coverage: %.1f%%", *codeElem.CoverageQuota)
	} else {
		sidebarElem.CoverageBarValue = 0 // Default for N/A
		sidebarElem.CoverageTitle = "Line coverage: N/A"
	}
	return sidebarElem
}

func (b *HtmlReportBuilder) buildMetricsTableForClassVM(classModel *model.Class, allMethodMetricsForClass []*model.MethodMetric) MetricsTableViewModel {
	metricsTable := MetricsTableViewModel{}
	seenMetricTypes := make(map[string]AngularMetricDefinitionViewModel)
	orderedMetricNames := []string{}

	for _, modelMM := range allMethodMetricsForClass {
		for _, modelMetric := range modelMM.Metrics {
			if _, found := seenMetricTypes[modelMetric.Name]; !found {
				explURL := b.getMetricExplanationURL(modelMetric.Name)
				seenMetricTypes[modelMetric.Name] = AngularMetricDefinitionViewModel{Name: modelMetric.Name, ExplanationURL: explURL}
				orderedMetricNames = append(orderedMetricNames, modelMetric.Name)
			}
		}
	}
	sort.Strings(orderedMetricNames) // Consistent header order
	for _, name := range orderedMetricNames {
		metricsTable.Headers = append(metricsTable.Headers, seenMetricTypes[name])
	}

	// Map method metrics to their file context (short path, index)
	methodMetricToFileContext := make(map[*model.MethodMetric]struct {
		Index     int
		ShortPath string
	})
	for fIdx, f := range classModel.Files {
		shortPath := sanitizeFilenameChars.ReplaceAllString(filepath.Base(f.Path), "_")
		for i := range f.MethodMetrics {
			methodMetricToFileContext[&f.MethodMetrics[i]] = struct {
				Index     int
				ShortPath string
			}{Index: fIdx, ShortPath: shortPath}
		}
	}

	for _, modelMM := range allMethodMetricsForClass {
		fileCtx := methodMetricToFileContext[modelMM]
		var correspondingCE *model.CodeElement
		for _, f := range classModel.Files { // Find corresponding CodeElement for coverage/type
			for _, ce := range f.CodeElements {
				if ce.FirstLine == modelMM.Line && strings.HasPrefix(ce.FullName, modelMM.Name) {
					correspondingCE = &ce
					break
				}
			}
			if correspondingCE != nil {
				break
			}
		}

		row := AngularMethodMetricsViewModel{
			Name:           modelMM.Name,
			FullName:       modelMM.Name, // Placeholder, full name with signature usually different
			FileIndexPlus1: fileCtx.Index + 1,
			Line:           modelMM.Line,
			FileShortPath:  fileCtx.ShortPath,
			MetricValues:   make([]string, len(metricsTable.Headers)),
		}
		if correspondingCE != nil {
			row.IsProperty = (correspondingCE.Type == model.PropertyElementType)
			row.CoverageQuota = correspondingCE.CoverageQuota
			row.FullName = correspondingCE.FullName // Use code element's full name
		}

		currentMethodValues := make(map[string]string)
		for _, metric := range modelMM.Metrics {
			currentMethodValues[metric.Name] = b.formatMetricValue(metric)
		}
		for i, header := range metricsTable.Headers {
			if val, ok := currentMethodValues[header.Name]; ok {
				row.MetricValues[i] = val
			} else {
				row.MetricValues[i] = "-" // Metric not present for this method
			}
		}
		metricsTable.Rows = append(metricsTable.Rows, row)
	}
	return metricsTable

}

func (b *HtmlReportBuilder) getMetricExplanationURL(metricName string) string {
	switch metricName {
	case "Cyclomatic Complexity", "Complexity": // "Complexity" is often used for Cyc. Comp.
		return "https://en.wikipedia.org/wiki/Cyclomatic_complexity"
	case "CrapScore":
		return "https://testing.googleblog.com/2011/02/this-code-is-crap.html"
	case "NPathComplexity":
		return "https://modess.io/npath-complexity-cyclomatic-complexity-explained/"
	case "Line coverage", "Branch coverage":
		return "https://en.wikipedia.org/wiki/Code_coverage"
	default:
		return "" // No specific URL for other metrics
	}
}

func (b *HtmlReportBuilder) formatMetricValue(metric model.Metric) string {
	if metric.Value == nil {
		return "-"
	}
	if valFloat, ok := metric.Value.(float64); ok {
		if math.IsNaN(valFloat) {
			return "NaN"
		}
		// Specific formatting for coverage percentages
		if metric.Name == "Line coverage" || metric.Name == "Branch coverage" {
			return fmt.Sprintf("%.1f%%", valFloat) // Assuming value is already a percentage 0-100
		}
		return fmt.Sprintf("%.0f", valFloat) // Default for other float metrics
	}
	if valInt, okInt := metric.Value.(int); okInt {
		return fmt.Sprintf("%d", valInt)
	}
	return fmt.Sprintf("%v", metric.Value) // Fallback for other types
}

func (b *HtmlReportBuilder) buildAngularClassDetailForJS(classModel *model.Class, classVMServer *ClassViewModelForDetail) (AngularClassDetailViewModel, error) {
	// Basic class info from server view model or re-calculate if necessary
	angularClassVMForJS := AngularClassViewModel{
		Name:                  classModel.DisplayName,
		CoveredLines:          classModel.LinesCovered,
		UncoveredLines:        classModel.LinesValid - classModel.LinesCovered,
		CoverableLines:        classModel.LinesValid,
		TotalLines:            classModel.TotalLines,
		CoveredMethods:        classVMServer.CoveredMethods,
		FullyCoveredMethods:   classVMServer.FullyCoveredMethods,
		TotalMethods:          classVMServer.TotalMethods,
		HistoricCoverages:     classVMServer.HistoricCoverages, // Re-use from server VM
		LineCoverageHistory:   classVMServer.LineCoverageHistory,
		BranchCoverageHistory: classVMServer.BranchCoverageHistory,
		Metrics:               classVMServer.Metrics, // Re-use aggregated metrics
	}
	if classModel.BranchesCovered != nil {
		angularClassVMForJS.CoveredBranches = *classModel.BranchesCovered
	}
	if classModel.BranchesValid != nil {
		angularClassVMForJS.TotalBranches = *classModel.BranchesValid
	}

	detailVM := AngularClassDetailViewModel{Class: angularClassVMForJS, Files: []AngularCodeFileViewModel{}}
	if classModel.Files == nil {
		return detailVM, nil
	}

	for _, fileInClass := range classModel.Files {
		angularFileForJS, err := b.buildAngularFileViewModelForJS(&fileInClass)
		if err != nil {
			// Log error, but try to continue
			fmt.Fprintf(os.Stderr, "Error building Angular file view model for JS (%s): %v\n", fileInClass.Path, err)
			continue
		}
		detailVM.Files = append(detailVM.Files, angularFileForJS)
	}
	return detailVM, nil

}

func (b *HtmlReportBuilder) buildAngularFileViewModelForJS(fileInClass *model.CodeFile) (AngularCodeFileViewModel, error) {
	angularFile := AngularCodeFileViewModel{
		Path:           fileInClass.Path,
		CoveredLines:   fileInClass.CoveredLines,
		CoverableLines: fileInClass.CoverableLines,
		TotalLines:     fileInClass.TotalLines,
		Lines:          []AngularLineAnalysisViewModel{},
	}
	sourceLines, err := filereader.ReadLinesInFile(fileInClass.Path)
	if err != nil {
		// If source can't be read, the lines array will be empty.
		// This is consistent with C# RG behavior (no lines if file not found).
		fmt.Fprintf(os.Stderr, "Warning: could not read source file %s for JS Angular VM: %v\n", fileInClass.Path, err)
		return angularFile, nil // Return with empty lines, not a fatal error for this part.
	}

	coverageLinesMap := make(map[int]*model.Line)
	if fileInClass.Lines != nil {
		for i := range fileInClass.Lines { // Use index for pointer
			covLine := &fileInClass.Lines[i]
			coverageLinesMap[covLine.Number] = covLine
		}
	}

	for i, content := range sourceLines {
		actualLineNumber := i + 1
		modelCovLine, hasCoverageData := coverageLinesMap[actualLineNumber]
		angularLine := b.buildAngularLineViewModelForJS(content, actualLineNumber, modelCovLine, hasCoverageData)
		angularFile.Lines = append(angularFile.Lines, angularLine)
	}
	return angularFile, nil

}

func (b *HtmlReportBuilder) buildAngularLineViewModelForJS(content string, actualLineNumber int, modelCovLine *model.Line, hasCoverageData bool) AngularLineAnalysisViewModel {
	lineVM := AngularLineAnalysisViewModel{
		LineNumber:  actualLineNumber,
		LineContent: content, // For JS, we send the raw content
	}
	if hasCoverageData {
		lineVM.Hits = modelCovLine.Hits
		lineVM.CoveredBranches = modelCovLine.CoveredBranches
		lineVM.TotalBranches = modelCovLine.TotalBranches
		lineVM.LineVisitStatus = lineVisitStatusToString(determineLineVisitStatus(modelCovLine.Hits, modelCovLine.IsBranchPoint, modelCovLine.CoveredBranches, modelCovLine.TotalBranches))
	} else {
		lineVM.LineVisitStatus = lineVisitStatusToString(lineVisitStatusNotCoverable)
	}
	return lineVM
}

func (b *HtmlReportBuilder) buildClassDetailPageData(classVM ClassViewModelForDetail, tag string, classDetailJS template.JS) ClassDetailData {
	appVersion := "0.0.1" // Placeholder, same as summary
	if b.ReportContext.ReportConfiguration() != nil {
		appVersion = "0.0.1" 
		// Logic to get actual app version if available
	}
	return ClassDetailData{
		ReportTitle:                           b.reportTitle,
		AppVersion:                            appVersion,
		CurrentDateTime:                       time.Now().Format("02/01/2006 - 15:04:05"),
		Class:                                 classVM,
		BranchCoverageAvailable:               b.branchCoverageAvailable,
		MethodCoverageAvailable:               b.methodCoverageAvailable,
		Tag:                                   tag,
		Translations:                          b.translations,
		MaximumDecimalPlacesForCoverageQuotas: b.maximumDecimalPlacesForCoverageQuotas,
		AngularCssFile:                        b.angularCssFile,
		AngularRuntimeJsFile:                  b.angularRuntimeJsFile,
		AngularPolyfillsJsFile:                b.angularPolyfillsJsFile,
		AngularMainJsFile:                     b.angularMainJsFile,
		AssembliesJSON:                        b.assembliesJSON,
		RiskHotspotsJSON:                      b.riskHotspotsJSON,
		MetricsJSON:                           b.metricsJSON,
		RiskHotspotMetricsJSON:                b.riskHotspotMetricsJSON,
		HistoricCoverageExecutionTimesJSON:    b.historicCoverageExecutionTimesJSON,
		TranslationsJSON:                      b.translationsJSON,
		ClassDetailJSON:                       classDetailJS,
	}
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

// --- Utility and Asset Handling methods ---

func (b *HtmlReportBuilder) getClassReportFilename(assemblyShortName, className string, existingFilenames map[string]struct{}) string {
	processedClassName := className
	if lastDot := strings.LastIndex(className, "."); lastDot != -1 {
		processedClassName = className[lastDot+1:]
	}
	if strings.HasSuffix(strings.ToLower(processedClassName), ".js") {
		processedClassName = processedClassName[:len(processedClassName)-3]
	}
	baseName := assemblyShortName + "" + processedClassName
	sanitizedName := sanitizeFilenameChars.ReplaceAllString(baseName, "")
	maxLengthBase := 95
	if len(sanitizedName) > maxLengthBase {
		if maxLengthBase > 50 {
			sanitizedName = sanitizedName[:50] + sanitizedName[len(sanitizedName)-(maxLengthBase-50):]
		} else {
			sanitizedName = sanitizedName[:maxLengthBase]
		}
	}
	fileName := sanitizedName + ".html"
	counter := 1
	normalizedFileNameToCheck := strings.ToLower(fileName)
	_, exists := existingFilenames[normalizedFileNameToCheck]
	for exists {
		counter++
		fileName = fmt.Sprintf("%s%d.html", sanitizedName, counter)
		normalizedFileNameToCheck = strings.ToLower(fileName)
		_, exists = existingFilenames[normalizedFileNameToCheck]
	}
	existingFilenames[normalizedFileNameToCheck] = struct{}{}
	return fileName
}

func (b *HtmlReportBuilder) parseAngularIndexHTML(angularIndexHTMLPath string) (cssFile, runtimeJs, polyfillsJs, mainJs string, err error) {
	file, err := os.Open(angularIndexHTMLPath)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to open Angular index.html at %s: %w", angularIndexHTMLPath, err)
	}
	defer file.Close()

	doc, err := html.Parse(file)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to parse Angular index.html: %w", err)
	}
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "link" {
				isStylesheet := false
				var href string
				for _, a := range n.Attr {
					if a.Key == "rel" && a.Val == "stylesheet" {
						isStylesheet = true
					}
					if a.Key == "href" {
						href = a.Val
					}
				}
				if isStylesheet && href != "" {
					cssFile = href
				}
			} else if n.Data == "script" {
				var src string
				for _, a := range n.Attr {
					if a.Key == "src" {
						src = a.Val
						break
					}
				}
				if src != "" {
					if strings.HasPrefix(filepath.Base(src), "runtime.") && strings.HasSuffix(src, ".js") {
						runtimeJs = src
					} else if strings.HasPrefix(filepath.Base(src), "polyfills.") && strings.HasSuffix(src, ".js") {
						polyfillsJs = src
					} else if strings.HasPrefix(filepath.Base(src), "main.") && strings.HasSuffix(src, ".js") {
						mainJs = src
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	// Validation already happens in initializeAssets
	return cssFile, runtimeJs, polyfillsJs, mainJs, nil

}

func (b *HtmlReportBuilder) copyAngularAssets(outputDir string) error {
	srcInfo, err := os.Stat(angularDistSourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("angular source directory %s does not exist: %w. Make sure to build the Angular app first ('npm run build' in 'go_report_generator/angular_frontend_spa')", angularDistSourcePath, err)
		}
		return fmt.Errorf("failed to stat angular source directory %s: %w", angularDistSourcePath, err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("angular source path %s is not a directory", angularDistSourcePath)
	}
	return filepath.WalkDir(angularDistSourcePath, func(srcPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("error accessing path %s during walk: %w", srcPath, walkErr)
		}
		relPath, err := filepath.Rel(angularDistSourcePath, srcPath)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", srcPath, err)
		}
		dstPath := filepath.Join(outputDir, relPath)
		if d.IsDir() {
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dstPath, err)
			}
		} else {
			srcFile, err := os.Open(srcPath)
			if err != nil {
				return fmt.Errorf("failed to open source file %s: %w", srcPath, err)
			}
			defer srcFile.Close()
			if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory for %s: %w", dstPath, err)
			}
			dstFile, err := os.Create(dstPath)
			if err != nil {
				return fmt.Errorf("failed to create destination file %s: %w", dstPath, err)
			}
			defer dstFile.Close()
			if _, err := io.Copy(dstFile, srcFile); err != nil {
				return fmt.Errorf("failed to copy file from %s to %s: %w", srcPath, dstPath, err)
			}
			srcFileInfo, statErr := d.Info()
			if statErr == nil {
				os.Chmod(dstPath, srcFileInfo.Mode())
			}
		}
		return nil
	})
}

func (b *HtmlReportBuilder) copyStaticAssets() error {
	filesToCopy := []string{
		"custom.css", "custom.js", "chartist.min.css", "chartist.min.js",
		"custom-azurepipelines.css", "custom-azurepipelines_adaptive.css",
		"custom-azurepipelines_dark.css", "custom_adaptive.css",
		"custom_bluered.css", "custom_dark.css",
	}
	for _, fileName := range filesToCopy {
		srcPath := filepath.Join(assetsDir, fileName)
		dstPath := filepath.Join(b.OutputDir, fileName)
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for asset %s: %w", dstPath, err)
		}
		srcFile, err := os.Open(srcPath)
		if err != nil {
			return fmt.Errorf("failed to open source asset %s (abs: %s): %w", fileName, srcPath, err)
		}
		defer srcFile.Close()
		dstFile, err := os.Create(dstPath)
		if err != nil {
			return fmt.Errorf("failed to create destination asset %s: %w", dstPath, err)
		}
		defer dstFile.Close()
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return fmt.Errorf("failed to copy asset from %s to %s: %w", srcPath, dstPath, err)
		}
	}
	customCSSBytes, err := os.ReadFile(filepath.Join(assetsDir, "custom.css"))
	if err != nil {
		return fmt.Errorf("failed to read custom.css: %w", err)
	}
	customDarkCSSBytes, err := os.ReadFile(filepath.Join(assetsDir, "custom_dark.css"))
	if err != nil {
		return fmt.Errorf("failed to read custom_dark.css: %w", err)
	}
	combinedCSS := append(customCSSBytes, []byte("\n")...)
	combinedCSS = append(combinedCSS, customDarkCSSBytes...)
	return os.WriteFile(filepath.Join(b.OutputDir, "report.css"), combinedCSS, 0644)
}

// --- Helper functions for determining line status and simple counts ---
func determineLineVisitStatus(hits int, isBranchPoint bool, coveredBranches int, totalBranches int) int {
	if hits < 0 {
		return lineVisitStatusNotCoverable
	}
	if isBranchPoint {
		if totalBranches == 0 {
			return lineVisitStatusNotCoverable
		}
		if coveredBranches == totalBranches {
			return lineVisitStatusCovered
		}
		if coveredBranches > 0 {
			return lineVisitStatusPartiallyCovered
		}
		return lineVisitStatusNotCovered
	}
	if hits > 0 {
		return lineVisitStatusCovered
	}
	return lineVisitStatusNotCovered
}

func lineVisitStatusToString(status int) string {
	switch status {
	case lineVisitStatusCovered:
		return "green"
	case lineVisitStatusNotCovered:
		return "red"
	case lineVisitStatusPartiallyCovered:
		return "orange"
	default: // lineVisitStatusNotCoverable
		return "gray"
	}
}

func countTotalClasses(assemblies []model.Assembly) int {
	count := 0
	for _, asm := range assemblies {
		count += len(asm.Classes)
	}
	return count
}

func countUniqueFiles(assemblies []model.Assembly) int {
	uniqueFiles := make(map[string]bool)
	for _, asm := range assemblies {
		for _, cls := range asm.Classes {
			for _, f := range cls.Files {
				uniqueFiles[f.Path] = true
			}
		}
	}
	return len(uniqueFiles)
}
