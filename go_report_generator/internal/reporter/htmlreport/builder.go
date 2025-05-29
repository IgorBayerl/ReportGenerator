package htmlreport

import (
	"encoding/json" // Added for marshalling translations
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"regexp" // Added for sanitizeFilenameChars

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/utils"
	"golang.org/x/net/html"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/filereader" // Added for reading source file lines
)

var (
	// Absolute paths to asset directories
	assetsDir             = filepath.Join(utils.ProjectRoot(), "assets", "htmlreport")
	angularDistSourcePath = filepath.Join(utils.ProjectRoot(), "angular_frontend_spa", "dist")

	// sanitizeFilenameChars replaces or removes characters that are invalid for filenames.
	sanitizeFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
)

// HTMLReportData holds all data for the base HTML template.
type HTMLReportData struct {
	Title                  string
	ParserName             string
	GeneratedAt            string
	Content                template.HTML // Keep for now, Angular might replace it
	AngularCssFile         string
	AngularRuntimeJsFile   string
	AngularPolyfillsJsFile string
	AngularMainJsFile      string

	// New fields for Angular data
	AssembliesJSON                     template.JS // Use template.JS for safety
	RiskHotspotsJSON                   template.JS
	MetricsJSON                        template.JS
	RiskHotspotMetricsJSON             template.JS
	HistoricCoverageExecutionTimesJSON template.JS
	TranslationsJSON                   template.JS
	ClassDetailJSON                    template.JS `json:"-"` // For class detail page specific data

	// New fields for Angular settings
	BranchCoverageAvailable               bool
	MethodCoverageAvailable               bool
	MaximumDecimalPlacesForCoverageQuotas int
}

// HtmlReportBuilder is responsible for generating HTML reports.
type HtmlReportBuilder struct {
	OutputDir string

	// Fields to store settings and paths for use in class detail page generation
	angularCssFile         string
	angularRuntimeJsFile   string
	angularPolyfillsJsFile string
	angularMainJsFile      string

	branchCoverageAvailable               bool
	methodCoverageAvailable               bool
	maximumDecimalPlacesForCoverageQuotas int
	parserName                            string
	reportTimestamp                       int64
	reportTitle                           string
	translations                          map[string]string // Keep translations map
	onlySummary                           bool

	// Pre-marshaled global data
	assembliesJSON                     template.JS
	riskHotspotsJSON                   template.JS
	metricsJSON                        template.JS
	riskHotspotMetricsJSON             template.JS
	historicCoverageExecutionTimesJSON template.JS
	translationsJSON                   template.JS // For marshaled translations
}

// NewHtmlReportBuilder creates a new HtmlReportBuilder.
func NewHtmlReportBuilder(outputDir string) *HtmlReportBuilder {
	return &HtmlReportBuilder{
		OutputDir: outputDir,
	}
}

// ReportType returns the type of report this builder creates.
func (b *HtmlReportBuilder) ReportType() string {
	return "Html"
}

