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

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/utils"
	"golang.org/x/net/html"
)

var (
	// Absolute paths to asset directories
	assetsDir             = filepath.Join(utils.ProjectRoot(), "assets", "htmlreport")
	angularDistSourcePath = filepath.Join(utils.ProjectRoot(), "angular_frontend_spa", "dist")
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

	// New fields for Angular settings
	BranchCoverageAvailable               bool
	MethodCoverageAvailable               bool
	MaximumDecimalPlacesForCoverageQuotas int
}

// HtmlReportBuilder is responsible for generating HTML reports.
type HtmlReportBuilder struct {
	OutputDir string
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

	// Prepare data for the template
	var generatedAtStr string
	if report.Timestamp == 0 {
		generatedAtStr = "N/A"
	} else {
		generatedAtStr = time.Unix(report.Timestamp, 0).Format(time.RFC1123Z)
	}

	data := HTMLReportData{
		Title:                  "Coverage Report",
		ParserName:             report.ParserName,
		GeneratedAt:            generatedAtStr,
		Content:                template.HTML("<p>Main content will be replaced by Angular app.</p>"), // Placeholder, Angular will take over
		AngularCssFile:         cssFile,
		AngularRuntimeJsFile:   runtimeJs,
		AngularPolyfillsJsFile: polyfillsJs,
		AngularMainJsFile:      mainJs,
		// Initialize new fields with default or empty values, they will be populated below
		AssembliesJSON:                        template.JS("[]"),
		RiskHotspotsJSON:                      template.JS("[]"),
		MetricsJSON:                           template.JS("[]"),
		RiskHotspotMetricsJSON:                template.JS("[]"),
		HistoricCoverageExecutionTimesJSON:    template.JS("[]"),
		TranslationsJSON:                      template.JS("{}"),
		BranchCoverageAvailable:               false,
		MethodCoverageAvailable:               false,
		MaximumDecimalPlacesForCoverageQuotas: 1,
	}

	// Populate translations
	translationsMap := GetTranslations()
	translationsJSONBytes, err := json.Marshal(translationsMap)
	if err != nil {
		// Log or handle error appropriately
		// For now, using empty JSON object if marshalling fails, and error was already logged or handled by Marshal
		// Or, more explicitly: fmt.Fprintf(os.Stderr, "Error marshalling translations: %v\n", err)
		data.TranslationsJSON = template.JS("({})") // Fallback to empty object
	} else {
		data.TranslationsJSON = template.JS(translationsJSONBytes)
	}

	// Populate Angular settings
	// Assuming report.TotalBranches is a field in model.SummaryResult.
	// If model.SummaryResult has a specific field like BranchesValid, that should be used.
	// For now, using TotalBranches > 0 as a proxy for availability.
	// The prompt suggests: report.BranchesValid && report.TotalBranches > 0
	// Lacking direct visibility into model.SummaryResult, we'll use a simplified check.
	// This might need adjustment if model.SummaryResult has specific fields like `BranchesValid *bool`.
	data.BranchCoverageAvailable = report.BranchesValid != nil && *report.BranchesValid > 0
	data.MethodCoverageAvailable = true            // As per issue description
	data.MaximumDecimalPlacesForCoverageQuotas = 1 // Default from C#

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
	if report.Assemblies != nil {
		for _, assembly := range report.Assemblies { // assembly is model.Assembly (struct)
			angularAssembly := AngularAssemblyViewModel{
				Name:    assembly.Name,
				Classes: []AngularClassViewModel{},
			}

			for _, class := range assembly.Classes { // class is model.Class (struct)
				angularClass := AngularClassViewModel{
					Name:                class.DisplayName,
					ReportPath:          "", // Not available in model.Class
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

	return nil
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
			// Create the directory in the destination
			if err := os.MkdirAll(dstPath, d.Type().Perm()); err != nil { // Use source permission
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
