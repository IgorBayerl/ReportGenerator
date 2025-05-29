package htmlreport

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"math" // Added for math.IsNaN, math.IsInf
	"os"
	"path/filepath"
	"regexp"
	"sort" // Added for sorting metric headers
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

type HTMLReportData struct {
	Title                                 string
	ParserName                            string
	GeneratedAt                           string
	Content                               template.HTML
	AngularCssFile                        string
	AngularRuntimeJsFile                  string
	AngularPolyfillsJsFile                string
	AngularMainJsFile                     string
	AssembliesJSON                        template.JS
	RiskHotspotsJSON                      template.JS
	MetricsJSON                           template.JS
	RiskHotspotMetricsJSON                template.JS
	HistoricCoverageExecutionTimesJSON    template.JS
	TranslationsJSON                      template.JS
	ClassDetailJSON                       template.JS `json:"-"`
	BranchCoverageAvailable               bool
	MethodCoverageAvailable               bool
	MaximumDecimalPlacesForCoverageQuotas int
}

type HtmlReportBuilder struct {
	OutputDir     string
	ReportContext reporting.IReportContext // Store the context

	// Fields for pre-marshaled global JSON data, accessible by all page generation methods
	angularCssFile                     string
	angularRuntimeJsFile               string
	angularPolyfillsJsFile             string
	angularMainJsFile                  string
	assembliesJSON                     template.JS
	riskHotspotsJSON                   template.JS
	metricsJSON                        template.JS
	riskHotspotMetricsJSON             template.JS
	historicCoverageExecutionTimesJSON template.JS
	translationsJSON                   template.JS // Marshaled translations for <script> block

	// Settings derived from context
	branchCoverageAvailable               bool
	methodCoverageAvailable               bool // For PRO feature display
	maximumDecimalPlacesForCoverageQuotas int
	parserName                            string
	reportTimestamp                       int64
	reportTitle                           string
	tag                                   string
	translations                          map[string]string // Raw translations map for Go template text
	onlySummary                           bool              // TODO: This flag might need to be set based on ReportContext.Settings if it exists
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
	if b.ReportContext == nil {
		return fmt.Errorf("HtmlReportBuilder.ReportContext is not set; it's required for configuration and settings")
	}
	reportConfig := b.ReportContext.ReportConfiguration()
	settings := b.ReportContext.Settings()

	if err := os.MkdirAll(b.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", b.OutputDir, err)
	}
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
		if cssFile == "" || runtimeJs == "" || mainJs == "" {
			return fmt.Errorf("missing critical Angular assets from index.html (css: '%s', runtime: '%s', main: '%s')", cssFile, runtimeJs, mainJs)
		}
	}

	// Store common builder properties
	b.angularCssFile = cssFile
	b.angularRuntimeJsFile = runtimeJs
	b.angularPolyfillsJsFile = polyfillsJs
	b.angularMainJsFile = mainJs
	b.reportTitle = reportConfig.Title()
	if b.reportTitle == "" {
		b.reportTitle = "Summary" // Default for summary page
	}
	b.parserName = report.ParserName
	b.reportTimestamp = report.Timestamp
	b.tag = reportConfig.Tag()
	b.branchCoverageAvailable = report.BranchesValid != nil && *report.BranchesValid > 0
	b.methodCoverageAvailable = true // Per original C# behavior, Pro features relate to this
	b.maximumDecimalPlacesForCoverageQuotas = settings.MaximumDecimalPlacesForCoverageQuotas
	b.translations = GetTranslations()

	// Pre-marshal global JSON data for all pages
	translationsJSONBytes, err := json.Marshal(b.translations)
	if err != nil {
		b.translationsJSON = template.JS("({})")
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

	var executionTimes []string
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
	for ts := range uniqueExecutionTimestamps {
		executionTimes = append(executionTimes, time.Unix(ts, 0).Format("2006-01-02 15:04:05"))
	}
	historicExecTimesJSONBytes, err := json.Marshal(executionTimes)
	if err != nil {
		b.historicCoverageExecutionTimesJSON = template.JS("([])")
	} else {
		b.historicCoverageExecutionTimesJSON = template.JS(historicExecTimesJSONBytes)
	}

	var angularAssemblies []AngularAssemblyViewModel
	summaryPageAssemblyFilenames := make(map[string]struct{}) // Used to generate unique filenames for class links from summary
	if report.Assemblies != nil {
		for _, assembly := range report.Assemblies {
			assemblyShortName := assembly.Name
			angularAssembly := AngularAssemblyViewModel{Name: assembly.Name, Classes: []AngularClassViewModel{}}
			for _, class := range assembly.Classes {
				classReportFilename := b.getClassReportFilename(assemblyShortName, class.Name, summaryPageAssemblyFilenames)
				angularClass := AngularClassViewModel{
					Name: class.DisplayName, ReportPath: classReportFilename, CoveredLines: class.LinesCovered,
					CoverableLines: class.LinesValid, UncoveredLines: class.LinesValid - class.LinesCovered, TotalLines: class.TotalLines,
					Metrics: make(map[string]float64), HistoricCoverages: []AngularHistoricCoverageViewModel{},
				}
				if len(class.Methods) > 0 {
					angularClass.TotalMethods = len(class.Methods)
					coveredMethodCount, fullCoveredMethodCount := 0, 0
					for _, m := range class.Methods {
						if m.LineRate > 0 {
							coveredMethodCount++
						}
						if m.LineRate >= (1.0-1e-9) && len(m.Lines) > 0 {
							fullCoveredMethodCount++
						}
					}
					angularClass.CoveredMethods = coveredMethodCount
					angularClass.FullyCoveredMethods = fullCoveredMethodCount
				}
				if class.BranchesCovered != nil {
					angularClass.CoveredBranches = *class.BranchesCovered
				}
				if class.BranchesValid != nil {
					angularClass.TotalBranches = *class.BranchesValid
				}

				if class.HistoricCoverages != nil {
					for _, hist := range class.HistoricCoverages {
						angularHist := AngularHistoricCoverageViewModel{ExecutionTime: time.Unix(hist.ExecutionTime, 0).Format("2006-01-02"), CoveredLines: hist.CoveredLines, CoverableLines: hist.CoverableLines, TotalLines: hist.TotalLines, CoveredBranches: hist.CoveredBranches, TotalBranches: hist.TotalBranches}
						if hist.CoverableLines > 0 {
							angularHist.LineCoverageQuota = (float64(hist.CoveredLines) / float64(hist.CoverableLines)) * 100
						}
						if hist.TotalBranches > 0 {
							angularHist.BranchCoverageQuota = (float64(hist.CoveredBranches) / float64(hist.TotalBranches)) * 100
						}
						angularClass.HistoricCoverages = append(angularClass.HistoricCoverages, angularHist)
						if angularHist.LineCoverageQuota >= 0 {
							angularClass.LineCoverageHistory = append(angularClass.LineCoverageHistory, angularHist.LineCoverageQuota)
						}
						if angularHist.BranchCoverageQuota >= 0 {
							angularClass.BranchCoverageHistory = append(angularClass.BranchCoverageHistory, angularHist.BranchCoverageQuota)
						}
					}
				}
				tempMetrics := make(map[string][]float64)
				if class.Methods != nil {
					for _, method := range class.Methods {
						if method.MethodMetrics != nil {
							for _, methodMetric := range method.MethodMetrics {
								if methodMetric.Metrics != nil {
									for _, metric := range methodMetric.Metrics {
										if metric.Name == "" {
											continue
										}
										if valFloat, ok := metric.Value.(float64); ok {
											if !math.IsNaN(valFloat) && !math.IsInf(valFloat, 0) {
												tempMetrics[metric.Name] = append(tempMetrics[metric.Name], valFloat)
											}
										}
									}
								}
							}
						}
					}
				}
				for name, values := range tempMetrics {
					if len(values) > 0 {
						var sum float64
						for _, v := range values {
							sum += v
						}
						angularClass.Metrics[name] = sum
					}
				}
				angularAssembly.Classes = append(angularAssembly.Classes, angularClass)
			}
			angularAssemblies = append(angularAssemblies, angularAssembly)
		}
	}
	assembliesJSONBytes, err := json.Marshal(angularAssemblies)
	if err != nil {
		b.assembliesJSON = template.JS("([])")
	} else {
		b.assembliesJSON = template.JS(assembliesJSONBytes)
	}

	var angularRiskHotspots []AngularRiskHotspotViewModel // Keep as empty for now for summary page JSON
	riskHotspotsJSONBytes, err := json.Marshal(angularRiskHotspots)
	if err != nil {
		b.riskHotspotsJSON = template.JS("([])")
	} else {
		b.riskHotspotsJSON = template.JS(riskHotspotsJSONBytes)
	}

	// === Render Summary Page (index.html) ===
	summaryData := SummaryPageData{
		ReportTitle:                           b.reportTitle,
		AppVersion:                            "0.0.1", // Placeholder
		CurrentDateTime:                       time.Now().Format("02/01/2006 - 15:04:05"),
		Translations:                          b.translations,
		HasRiskHotspots:                       len(angularRiskHotspots) > 0, // Based on data intended for <risk-hotspots>
		HasAssemblies:                         len(report.Assemblies) > 0,
		AssembliesJSON:                        b.assembliesJSON,         // For <coverage-info>
		RiskHotspotsJSON:                      b.riskHotspotsJSON,       // For <risk-hotspots>
		MetricsJSON:                           b.metricsJSON,            // For <coverage-info> options
		RiskHotspotMetricsJSON:                b.riskHotspotMetricsJSON, // For <risk-hotspots>
		HistoricCoverageExecutionTimesJSON:    b.historicCoverageExecutionTimesJSON,
		TranslationsJSON:                      b.translationsJSON, // For all components
		AngularCssFile:                        b.angularCssFile,
		AngularRuntimeJsFile:                  b.angularRuntimeJsFile,
		AngularPolyfillsJsFile:                b.angularPolyfillsJsFile,
		AngularMainJsFile:                     b.angularMainJsFile,
		BranchCoverageAvailable:               b.branchCoverageAvailable,
		MethodCoverageAvailable:               b.methodCoverageAvailable,
		MaximumDecimalPlacesForCoverageQuotas: b.maximumDecimalPlacesForCoverageQuotas,
	}

	// Populate SummaryCards for server-side rendering in summaryPageTpl
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
	summaryData.SummaryCards = append(summaryData.SummaryCards, CardViewModel{Title: b.translations["Information"], Rows: infoCardRows})

	lineCovQuota := 0.0
	lineCovText := "N/A"
	lineCovTooltip := "-"
	lineCovBar := 0
	if report.LinesValid > 0 {
		lineCovQuota = (float64(report.LinesCovered) / float64(report.LinesValid)) * 100.0
		lineCovText = fmt.Sprintf("%.1f%%", lineCovQuota)
		lineCovTooltip = fmt.Sprintf("%d of %d", report.LinesCovered, report.LinesValid)
		lineCovBar = 100 - int(math.Round(lineCovQuota))
	}
	summaryData.SummaryCards = append(summaryData.SummaryCards, CardViewModel{Title: b.translations["LineCoverage"], SubTitle: lineCovText, SubTitlePercentageBarValue: lineCovBar, Rows: []CardRowViewModel{
		{Header: b.translations["CoveredLines"], Text: fmt.Sprintf("%d", report.LinesCovered), Alignment: "right"}, {Header: b.translations["UncoveredLines"], Text: fmt.Sprintf("%d", report.LinesValid-report.LinesCovered), Alignment: "right"},
		{Header: b.translations["CoverableLines"], Text: fmt.Sprintf("%d", report.LinesValid), Alignment: "right"}, {Header: b.translations["TotalLines"], Text: fmt.Sprintf("%d", report.TotalLines), Alignment: "right"},
		{Header: b.translations["LineCoverage"], Text: lineCovText, Tooltip: lineCovTooltip, Alignment: "right"},
	}})
	if b.branchCoverageAvailable && report.BranchesCovered != nil && report.BranchesValid != nil {
		branchCovQuota := 0.0
		branchCovText := "N/A"
		branchCovTooltip := "-"
		branchCovBar := 0
		if *report.BranchesValid > 0 {
			branchCovQuota = (float64(*report.BranchesCovered) / float64(*report.BranchesValid)) * 100.0
			branchCovText = fmt.Sprintf("%.1f%%", branchCovQuota)
			branchCovTooltip = fmt.Sprintf("%d of %d", *report.BranchesCovered, *report.BranchesValid)
			branchCovBar = 100 - int(math.Round(branchCovQuota))
		}
		summaryData.SummaryCards = append(summaryData.SummaryCards, CardViewModel{Title: b.translations["BranchCoverage"], SubTitle: branchCovText, SubTitlePercentageBarValue: branchCovBar, Rows: []CardRowViewModel{
			{Header: b.translations["CoveredBranches2"], Text: fmt.Sprintf("%d", *report.BranchesCovered), Alignment: "right"}, {Header: b.translations["TotalBranches"], Text: fmt.Sprintf("%d", *report.BranchesValid), Alignment: "right"},
			{Header: b.translations["BranchCoverage"], Text: branchCovText, Tooltip: branchCovTooltip, Alignment: "right"},
		}})
	}
	totalMethods, coveredMethods, fullyCoveredMethods := 0, 0, 0
	for _, asm := range report.Assemblies {
		for _, cls := range asm.Classes {
			totalMethods += len(cls.Methods)
			for _, m := range cls.Methods {
				if m.LineRate > 0 {
					coveredMethods++
				}
				if m.LineRate >= (1.0-1e-9) && len(m.Lines) > 0 {
					fullyCoveredMethods++
				}
			}
		}
	}
	methodCovText := "N/A"
	methodCovBar := 0
	methodCovTooltip := "-"
	fullMethodCovText := "N/A"
	fullMethodCovTooltip := "-"
	if totalMethods > 0 {
		methodRate := (float64(coveredMethods) / float64(totalMethods)) * 100.0
		methodCovText = fmt.Sprintf("%.1f%%", methodRate)
		methodCovBar = 100 - int(math.Round(methodRate))
		methodCovTooltip = fmt.Sprintf("%d of %d", coveredMethods, totalMethods)
		fullMethodRate := (float64(fullyCoveredMethods) / float64(totalMethods)) * 100.0
		fullMethodCovText = fmt.Sprintf("%.1f%%", fullMethodRate)
		fullMethodCovTooltip = fmt.Sprintf("%d of %d", fullyCoveredMethods, totalMethods)
	}
	summaryData.SummaryCards = append(summaryData.SummaryCards, CardViewModel{
		Title: b.translations["MethodCoverage"], ProRequired: !b.methodCoverageAvailable, SubTitle: methodCovText, SubTitlePercentageBarValue: methodCovBar,
		Rows: []CardRowViewModel{
			{Header: b.translations["CoveredCodeElements"], Text: fmt.Sprintf("%d", coveredMethods), Alignment: "right"}, {Header: b.translations["FullCoveredCodeElements"], Text: fmt.Sprintf("%d", fullyCoveredMethods), Alignment: "right"},
			{Header: b.translations["TotalCodeElements"], Text: fmt.Sprintf("%d", totalMethods), Alignment: "right"},
			{Header: b.translations["CodeElementCoverageQuota2"], Text: methodCovText, Tooltip: methodCovTooltip, Alignment: "right"},
			{Header: b.translations["FullCodeElementCoverageQuota2"], Text: fullMethodCovText, Tooltip: fullMethodCovTooltip, Alignment: "right"},
		},
	})
	summaryData.OverallHistoryChartData = HistoryChartDataViewModel{Series: false} // Simplified: no server-rendered history chart on summary for now

	outputIndexPath := filepath.Join(b.OutputDir, "index.html")
	summaryFile, err := os.Create(outputIndexPath)
	if err != nil {
		return fmt.Errorf("failed to create index.html: %w", err)
	}
	defer summaryFile.Close()
	if err := summaryPageTpl.Execute(summaryFile, summaryData); err != nil {
		return fmt.Errorf("failed to execute summary page template: %w", err)
	}
	summaryFile.Close()

	// === Render Class Detail Pages ===
	if !b.onlySummary {
		// Create a new map for filenames for class detail pages to avoid conflicts with summary page's usage (if any)
		classPageFilenames := make(map[string]struct{})

		for _, assemblyModel := range report.Assemblies {
			for _, classModel := range assemblyModel.Classes {
				// Determine filename. Since angularAssemblies was for summary page's JS,
				// we might need to call getClassReportFilename again here if paths differ or ensure consistency.
				// For now, assume angularAssemblies has the correct ReportPath for linking.
				var classReportFilename string
				foundFilename := false
				for _, asmView := range angularAssemblies { // angularAssemblies created for summary page JS
					if asmView.Name == assemblyModel.Name {
						for _, classView := range asmView.Classes {
							if classView.Name == classModel.DisplayName {
								classReportFilename = classView.ReportPath // Get the pre-determined path
								// Add to classPageFilenames to ensure it's "reserved" if getClassReportFilename is called again with this map
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
					fmt.Fprintf(os.Stderr, "Warning: Could not determine report filename for class %s in CreateReport, using direct generation.\n", classModel.DisplayName)
					// Fallback: generate filename directly for this context
					classReportFilename = b.getClassReportFilename(assemblyModel.Name, classModel.Name, classPageFilenames)
				}

				err := b.generateClassDetailHTML(&classModel, classReportFilename, b.tag)
				if err != nil {
					return fmt.Errorf("failed to generate detail page for class '%s' (file: %s): %w", classModel.DisplayName, classReportFilename, err)
				}
			}
		}
	}
	return nil
}

const (
	lineVisitStatusNotCoverable     = 0
	lineVisitStatusCovered          = 1
	lineVisitStatusNotCovered       = 2
	lineVisitStatusPartiallyCovered = 3
)

func determineLineVisitStatus(hits int, isBranchPoint bool, coveredBranches int, totalBranches int) int {
	// Based on C# ReportGenerator.Core.Parser.Analysis.LineVisitStatus logic
	// and how CoberturaParser sets it.
	// If hits < 0, it's considered not coverable by some parsers.
	if hits < 0 {
		return lineVisitStatusNotCoverable
	}

	if isBranchPoint {
		if totalBranches == 0 { // A branch point with no actual branches defined
			return lineVisitStatusNotCoverable // Or Covered if hits > 0, C# seems to lean towards NotCoverable if no branches
		}
		if coveredBranches == totalBranches {
			return lineVisitStatusCovered
		}
		if coveredBranches > 0 {
			return lineVisitStatusPartiallyCovered
		}
		// hits can be > 0 but coveredBranches == 0 if the line itself was hit but no branches taken
		// or hits == 0 and coveredBranches == 0
		return lineVisitStatusNotCovered
	}

	// Not a branch point
	if hits > 0 {
		return lineVisitStatusCovered
	}
	return lineVisitStatusNotCovered
}

func lineVisitStatusToString(status int) string {
	switch status {
	case lineVisitStatusCovered:
		return "green" // Used for CSS class .lightgreen
	case lineVisitStatusNotCovered:
		return "red" // Used for CSS class .lightred
	case lineVisitStatusPartiallyCovered:
		return "orange" // Used for CSS class .lightorange
	case lineVisitStatusNotCoverable:
		return "gray" // Used for CSS class .lightgray (though C# often just uses empty for this)
	default:
		return "gray"
	}
}

func (b *HtmlReportBuilder) generateClassDetailHTML(classModel *model.Class, classReportFilename string, tag string) error {
	classVM := ClassViewModelForDetail{
		Name: classModel.DisplayName,
	}
	if dotIndex := strings.LastIndex(classModel.Name, "."); dotIndex > -1 {
		classVM.AssemblyName = classModel.Name[:dotIndex]
	} else {
		classVM.AssemblyName = "Default"
	}

	classVM.CoveredLines = classModel.LinesCovered
	classVM.CoverableLines = classModel.LinesValid
	classVM.UncoveredLines = classVM.CoverableLines - classVM.CoveredLines
	classVM.TotalLines = classModel.TotalLines

	if classVM.CoverableLines > 0 {
		lineCoverage := (float64(classVM.CoveredLines) / float64(classVM.CoverableLines)) * 100.0
		classVM.CoveragePercentageForDisplay = fmt.Sprintf("%.1f%%", lineCoverage)
		classVM.CoveragePercentageBarValue = 100 - int(math.Round(lineCoverage))
		classVM.CoverageRatioTextForDisplay = fmt.Sprintf("%d of %d", classVM.CoveredLines, classVM.CoverableLines)
	} else {
		classVM.CoveragePercentageForDisplay = "N/A"
		classVM.CoveragePercentageBarValue = 0
		classVM.CoverageRatioTextForDisplay = "-"
	}

	if classModel.BranchesValid != nil && *classModel.BranchesValid > 0 && classModel.BranchesCovered != nil {
		classVM.CoveredBranches = *classModel.BranchesCovered
		classVM.TotalBranches = *classModel.BranchesValid
		branchCoverage := (float64(classVM.CoveredBranches) / float64(classVM.TotalBranches)) * 100.0
		classVM.BranchCoveragePercentageForDisplay = fmt.Sprintf("%.1f%%", branchCoverage)
		classVM.BranchCoveragePercentageBarValue = 100 - int(math.Round(branchCoverage))
		classVM.BranchCoverageRatioTextForDisplay = fmt.Sprintf("%d of %d", classVM.CoveredBranches, classVM.TotalBranches)
	} else {
		classVM.BranchCoveragePercentageForDisplay = "N/A"
		classVM.BranchCoveragePercentageBarValue = 0
		classVM.BranchCoverageRatioTextForDisplay = "-"
	}

	classVM.TotalMethods = len(classModel.Methods)
	if classVM.TotalMethods > 0 {
		coveredMethodCount := 0
		fullCoveredMethodCount := 0
		for _, m := range classModel.Methods {
			if m.LineRate > 0 {
				coveredMethodCount++
			}
			if m.LineRate >= (1.0-1e-9) && len(m.Lines) > 0 {
				fullCoveredMethodCount++
			}
		}
		classVM.CoveredMethods = coveredMethodCount
		classVM.FullyCoveredMethods = fullCoveredMethodCount
		methodCov := (float64(classVM.CoveredMethods) / float64(classVM.TotalMethods)) * 100.0
		fullMethodCovVal := (float64(classVM.FullyCoveredMethods) / float64(classVM.TotalMethods)) * 100.0
		classVM.MethodCoveragePercentageForDisplay = fmt.Sprintf("%.1f%%", methodCov)
		classVM.MethodCoveragePercentageBarValue = 100 - int(math.Round(methodCov))
		classVM.MethodCoverageRatioTextForDisplay = fmt.Sprintf("%d of %d", classVM.CoveredMethods, classVM.TotalMethods)
		classVM.FullMethodCoveragePercentageForDisplay = fmt.Sprintf("%.1f%%", fullMethodCovVal)
		classVM.FullMethodCoverageRatioTextForDisplay = fmt.Sprintf("%d of %d", classVM.FullyCoveredMethods, classVM.TotalMethods)
	} else {
		classVM.MethodCoveragePercentageForDisplay = "N/A"
		classVM.MethodCoveragePercentageBarValue = 0
		classVM.MethodCoverageRatioTextForDisplay = "-"
		classVM.FullMethodCoveragePercentageForDisplay = "N/A"
		classVM.FullMethodCoverageRatioTextForDisplay = "-"
	}
	classVM.IsMultiFile = len(classModel.Files) > 1

	var serverSideHistoricCoverages []AngularHistoricCoverageViewModel   // For server-side template, if needed for charts directly
	var angularHistoricCoveragesForJS []AngularHistoricCoverageViewModel // For JS window.classDetails.class.hc
	var lineCoverageHistoryForJS []float64
	var branchCoverageHistoryForJS []float64

	if classModel.HistoricCoverages != nil {
		for _, hist := range classModel.HistoricCoverages {
			angularHist := AngularHistoricCoverageViewModel{
				ExecutionTime: time.Unix(hist.ExecutionTime, 0).Format("2006-01-02"), CoveredLines: hist.CoveredLines, CoverableLines: hist.CoverableLines,
				TotalLines: hist.TotalLines, CoveredBranches: hist.CoveredBranches, TotalBranches: hist.TotalBranches,
			}
			if hist.CoverableLines > 0 {
				angularHist.LineCoverageQuota = (float64(hist.CoveredLines) / float64(hist.CoverableLines)) * 100.0
			}
			if hist.TotalBranches > 0 {
				angularHist.BranchCoverageQuota = (float64(hist.CoveredBranches) / float64(hist.TotalBranches)) * 100.0
			}

			serverSideHistoricCoverages = append(serverSideHistoricCoverages, angularHist)     // If template renders charts
			angularHistoricCoveragesForJS = append(angularHistoricCoveragesForJS, angularHist) // For JS
			if angularHist.LineCoverageQuota >= 0 {
				lineCoverageHistoryForJS = append(lineCoverageHistoryForJS, angularHist.LineCoverageQuota)
			}
			if angularHist.BranchCoverageQuota >= 0 {
				branchCoverageHistoryForJS = append(branchCoverageHistoryForJS, angularHist.BranchCoverageQuota)
			}
		}
	}
	classVM.HistoricCoverages = serverSideHistoricCoverages    // For Go template
	classVM.LineCoverageHistory = lineCoverageHistoryForJS     // For Go template, if it renders mini charts
	classVM.BranchCoverageHistory = branchCoverageHistoryForJS // For Go template

	classAggregatedMetrics := make(map[string]float64)
	if classModel.Methods != nil {
		for _, method := range classModel.Methods {
			for _, methodMetric := range method.MethodMetrics {
				for _, metric := range methodMetric.Metrics {
					if valFloat, ok := metric.Value.(float64); ok {
						if !math.IsNaN(valFloat) && !math.IsInf(valFloat, 0) {
							classAggregatedMetrics[metric.Name] += valFloat
						}
					}
				}
			}
		}
	}
	classVM.Metrics = classAggregatedMetrics

	var allMethodMetricsForClass []*model.MethodMetric
	for fileIdx, fileInClass := range classModel.Files {
		sourceFileLines, _ := filereader.ReadLinesInFile(fileInClass.Path)
		fileVM := FileViewModelForDetail{Path: fileInClass.Path, ShortPath: sanitizeFilenameChars.ReplaceAllString(filepath.Base(fileInClass.Path), "_")}
		coverageLinesMap := make(map[int]*model.Line)
		for i := range fileInClass.Lines {
			covLine := &fileInClass.Lines[i]
			coverageLinesMap[covLine.Number] = covLine
		}
		for lineNumIdx, lineContent := range sourceFileLines {
			actualLineNumber := lineNumIdx + 1
			modelCovLine, hasCoverageData := coverageLinesMap[actualLineNumber]
			lineVM := LineViewModelForDetail{LineNumber: actualLineNumber, LineContent: lineContent}
			dataCoverageMap := map[string]map[string]string{"AllTestMethods": {"VC": "", "LVS": "gray"}}
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
				default:
					lineVM.Tooltip = "Not coverable"
				}
			} else {
				lineVM.LineVisitStatus = "gray"
				lineVM.Hits = ""
				lineVM.Tooltip = "Not coverable"
			}
			dataCoverageBytes, _ := json.Marshal(dataCoverageMap)
			lineVM.DataCoverage = template.JS(dataCoverageBytes)
			fileVM.Lines = append(fileVM.Lines, lineVM)
		}
		classVM.Files = append(classVM.Files, fileVM)
		for i := range fileInClass.MethodMetrics {
			allMethodMetricsForClass = append(allMethodMetricsForClass, &fileInClass.MethodMetrics[i])
		}
		for _, codeElem := range fileInClass.CodeElements {
			sidebarElem := SidebarElementViewModel{Name: codeElem.Name, FileShortPath: fileVM.ShortPath, FileIndexPlus1: fileIdx + 1, Line: codeElem.FirstLine, Icon: "cube"}
			if codeElem.Type == model.PropertyElementType {
				sidebarElem.Icon = "wrench"
			}
			if codeElem.CoverageQuota != nil {
				sidebarElem.CoverageBarValue = 100 - int(math.Round(*codeElem.CoverageQuota))
				sidebarElem.CoverageTitle = fmt.Sprintf("Line coverage: %.1f%%", *codeElem.CoverageQuota)
			} else {
				sidebarElem.CoverageBarValue = 0
				sidebarElem.CoverageTitle = "Line coverage: N/A"
			}
			classVM.SidebarElements = append(classVM.SidebarElements, sidebarElem)
		}
	}

	if len(allMethodMetricsForClass) > 0 {
		classVM.FilesWithMetrics = true
		metricsTable := MetricsTableViewModel{}
		seenMetricTypes := make(map[string]AngularMetricDefinitionViewModel)
		orderedMetricNames := []string{}
		for _, modelMM := range allMethodMetricsForClass {
			for _, modelMetric := range modelMM.Metrics {
				if _, found := seenMetricTypes[modelMetric.Name]; !found {
					var explURL string
					switch modelMetric.Name {
					case "Cyclomatic Complexity":
						explURL = "https://en.wikipedia.org/wiki/Cyclomatic_complexity"
					case "CrapScore":
						explURL = "https://testing.googleblog.com/2011/02/this-code-is-crap.html"
					case "Line coverage", "Branch coverage":
						explURL = "https://en.wikipedia.org/wiki/Code_coverage"
					}
					seenMetricTypes[modelMetric.Name] = AngularMetricDefinitionViewModel{Name: modelMetric.Name, ExplanationURL: explURL}
					orderedMetricNames = append(orderedMetricNames, modelMetric.Name)
				}
			}
		}
		sort.Strings(orderedMetricNames)
		for _, name := range orderedMetricNames {
			metricsTable.Headers = append(metricsTable.Headers, seenMetricTypes[name])
		}
		methodMetricToFileContext := make(map[*model.MethodMetric]struct {
			Index     int
			ShortPath string
		})
		for fIdx, f := range classModel.Files {
			for i := range f.MethodMetrics {
				mmPtr := &f.MethodMetrics[i]
				methodMetricToFileContext[mmPtr] = struct {
					Index     int
					ShortPath string
				}{Index: fIdx, ShortPath: sanitizeFilenameChars.ReplaceAllString(filepath.Base(f.Path), "_")}
			}
		}
		for _, modelMM := range allMethodMetricsForClass {
			fileCtx, _ := methodMetricToFileContext[modelMM]
			var correspondingCE *model.CodeElement
			var methodLineCoverageQuota *float64
			isProperty := false
			for _, f := range classModel.Files {
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
			if correspondingCE != nil {
				methodLineCoverageQuota = correspondingCE.CoverageQuota
				isProperty = (correspondingCE.Type == model.PropertyElementType)
			}
			row := AngularMethodMetricsViewModel{Name: modelMM.Name, FullName: modelMM.Name, FileIndexPlus1: fileCtx.Index + 1, Line: modelMM.Line, FileShortPath: fileCtx.ShortPath, MetricValues: make([]string, len(metricsTable.Headers)), IsProperty: isProperty, CoverageQuota: methodLineCoverageQuota}
			currentMethodValues := make(map[string]string)
			for _, metric := range modelMM.Metrics {
				valStr := "-"
				if metric.Value != nil {
					if valFloat, ok := metric.Value.(float64); ok {
						if math.IsNaN(valFloat) {
							valStr = "NaN"
						} else {
							if metric.Name == "Line coverage" || metric.Name == "Branch coverage" {
								valStr = fmt.Sprintf("%.1f%%", valFloat)
							} else {
								valStr = fmt.Sprintf("%.0f", valFloat)
							}
						}
					} else if valInt, okInt := metric.Value.(int); okInt {
						valStr = fmt.Sprintf("%d", valInt)
					}
				}
				currentMethodValues[metric.Name] = valStr
			}
			for i, header := range metricsTable.Headers {
				if val, ok := currentMethodValues[header.Name]; ok {
					row.MetricValues[i] = val
				} else {
					row.MetricValues[i] = "-"
				}
			}
			metricsTable.Rows = append(metricsTable.Rows, row)
		}
		classVM.MetricsTable = metricsTable
	}

	angularClassVMForJS := AngularClassViewModel{
		Name: classModel.DisplayName, CoveredLines: classModel.LinesCovered, UncoveredLines: classModel.LinesValid - classModel.LinesCovered,
		CoverableLines: classModel.LinesValid, TotalLines: classModel.TotalLines,
		CoveredMethods: classVM.CoveredMethods, FullyCoveredMethods: classVM.FullyCoveredMethods, TotalMethods: classVM.TotalMethods,
		HistoricCoverages: angularHistoricCoveragesForJS, LineCoverageHistory: lineCoverageHistoryForJS, BranchCoverageHistory: branchCoverageHistoryForJS,
		Metrics: classVM.Metrics,
	}
	if classModel.BranchesCovered != nil {
		angularClassVMForJS.CoveredBranches = *classModel.BranchesCovered
	}
	if classModel.BranchesValid != nil {
		angularClassVMForJS.TotalBranches = *classModel.BranchesValid
	}

	angularClassDetailForJS := AngularClassDetailViewModel{Class: angularClassVMForJS, Files: []AngularCodeFileViewModel{}}
	if classModel.Files != nil {
		for _, fileInClass := range classModel.Files {
			sourceFileLines, _ := filereader.ReadLinesInFile(fileInClass.Path)
			angularFileForJS := AngularCodeFileViewModel{Path: fileInClass.Path, CoveredLines: fileInClass.CoveredLines, CoverableLines: fileInClass.CoverableLines, TotalLines: fileInClass.TotalLines, Lines: []AngularLineAnalysisViewModel{}}
			coverageLinesMap := make(map[int]*model.Line)
			if fileInClass.Lines != nil {
				for i := range fileInClass.Lines {
					covLine := &fileInClass.Lines[i]
					coverageLinesMap[covLine.Number] = covLine
				}
			}
			for i, content := range sourceFileLines {
				actualLineNumber := i + 1
				modelCovLine, hasCoverageData := coverageLinesMap[actualLineNumber]
				var hits, coveredBranches, totalBranches int
				var lineVisitStatusString string
				if hasCoverageData {
					hits = modelCovLine.Hits
					coveredBranches = modelCovLine.CoveredBranches
					totalBranches = modelCovLine.TotalBranches
					lineVisitStatusString = lineVisitStatusToString(determineLineVisitStatus(hits, modelCovLine.IsBranchPoint, coveredBranches, totalBranches))
				} else {
					lineVisitStatusString = lineVisitStatusToString(lineVisitStatusNotCoverable)
				}
				angularFileForJS.Lines = append(angularFileForJS.Lines, AngularLineAnalysisViewModel{LineNumber: actualLineNumber, LineContent: content, Hits: hits, LineVisitStatus: lineVisitStatusString, CoveredBranches: coveredBranches, TotalBranches: totalBranches})
			}
			angularClassDetailForJS.Files = append(angularClassDetailForJS.Files, angularFileForJS)
		}
	}
	classDetailJSONBytes, err := json.Marshal(angularClassDetailForJS)
	if err != nil {
		return fmt.Errorf("failed to marshal Angular class detail JSON for %s: %w", classModel.DisplayName, err)
	}

	var appVersion string
	if b.ReportContext.ReportConfiguration != nil { // Check for nil before accessing
		// Assuming AppVersion is a constant or from a config, not directly on IReportConfiguration
		// For now, hardcoding a placeholder. In a real app, this would come from build info or similar.
		appVersion = "0.0.1" // Placeholder
	}

	templateData := ClassDetailData{
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
		ClassDetailJSON:                       template.JS(classDetailJSONBytes),
	}

	outputFilePath := filepath.Join(b.OutputDir, classReportFilename)
	fileWriter, err := os.Create(outputFilePath)
	if err != nil {
		return fmt.Errorf("failed to create class report file %s: %w", outputFilePath, err)
	}
	defer fileWriter.Close()

	if err := classDetailTpl.Execute(fileWriter, templateData); err != nil {
		return fmt.Errorf("failed to execute class detail template for report %s: %w", outputFilePath, err)
	}
	return nil
}

func (b *HtmlReportBuilder) getClassReportFilename(assemblyShortName, className string, existingFilenames map[string]struct{}) string {
	// Process className to get the short name
	processedClassName := className
	if lastDot := strings.LastIndex(className, "."); lastDot != -1 {
		processedClassName = className[lastDot+1:]
	}

	// Handle potential .js endings, similar to C# logic
	if strings.HasSuffix(strings.ToLower(processedClassName), ".js") { // Case-insensitive check for .js
		processedClassName = processedClassName[:len(processedClassName)-3]
	}

	baseName := assemblyShortName + "_" + processedClassName
	sanitizedName := sanitizeFilenameChars.ReplaceAllString(baseName, "_")

	// Truncate if too long (e.g., > 95 chars to leave room for counter and .html)
	maxLengthBase := 95
	if len(sanitizedName) > maxLengthBase {
		// Truncation logic: first 50 and last (maxLengthBase - 50) characters.
		// Ensure maxLengthBase is large enough for this logic to be meaningful.
		if maxLengthBase > 50 { // Default case: 50 + 45 = 95
			sanitizedName = sanitizedName[:50] + sanitizedName[len(sanitizedName)-(maxLengthBase-50):]
		} else { // If maxLengthBase is very small, just take the prefix
			sanitizedName = sanitizedName[:maxLengthBase]
		}
	}

	fileName := sanitizedName + ".html"
	counter := 1

	// Check for collisions using a normalized (lowercase) version of the filename.
	// The existingFilenames map stores these normalized names as keys.
	normalizedFileNameToCheck := strings.ToLower(fileName)

	_, exists := existingFilenames[normalizedFileNameToCheck]

	for exists {
		counter++
		fileName = fmt.Sprintf("%s%d.html", sanitizedName, counter)
		normalizedFileNameToCheck = strings.ToLower(fileName)
		_, exists = existingFilenames[normalizedFileNameToCheck]
	}

	existingFilenames[normalizedFileNameToCheck] = struct{}{} // Add the normalized version for future collision checks
	return fileName                                           // Return the actual generated filename (with original casing)
}

// parseAngularIndexHTML parses the Angular generated index.html to find asset filenames.
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
					cssFile = href // Expects only one main stylesheet
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

	// Basic validation: check if any of the crucial files were not found
	// Allow polyfills to be missing as some Angular configs might not emit it
	if cssFile == "" {
		err = fmt.Errorf("could not find Angular CSS file in %s", angularIndexHTMLPath)
	} else if runtimeJs == "" {
		err = fmt.Errorf("could not find Angular runtime.js file in %s", angularIndexHTMLPath)
	} else if mainJs == "" {
		err = fmt.Errorf("could not find Angular main.js file in %s", angularIndexHTMLPath)
	}

	return cssFile, runtimeJs, polyfillsJs, mainJs, err
}

// copyAngularAssets copies the built Angular application assets to the output directory.
func (b *HtmlReportBuilder) copyAngularAssets(outputDir string) error {
	// Ensure the source directory exists
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

	// Walk the Angular distribution directory
	return filepath.WalkDir(angularDistSourcePath, func(srcPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error accessing path %s during walk: %w", srcPath, err)
		}

		// Determine the relative path from the source root
		relPath, err := filepath.Rel(angularDistSourcePath, srcPath)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", srcPath, err)
		}

		// Construct the destination path
		dstPath := filepath.Join(outputDir, relPath)

		if d.IsDir() {
			// Create the directory in the destination with standard permissions
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dstPath, err)
			}
		} else {
			// It's a file, copy it
			srcFile, err := os.Open(srcPath)
			if err != nil {
				return fmt.Errorf("failed to open source file %s: %w", srcPath, err)
			}
			defer srcFile.Close()

			// Ensure the destination directory exists (it should, if WalkDir processes dirs first, but good to be safe)
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

			// Attempt to set file permissions to match source - best effort
			srcFileInfo, statErr := d.Info()
			if statErr == nil {
				os.Chmod(dstPath, srcFileInfo.Mode())
			}
		}
		return nil
	})
}