// CreateReport generates the HTML report based on the SummaryResult.
// For now, it's a placeholder and will return nil.
func (b *HtmlReportBuilder) CreateReport(report *model.SummaryResult) error {
	// Ensure output directory exists
	if err := os.MkdirAll(b.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", b.OutputDir, err)
	}

	// Copy static assets
	if err := b.copyStaticAssets(); err != nil {
		return fmt.Errorf("failed to copy static assets: %w", err)
	}

	// Copy Angular assets
	if err := b.copyAngularAssets(b.OutputDir); err != nil {
		return fmt.Errorf("failed to copy angular assets: %w", err)
	}

	// Parse Angular index.html to get asset filenames
	angularIndexHTMLPath := filepath.Join(angularDistSourcePath, "index.html")
	cssFile, runtimeJs, polyfillsJs, mainJs, err := b.parseAngularIndexHTML(angularIndexHTMLPath)
	if err != nil {
		return fmt.Errorf("failed to parse Angular index.html: %w", err)
	}
	if cssFile == "" || runtimeJs == "" || polyfillsJs == "" || mainJs == "" {
		return fmt.Errorf("missing one or more critical Angular assets from index.html (css: %s, runtime: %s, polyfills: %s, main: %s)", cssFile, runtimeJs, polyfillsJs, mainJs)
	}

	// Store parsed asset filenames and report settings on the builder instance
	b.angularCssFile = cssFile
	b.angularRuntimeJsFile = runtimeJs
	b.angularPolyfillsJsFile = polyfillsJs
	b.angularMainJsFile = mainJs
	b.reportTitle = "Coverage Report" // Default title, can be made configurable
	b.parserName = report.ParserName
	b.reportTimestamp = report.Timestamp
	b.branchCoverageAvailable = report.BranchesValid != nil && *report.BranchesValid > 0
	b.methodCoverageAvailable = true            // As per issue description
	b.maximumDecimalPlacesForCoverageQuotas = 1 // Default from C#

	// Prepare data for the main index.html template
	var generatedAtStr string
	if b.reportTimestamp == 0 {
		generatedAtStr = "N/A"
	} else {
		generatedAtStr = time.Unix(b.reportTimestamp, 0).Format(time.RFC1123Z)
	}

	data := HTMLReportData{
		Title:                                 b.reportTitle,
		ParserName:                            b.parserName,
		GeneratedAt:                           generatedAtStr,
		Content:                               template.HTML("<p>Main content will be replaced by Angular app.</p>"),
		AngularCssFile:                        b.angularCssFile,
		AngularRuntimeJsFile:                  b.angularRuntimeJsFile,
		AngularPolyfillsJsFile:                b.angularPolyfillsJsFile,
		AngularMainJsFile:                     b.angularMainJsFile,
		AssembliesJSON:                        template.JS("[]"), // Will be populated below
		RiskHotspotsJSON:                      template.JS("[]"), // Will be populated below
		MetricsJSON:                           template.JS("[]"), // Will be populated below
		RiskHotspotMetricsJSON:                template.JS("[]"), // Will be populated below
		HistoricCoverageExecutionTimesJSON:    template.JS("[]"), // Will be populated below
		TranslationsJSON:                      template.JS("{}"), // Will be populated below
		BranchCoverageAvailable:               b.branchCoverageAvailable,
		MethodCoverageAvailable:               b.methodCoverageAvailable,
		MaximumDecimalPlacesForCoverageQuotas: b.maximumDecimalPlacesForCoverageQuotas,
	}

	// Populate translations
	b.translations = GetTranslations()
	translationsJSONBytes, err := json.Marshal(b.translations)
	if err != nil {
		data.TranslationsJSON = template.JS("({})") 
	} else {
		data.TranslationsJSON = template.JS(translationsJSONBytes)
	}
	b.translationsJSON = data.TranslationsJSON // Store for class pages

	// Populate AngularMetricViewModel slice (for window.metrics)
	// These are the overall metrics available in the report.
	// For now, we'll create a placeholder or extract from a predefined list if model doesn't directly provide this.
	// This part will need refinement once we know how overall available metrics are determined from 'report *model.SummaryResult'.
	// Based on C# HtmlRenderer.CustomSummary(), these are usually fixed metrics.
	availableMetrics := []AngularMetricViewModel{
		{Name: "NPath complexity", Abbreviation: "npath", ExplanationURL: "https://modess.io/npath-complexity-cyclomatic-complexity-explained/"},
		{Name: "CrapScore", Abbreviation: "crap", ExplanationURL: "https://testing.googleblog.com/2011/02/this-code-is-crap.html"},
		// Add other common metrics as needed, e.g., from C# ReportResources or common usage.
		// {Name: "Cyclomatic Complexity", Abbreviation: "cyclomatic", ExplanationURL: "some_url"}, // Example
	}
	// In a more complete version, this list would be dynamically populated based on report.AvailableMetrics or similar
	metricsJSONBytes, err := json.Marshal(availableMetrics)
	if err != nil {
		data.MetricsJSON = template.JS("([])")
	} else {
		data.MetricsJSON = template.JS(metricsJSONBytes)
	}

	// Populate AngularRiskHotspotMetricHeaderViewModel slice (for window.riskHotspotMetrics)
	// These are the headers for the risk hotspot table.
	riskHotspotMetricHeaders := []AngularRiskHotspotMetricHeaderViewModel{
		{Name: "Cyclomatic complexity", Abbreviation: "cyclomatic", ExplanationURL: "https://www.ndepend.com/docs/code-metrics#CC"},
		{Name: "CrapScore", Abbreviation: "crap", ExplanationURL: "https://testing.googleblog.com/2011/02/this-code-is-crap.html"},
		{Name: "NPath complexity", Abbreviation: "npath", ExplanationURL: "https://modess.io/npath-complexity-cyclomatic-complexity-explained/"},
		// {Name: "Coverage", Abbreviation: "coverage", ExplanationURL: ""}, // Example, if needed
		// {Name: "Something else", Abbreviation: "else", ExplanationURL: ""}, // Example
	}
	riskHotspotMetricsJSONBytes, err := json.Marshal(riskHotspotMetricHeaders)
	if err != nil {
		data.RiskHotspotMetricsJSON = template.JS("([])")
	} else {
		data.RiskHotspotMetricsJSON = template.JS(riskHotspotMetricsJSONBytes)
	}

	// Populate HistoricCoverageExecutionTimes
	// Based on C# HtmlRenderer.CustomSummary(), this filters and formats these.
	var executionTimes []string
	uniqueExecutionTimestamps := make(map[int64]bool)

	if report.Assemblies != nil {
		for _, assembly := range report.Assemblies {
			for _, class := range assembly.Classes {
				for _, hist := range class.HistoricCoverages {
					// hist.ExecutionTime is int64
					if _, exists := uniqueExecutionTimestamps[hist.ExecutionTime]; !exists {
						uniqueExecutionTimestamps[hist.ExecutionTime] = true
					}
				}
			}
		}
	}

	// Sort unique timestamps if order matters - prompt doesn't specify, so skipping for now
	// but in a real scenario, sorting timestamps chronologically would be good.
	// For example, collect keys from map, sort them, then format.

	for ts := range uniqueExecutionTimestamps {
		executionTimes = append(executionTimes, time.Unix(ts, 0).Format("2006-01-02 15:04:05"))
	}
	// If a specific order is required for executionTimes (e.g., chronological),
	// they should be sorted here before marshalling. For now, map iteration order is used.

	historicExecTimesJSONBytes, err := json.Marshal(executionTimes)
	if err != nil {
		data.HistoricCoverageExecutionTimesJSON = template.JS("([])") // Fallback to empty array
	} else {
		data.HistoricCoverageExecutionTimesJSON = template.JS(historicExecTimesJSONBytes)
	}

	// Populate AngularAssemblyViewModel slice (for window.assemblies)
	var angularAssemblies []AngularAssemblyViewModel
	existingFilenames := make(map[string]struct{}) // Initialize existingFilenames map

	if report.Assemblies != nil {
		for _, assembly := range report.Assemblies { // assembly is model.Assembly (struct)
			// Use assembly.Name as model.Assembly does not have ShortName.
			assemblyShortName := assembly.Name

			angularAssembly := AngularAssemblyViewModel{
				Name:    assembly.Name, // This is for display in the Angular app.
				Classes: []AngularClassViewModel{},
			}

			for _, class := range assembly.Classes { // class is model.Class (struct)
				// Generate filename using class.Name (not class.DisplayName)
				classReportFilename := b.getClassReportFilename(assemblyShortName, class.Name, existingFilenames)

				angularClass := AngularClassViewModel{
					Name:                class.DisplayName,
					ReportPath:          classReportFilename, // Populate ReportPath
					CoveredLines:        class.LinesCovered,
					CoverableLines:      class.LinesValid,
					UncoveredLines:      class.LinesValid - class.LinesCovered,
					TotalLines:          class.TotalLines,
					CoveredMethods:      0, // Not directly available, set to 0
					FullyCoveredMethods: 0, // Not directly available, set to 0
					TotalMethods:        0, // Not directly available, set to 0
					Metrics:             make(map[string]float64),
					HistoricCoverages:   []AngularHistoricCoverageViewModel{},
				}

				if class.BranchesCovered != nil {
					angularClass.CoveredBranches = *class.BranchesCovered
				}
				if class.BranchesValid != nil {
					angularClass.TotalBranches = *class.BranchesValid
				}

				// Populate HistoricCoverages for the class
				if class.HistoricCoverages != nil {
					for _, hist := range class.HistoricCoverages { // hist is model.HistoricCoverage
						angularHist := AngularHistoricCoverageViewModel{
							ExecutionTime:   time.Unix(hist.ExecutionTime, 0).Format("2006-01-02"),
							CoveredLines:    hist.CoveredLines,
							CoverableLines:  hist.CoverableLines,
							TotalLines:      hist.TotalLines, // Assuming this field exists on model.HistoricCoverage
							CoveredBranches: hist.CoveredBranches,
							TotalBranches:   hist.TotalBranches,
						}
						if hist.CoverableLines > 0 {
							angularHist.LineCoverageQuota = float64(hist.CoveredLines) / float64(hist.CoverableLines) * 100
						}
						if hist.TotalBranches > 0 {
							angularHist.BranchCoverageQuota = float64(hist.CoveredBranches) / float64(hist.TotalBranches) * 100
						}
						angularClass.HistoricCoverages = append(angularClass.HistoricCoverages, angularHist)

						// Populate history arrays (lch, bch, etc.)
						// Ensure we only append valid quota values, e.g. > 0 or based on your logic for "valid" history points
						if angularHist.LineCoverageQuota >= 0 { // Assuming 0 is a valid quota to record
							angularClass.LineCoverageHistory = append(angularClass.LineCoverageHistory, angularHist.LineCoverageQuota)
						}
						if angularHist.BranchCoverageQuota >= 0 { // Assuming 0 is a valid quota to record
							angularClass.BranchCoverageHistory = append(angularClass.BranchCoverageHistory, angularHist.BranchCoverageQuota)
						}
						// MethodCoverageHistory and FullMethodCoverageHistory would require method-level historic data.
					}
				}

				// Populate Metrics for the class by aggregating from methods
				tempMetrics := make(map[string][]float64) // metric Name to list of values
				if class.Methods != nil {
					for _, method := range class.Methods { // method is model.Method
						if method.MethodMetrics != nil {
							for _, methodMetric := range method.MethodMetrics { // methodMetric is model.MethodMetric
								if methodMetric.Metrics != nil {
									for _, metric := range methodMetric.Metrics { // metric is model.Metric
										if metric.Name == "" { // Use Name as key
											continue
										}
										// Value is interface{}, attempt type assertion to float64
										if valFloat, ok := metric.Value.(float64); ok {
											tempMetrics[metric.Name] = append(tempMetrics[metric.Name], valFloat)
										} else {
											// Optionally log or handle metrics with unexpected types
											// fmt.Fprintf(os.Stderr, "Metric %s has non-float64 value: %T\n", metric.Name, metric.Value)
										}
									}
								}
							}
						}
					}
				}
				for name, values := range tempMetrics { // Use name instead of abbr
					if len(values) > 0 {
						var sum float64
						for _, v := range values {
							sum += v
						}
						angularClass.Metrics[name] = sum // Use name as key
					}
				}

				angularAssembly.Classes = append(angularAssembly.Classes, angularClass)
			}
			angularAssemblies = append(angularAssemblies, angularAssembly)
		}
	}

	assembliesJSONBytes, err := json.Marshal(angularAssemblies)
	if err != nil {
		data.AssembliesJSON = template.JS("([])")
	} else {
		data.AssembliesJSON = template.JS(assembliesJSONBytes)
	}

	// Populate AngularRiskHotspotViewModel slice (for window.riskHotspots)
	// This requires a RiskHotspotAnalysisResult similar to C#'s.
	// For now, creating placeholder/empty data or preparing a basic structure.
	// The actual C# logic iterates `reportGenerator.RiskHotspotAnalysisResult.RiskHotspots`.
	// We need to see if `report *model.SummaryResult` has something similar,
	// or if we need to compute hotspots from scratch based on metrics in classes/methods.

	var angularRiskHotspots []AngularRiskHotspotViewModel
	// Placeholder: If you have a way to identify methods with high complexity metrics,
	// you could iterate them here.
	// Example: Iterate through assemblies, classes, files, methodmetrics
	// For now, let's assume report.RiskHotspots exists or is empty.
	// If report.RiskHotspots (or a similar field) is not available on model.SummaryResult,
	// this will just result in an empty list, which is acceptable for this step.

	/*
		// Example of how it might look if data was available on report (conceptual)
		if report.RiskHotspots != nil { // Assuming report.RiskHotspots is a slice of model.RiskHotspot
			for _, rs := range report.RiskHotspots {
				if rs == nil { continue }
				angularRs := AngularRiskHotspotViewModel{
					Assembly:    rs.AssemblyName,
					Class:       rs.ClassName,
					ReportPath:  rs.ReportPath, // Path to the class report HTML file
					MethodName:  rs.MethodName,
					MethodShortName: rs.MethodShortName, // Or derive from MethodName
					FileIndex:   0, // This was related to a specific C# structure, may not map directly
					Line:        rs.Line, // Starting line of the method
					Metrics:     []AngularRiskHotspotStatusMetricViewModel{},
				}

				// Example: Populate metrics for the hotspot
				// This would depend on what metrics are stored with model.RiskHotspot
				// For instance, if rs.ComplexityMetric > rs.ComplexityThreshold
				if rs.ComplexityMetricValue > 10 { // Example
					angularRs.Metrics = append(angularRs.Metrics, AngularRiskHotspotStatusMetricViewModel{
						Value:    fmt.Sprintf("%.0f", rs.ComplexityMetricValue),
						Exceeded: true,
					})
				}
				// Add other relevant metrics (e.g., CrapScore, NPath) if available on model.RiskHotspot

				angularRiskHotspots = append(angularRiskHotspots, angularRs)
			}
		}
	*/
	// Since the structure of risk hotspots in the Go model is not yet defined
	// or provided in report.SummaryResult, we will pass an empty array for now.
	// This matches the "placeholder/empty data" requirement.

	riskHotspotsJSONBytes, err := json.Marshal(angularRiskHotspots) // angularRiskHotspots is currently an empty slice
	if err != nil {
		data.RiskHotspotsJSON = template.JS("([])") // Corrected to use 'data'
	} else {
		data.RiskHotspotsJSON = template.JS(riskHotspotsJSONBytes) // Corrected to use 'data'
	}

	// Create index.html file in the output directory
	outputIndexPath := filepath.Join(b.OutputDir, "index.html")
	file, err := os.Create(outputIndexPath)
	if err != nil {
		return fmt.Errorf("failed to create index.html at %s: %w", outputIndexPath, err)
	}
	defer file.Close()

	// Execute the base template with the data
	// baseTpl is parsed in templates.go and available in this package
	if err := baseTpl.Execute(file, data); err != nil {
		return fmt.Errorf("failed to execute base template into %s: %w", outputIndexPath, err)
	}
	file.Close() // Close index.html file before starting detail pages

	// --- START: New loop for class detail pages (Step 6) ---
	if !b.onlySummary {
		// 'angularAssemblies' is the slice of AngularAssemblyViewModel populated earlier for the summary.
		// report.Assemblies is the original model data.
		for _, assemblyModel := range report.Assemblies {
			for _, classModel := range assemblyModel.Classes { // classModel is of type model.Class
				var classReportFilename string
				foundFilename := false

				// Find the classReportFilename for classModel from angularAssemblies
				for _, asmView := range angularAssemblies { // Use 'angularAssemblies' which is in scope
					if asmView.Name == assemblyModel.Name {
						for _, classView := range asmView.Classes {
							if classView.Name == classModel.DisplayName {
								classReportFilename = classView.ReportPath
								foundFilename = true
								break
							}
						}
					}
					if foundFilename {
						break
					}
				}

				if !foundFilename {
					return fmt.Errorf("could not find pre-generated report filename for class: '%s' in assembly '%s'", classModel.DisplayName, assemblyModel.Name)
				}

				if classReportFilename == "" {
					fmt.Fprintf(os.Stderr, "Warning: Empty report filename for class '%s' in assembly '%s', skipping detail page generation.\n", classModel.DisplayName, assemblyModel.Name)
					continue
				}

				// Call generateClassDetailHTML, passing &classModel (pointer)
				err := b.generateClassDetailHTML(&classModel, angularAssemblies, classReportFilename, b.translations) // Use 'angularAssemblies'
				if err != nil {
					return fmt.Errorf("failed to generate detail page for class '%s' (file: %s): %w", classModel.DisplayName, classReportFilename, err)
				}
			}
		}
	}
	// --- END: New loop for class detail pages ---

	return nil
}

// TODO: Verify model.LineVisitStatus enum values once its definition is found.
// Assuming: 0 for NotCoverable, 1 for Covered, 2 for NotCovered, 3 for PartiallyCovered.
const (
	lineVisitStatusNotCoverable     = 0 // Placeholder
	lineVisitStatusCovered          = 1 // Placeholder
	lineVisitStatusNotCovered       = 2 // Placeholder
	lineVisitStatusPartiallyCovered = 3 // Placeholder
)

// determineLineVisitStatus determines the line visit status based on coverage data.
func determineLineVisitStatus(hits int, isBranchPoint bool, coveredBranches int, totalBranches int) int {
	if hits > 0 {
		if isBranchPoint {
			if totalBranches == 0 {
				return lineVisitStatusCovered // No branches to cover, so considered covered
			} else if coveredBranches == totalBranches {
				return lineVisitStatusCovered
			} else if coveredBranches > 0 {
				return lineVisitStatusPartiallyCovered
			} else { // coveredBranches == 0
				return lineVisitStatusNotCovered
			}
		} else { // Not a branch point
			return lineVisitStatusCovered
		}
	} else { // hits == 0
		return lineVisitStatusNotCovered
	}
}

func lineVisitStatusToString(status int) string { // Assuming status is int for now
	switch status {
	case lineVisitStatusCovered:
		return "covered"
	case lineVisitStatusNotCovered:
		return "uncovered"
	case lineVisitStatusPartiallyCovered:
		return "partiallycovered"
	default: // lineVisitStatusNotCoverable and any other undefined states
		return "notcoverable"
	}
}