// copyStaticAssets copies static assets to the output directory.
func (b *HtmlReportBuilder) copyStaticAssets() error {
	filesToCopy := []string{
		"custom.css",
		"custom.js",
		"chartist.min.css",
		"chartist.min.js",
		"custom-azurepipelines.css",
		"custom-azurepipelines_adaptive.css",
		"custom-azurepipelines_dark.css",
		"custom_adaptive.css",
		"custom_bluered.css",
		"custom_dark.css",
	}

	for _, fileName := range filesToCopy {
		// Construct source path relative to the assetsDir constant
		srcPath := filepath.Join(assetsDir, fileName)
		// Construct destination path relative to the builder's OutputDir
		dstPath := filepath.Join(b.OutputDir, fileName)

		// Ensure destination directory for the asset exists
		dstDir := filepath.Dir(dstPath)
		if err := os.MkdirAll(dstDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory for asset %s: %w", dstPath, err)
		}

		srcFile, err := os.Open(srcPath)
		if err != nil {
			// Try to give a more specific error if the source asset itself is not found
			if os.IsNotExist(err) {
				return fmt.Errorf("source asset %s not found: %w", srcPath, err)
			}
			return fmt.Errorf("failed to open source asset %s: %w", srcPath, err)
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

	// Combine custom.css and custom_dark.css into report.css
	// Source paths are relative to assetsDir
	customCSSPath := filepath.Join(assetsDir, "custom.css")
	customDarkCSSPath := filepath.Join(assetsDir, "custom_dark.css")

	customCSSBytes, err := os.ReadFile(customCSSPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("custom.css not found at %s: %w", customCSSPath, err)
		}
		return fmt.Errorf("failed to read custom.css from %s: %w", customCSSPath, err)
	}

	customDarkCSSBytes, err := os.ReadFile(customDarkCSSPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("custom_dark.css not found at %s: %w", customDarkCSSPath, err)
		}
		return fmt.Errorf("failed to read custom_dark.css from %s: %w", customDarkCSSPath, err)
	}

	var combinedCSS []byte
	combinedCSS = append(combinedCSS, customCSSBytes...)
	combinedCSS = append(combinedCSS, []byte("\n")...) // Add a newline separator
	combinedCSS = append(combinedCSS, customDarkCSSBytes...)

	// Destination path for report.css is relative to builder's OutputDir
	reportCSSPath := filepath.Join(b.OutputDir, "report.css")
	if err := os.WriteFile(reportCSSPath, combinedCSS, 0644); err != nil {
		return fmt.Errorf("failed to write combined report.css to %s: %w", reportCSSPath, err)
	}

	return nil
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

func IfThenElse(condition bool, a, b float64) float64 { // Simplified for float64 for now
	if condition {
		return a
	}
	return b
}