// generateClassDetailHTML generates the HTML page for a single class.
func (b *HtmlReportBuilder) generateClassDetailHTML(classModel *model.Class, allAssembliesForAngular []AngularAssemblyViewModel, classReportFilename string, translations map[string]string) error {
	// 1. Prepare AngularClassViewModel for the current class (currentClassVM)
	currentClassVM := AngularClassViewModel{
		Name:                classModel.DisplayName,
		ReportPath:          "", // Current page
		CoveredLines:        classModel.LinesCovered,
		UncoveredLines:      classModel.LinesValid - classModel.LinesCovered,
		CoverableLines:      classModel.LinesValid,
		TotalLines:          classModel.TotalLines,
		CoveredMethods:      0, // Placeholder, requires method-level analysis within classModel
		FullyCoveredMethods: 0, // Placeholder
		TotalMethods:        0, // Placeholder
		Metrics:             make(map[string]float64),
		HistoricCoverages:   []AngularHistoricCoverageViewModel{},
		// LineCoverageHistory, BranchCoverageHistory, etc. will be populated from HistoricCoverages
	}
	if classModel.BranchesCovered != nil {
		currentClassVM.CoveredBranches = *classModel.BranchesCovered
	}
	if classModel.BranchesValid != nil {
		currentClassVM.TotalBranches = *classModel.BranchesValid
	}

	// Populate HistoricCoverages for the class (similar to summary page logic for a class)
	if classModel.HistoricCoverages != nil {
		for _, hist := range classModel.HistoricCoverages {
			angularHist := AngularHistoricCoverageViewModel{
				ExecutionTime:   time.Unix(hist.ExecutionTime, 0).Format("2006-01-02"),
				CoveredLines:    hist.CoveredLines,
				CoverableLines:  hist.CoverableLines,
				TotalLines:      hist.TotalLines,
				CoveredBranches: hist.CoveredBranches,
				TotalBranches:   hist.TotalBranches,
			}
			if hist.CoverableLines > 0 {
				angularHist.LineCoverageQuota = float64(hist.CoveredLines) / float64(hist.CoverableLines) * 100
			}
			if hist.TotalBranches > 0 {
				angularHist.BranchCoverageQuota = float64(hist.CoveredBranches) / float64(hist.TotalBranches) * 100
			}
			currentClassVM.HistoricCoverages = append(currentClassVM.HistoricCoverages, angularHist)
			if angularHist.LineCoverageQuota >= 0 {
				currentClassVM.LineCoverageHistory = append(currentClassVM.LineCoverageHistory, angularHist.LineCoverageQuota)
			}
			if angularHist.BranchCoverageQuota >= 0 {
				currentClassVM.BranchCoverageHistory = append(currentClassVM.BranchCoverageHistory, angularHist.BranchCoverageQuota)
			}
		}
	}

	// Populate Metrics for the class (similar to summary page logic for a class)
	tempMetrics := make(map[string][]float64)
	if classModel.Methods != nil {
		for _, method := range classModel.Methods {
			if method.MethodMetrics != nil {
				for _, methodMetric := range method.MethodMetrics {
					if methodMetric.Metrics != nil {
						for _, metric := range methodMetric.Metrics {
							if metric.Name == "" {
								continue
							}
							if valFloat, ok := metric.Value.(float64); ok {
								tempMetrics[metric.Name] = append(tempMetrics[metric.Name], valFloat)
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
			currentClassVM.Metrics[name] = sum
		}
	}

	// 2. Prepare Files (slice of AngularCodeFileViewModel)
	var angularCodeFiles []AngularCodeFileViewModel
	if classModel.Files != nil {
		for _, fileInClass := range classModel.Files { // fileInClass is model.CodeFile
			// Read source file lines
			sourceFileLines, err := filereader.ReadLinesInFile(fileInClass.Path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not read source file %s for class %s for line content: %v\n", fileInClass.Path, classModel.DisplayName, err)
				sourceFileLines = []string{} // Use empty slice if file read fails
			}

			angularFile := AngularCodeFileViewModel{
				Path:           fileInClass.Path,
				CoveredLines:   fileInClass.CoveredLines,   // Specific to this file's part in the class
				CoverableLines: fileInClass.CoverableLines, // Specific to this file's part
				TotalLines:     fileInClass.TotalLines,     // Total physical lines in this source file
				Lines:          []AngularLineAnalysisViewModel{},
				MethodMetrics:  []AngularMethodMetricViewModel{}, // Placeholder as model.CodeFile doesn't have MethodMetrics
				CodeElements:   []AngularCodeElementViewModel{},  // Placeholder as model.CodeFile doesn't have CodeElements
			}

			// Create a map for quick lookup of coverage data
			coverageLinesMap := make(map[int]*model.Line)
			if fileInClass.Lines != nil {
				for i := range fileInClass.Lines { // Iterate by index to get pointer
					covLine := &fileInClass.Lines[i]
					coverageLinesMap[covLine.Number] = covLine
				}
			}

			var processedLines []AngularLineAnalysisViewModel
			for i, content := range sourceFileLines {
				actualLineNumber := i + 1
				currentLineContent := content

				var hits int
				var isBranchPoint bool
				var coveredBranches int
				var totalBranches int
				var lineVisitStatusString string

				modelCovLine, hasCoverageData := coverageLinesMap[actualLineNumber]

				if hasCoverageData {
					hits = modelCovLine.Hits
					isBranchPoint = modelCovLine.IsBranchPoint
					coveredBranches = modelCovLine.CoveredBranches
					totalBranches = modelCovLine.TotalBranches
					status := determineLineVisitStatus(hits, isBranchPoint, coveredBranches, totalBranches)
					lineVisitStatusString = lineVisitStatusToString(status)
				} else {
					hits = 0
					isBranchPoint = false
					coveredBranches = 0
					totalBranches = 0
					lineVisitStatusString = "notcoverable" // Explicitly mark as notcoverable
				}

				angularLine := AngularLineAnalysisViewModel{
					LineNumber:      actualLineNumber,
					LineContent:     currentLineContent,
					Hits:            hits,
					LineVisitStatus: lineVisitStatusString,
					CoveredBranches: coveredBranches,
					TotalBranches:   totalBranches,
				}
				processedLines = append(processedLines, angularLine)
			}
			angularFile.Lines = processedLines
			angularCodeFiles = append(angularCodeFiles, angularFile)
		}
	}

	// 3. Assemble AngularClassDetailViewModel
	classDetailVM := AngularClassDetailViewModel{
		Class: currentClassVM,
		Files: angularCodeFiles,
	}

	// 4. Marshal to JSON
	classDetailJSONBytes, err := json.Marshal(classDetailVM)
	if err != nil {
		return fmt.Errorf("failed to marshal class detail view model for %s: %w", classModel.DisplayName, err)
	}
	assembliesJSONBytes, err := json.Marshal(allAssembliesForAngular)
	if err != nil {
		return fmt.Errorf("failed to marshal all assemblies for %s: %w", classModel.DisplayName, err)
	}
	translationsJSONBytes, err := json.Marshal(translations)
	if err != nil {
		return fmt.Errorf("failed to marshal translations for %s: %w", classModel.DisplayName, err)
	}

	// 5. Prepare HTMLReportData
	generatedAtStr := "N/A"
	if b.reportTimestamp != 0 {
		generatedAtStr = time.Unix(b.reportTimestamp, 0).Format(time.RFC1123Z)
	}

	htmlData := HTMLReportData{
		Title:                                 classModel.DisplayName, // Page title is class name
		ParserName:                            b.parserName,
		GeneratedAt:                           generatedAtStr,
		AngularCssFile:                        b.angularCssFile,
		AngularRuntimeJsFile:                  b.angularRuntimeJsFile,
		AngularPolyfillsJsFile:                b.angularPolyfillsJsFile,
		AngularMainJsFile:                     b.angularMainJsFile,
		AssembliesJSON:                        template.JS(assembliesJSONBytes),
		TranslationsJSON:                      template.JS(translationsJSONBytes),
		ClassDetailJSON:                       template.JS(classDetailJSONBytes),
		BranchCoverageAvailable:               b.branchCoverageAvailable,
		MethodCoverageAvailable:               b.methodCoverageAvailable,
		MaximumDecimalPlacesForCoverageQuotas: b.maximumDecimalPlacesForCoverageQuotas,
		// RiskHotspotsJSON, MetricsJSON, RiskHotspotMetricsJSON, HistoricCoverageExecutionTimesJSON are not directly needed for class detail page's primary data
		// Content is also not needed as Angular takes over.
	}

	// 6. Render HTML
	outputFilePath := filepath.Join(b.OutputDir, classReportFilename)
	fileWriter, err := os.Create(outputFilePath)
	if err != nil {
		return fmt.Errorf("failed to create class report file %s: %w", outputFilePath, err)
	}
	defer fileWriter.Close()

	// Assuming baseTpl is parsed and available (e.g., in templates.go)
	// Use Execute to render the main template associated with baseTpl (named "base").
	if err := baseTpl.Execute(fileWriter, htmlData); err != nil {
		return fmt.Errorf("failed to execute template for class report %s: %w", outputFilePath, err)
	}

	return nil
}

// getClassReportFilename generates a unique filename for a class report.
// It mirrors the logic of C#'s HtmlRenderer.GetClassReportFilename().
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
	if cssFile == "" {
		err = fmt.Errorf("could not find Angular CSS file in %s", angularIndexHTMLPath)
	} else if runtimeJs == "" {
		err = fmt.Errorf("could not find Angular runtime.js file in %s", angularIndexHTMLPath)
	} else if polyfillsJs == "" {
		err = fmt.Errorf("could not find Angular polyfills.js file in %s", angularIndexHTMLPath)
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
			return fmt.Errorf("angular source directory %s does not exist: %w", angularDistSourcePath, err)
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
// Note: outputDir argument was removed as b.OutputDir is used directly.
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
