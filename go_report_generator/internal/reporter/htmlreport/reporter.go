package htmlreport

import (
	"bytes" // For text/template
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template" // For safer JS generation

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reporter"
)

// AngularAssembly matches the structure expected by the frontend's assembly.class.ts
type AngularAssembly struct {
	Name    string         `json:"name"`    // Corresponds to 'name' in assembly.class.ts
	Classes []AngularClass `json:"classes"` // Corresponds to 'classes' in assembly.class.ts
}

// AngularClass matches the structure expected by the frontend's class.class.ts
type AngularClass struct {
	Name string `json:"name"` // 'name' in class.class.ts
	Rp   string `json:"rp"`   // 'rp' (report path) in class.class.ts - placeholder for now

	Cl  int `json:"cl"`  // Covered Lines
	Ucl int `json:"ucl"` // Uncovered Lines
	Cal int `json:"cal"` // Coverable Lines
	Tl  int `json:"tl"`  // Total Lines

	Cb int `json:"cb"` // Covered Branches
	Tb int `json:"tb"` // Total Branches

	Cm  int `json:"cm"`  // Covered Methods
	Fcm int `json:"fcm"` // Fully Covered Methods
	Tm  int `json:"tm"`  // Total Methods
}

// transformSummaryResultToAngularData transforms model.SummaryResult to []AngularAssembly.
func transformSummaryResultToAngularData(summary *model.SummaryResult) []AngularAssembly {
	if summary == nil {
		return nil
	}

	angularAssemblies := make([]AngularAssembly, len(summary.Assemblies))

	for i, modelAssembly := range summary.Assemblies {
		angularClasses := make([]AngularClass, len(modelAssembly.Classes))
		for j, modelClass := range modelAssembly.Classes {
			var coveredBranches, totalBranches int
			if modelClass.BranchesCovered != nil {
				coveredBranches = *modelClass.BranchesCovered
			}
			if modelClass.BranchesValid != nil {
				totalBranches = *modelClass.BranchesValid
			}

			var coveredMethods, fullyCoveredMethods int
			for _, method := range modelClass.Methods {
				if method.LineRate > 0 {
					coveredMethods++
				}
				if method.LineRate == 1.0 {
					fullyCoveredMethods++
				}
			}

			className := modelClass.Name
			if modelClass.DisplayName != "" {
				className = modelClass.DisplayName
			}

			angularClasses[j] = AngularClass{
				Name: className,
				Rp:   fmt.Sprintf("%s_report.html", strings.ReplaceAll(className, "/", "_")), // Placeholder for report path
				Cl:   modelClass.LinesCovered,
				Cal:  modelClass.LinesValid,
				Ucl:  modelClass.LinesValid - modelClass.LinesCovered,
				Tl:   modelClass.TotalLines,
				Cb:   coveredBranches,
				Tb:   totalBranches,
				Tm:   len(modelClass.Methods),
				Cm:   coveredMethods,
				Fcm:  fullyCoveredMethods,
			}
		}

		angularAssemblies[i] = AngularAssembly{
			Name:    modelAssembly.Name,
			Classes: angularClasses,
		}
	}
	return angularAssemblies
}

// HTMLReport generates an HTML report.
type HTMLReport struct {
	outputDir string
}

// NewHTMLReport creates a new HTMLReport.
func NewHTMLReport(outputDir string) reporter.ReportBuilder {
	return &HTMLReport{outputDir: outputDir}
}

// ReportType returns the type of report this builder generates.
func (r *HTMLReport) ReportType() string {
	return "HTML"
}

const reportDataJSTemplate = `// Built by Go
window.historicCoverageExecutionTimes = JSON.parse({{.HistoricCoverageExecutionTimesJSON}});
window.branchCoverageAvailable = {{.BranchCoverageAvailable}};
window.methodCoverageAvailable = {{.MethodCoverageAvailable}};
window.metrics = JSON.parse({{.MetricsJSON}});
window.translations = JSON.parse({{.TranslationsJSON}});
window.maximumDecimalPlacesForCoverageQuotas = {{.MaximumDecimalPlacesForCoverageQuotas}};
window.assemblies = JSON.parse({{.AssembliesJSON}});
`

// CreateReport generates the HTML report.
func (r *HTMLReport) CreateReport(summary *model.SummaryResult) error {
	// 1. Ensure the output directory exists
	if err := os.MkdirAll(r.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory '%s': %w", r.outputDir, err)
	}

	// 2. Transform data for Angular frontend
	angularAssemblies := transformSummaryResultToAngularData(summary)

	// 3. Prepare additional global variables
	branchCoverageAvailable := summary.BranchesValid != nil && *summary.BranchesValid > 0
	methodCoverageAvailable := false
	if summary != nil {
		for _, asm := range summary.Assemblies {
			for _, cls := range asm.Classes {
				if len(cls.Methods) > 0 {
					methodCoverageAvailable = true
					break
				}
			}
			if methodCoverageAvailable {
				break
			}
		}
	}

	translations := map[string]string{
		"coverage":             "Coverage",
		"branchCoverage":       "Branch Coverage",
		"methodCoverage":       "Method Coverage",
		"fullMethodCoverage":   "Full Method Coverage",
		"name":                 "Name",
		"covered":              "Covered",
		"uncovered":            "Uncovered",
		"coverable":            "Coverable",
		"total":                "Total",
		"percentage":           "Percentage",
		"metrics":              "Metrics",
		"collapseAll":          "Collapse all",
		"expandAll":            "Expand all",
		"noGrouping":           "No grouping",
		"byAssembly":           "By Assembly",
		"byNamespace":          "By Namespace",
		"grouping":             "Grouping",
		"compareHistory":       "Compare history",
		"date":                 "Date",
		"filter":               "Filter",
		"allChanges":           "All changes",
		"lines":                "Lines",
		"branches":             "Branches",
		"methods":              "Methods",
		"file":                 "File",
		"class":                "Class",
		"averageComplexity":    "Avg. Complexity",
		"coveredLines":         "Covered lines",
		"coverableLines":       "Coverable lines",
		"totalLines":           "Total lines",
		"lineCoverage":         "Line coverage",
		"branchCoverageFull":   "Branch coverage", // Assuming this is for table headers etc.
		"methodCoverageFull":   "Method coverage", // Assuming this is for table headers etc.
		"complexity":           "Complexity",
		"history":              "History",
		"hotspots":             "Hotspots",
		"riskHotspots":         "Risk Hotspots",
		"summary":              "Summary",
		"parser":               "Parser",
		"sourceDirectories":    "Source Directories",
		"generatedOn":          "Generated on",
		"generatedBy":          "Generated by ReportGenerator", // Default value
	}
	historicCoverageExecutionTimes := []string{}
	metrics := []map[string]string{} // No AngularMetric struct yet
	maximumDecimalPlacesForCoverageQuotas := 2

	// Marshal data to JSON strings
	assembliesJSON, err := json.Marshal(angularAssemblies)
	if err != nil {
		return fmt.Errorf("failed to marshal assemblies to JSON: %w", err)
	}
	translationsJSON, err := json.Marshal(translations)
	if err != nil {
		return fmt.Errorf("failed to marshal translations to JSON: %w", err)
	}
	historicCoverageExecutionTimesJSON, err := json.Marshal(historicCoverageExecutionTimes)
	if err != nil {
		return fmt.Errorf("failed to marshal historic coverage times to JSON: %w", err)
	}
	metricsJSON, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics to JSON: %w", err)
	}

	// Prepare data for the template
	templateData := map[string]interface{}{
		"HistoricCoverageExecutionTimesJSON": string(historicCoverageExecutionTimesJSON),
		"BranchCoverageAvailable":            branchCoverageAvailable,
		"MethodCoverageAvailable":            methodCoverageAvailable,
		"MetricsJSON":                        string(metricsJSON),
		"TranslationsJSON":                   string(translationsJSON),
		"MaximumDecimalPlacesForCoverageQuotas": maximumDecimalPlacesForCoverageQuotas,
		"AssembliesJSON":                     string(assembliesJSON),
	}

	// 4. Create assets/report_data.js
	assetsDir := filepath.Join(r.outputDir, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return fmt.Errorf("failed to create assets directory '%s': %w", assetsDir, err)
	}

	reportDataJSPath := filepath.Join(assetsDir, "report_data.js")
	var jsContent bytes.Buffer
	tmpl, err := template.New("report_data.js").Parse(reportDataJSTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse report_data.js template: %w", err)
	}
	if err := tmpl.Execute(&jsContent, templateData); err != nil {
		return fmt.Errorf("failed to execute report_data.js template: %w", err)
	}

	if err := os.WriteFile(reportDataJSPath, jsContent.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write report_data.js to '%s': %w", reportDataJSPath, err)
	}

	// 5. Copy Angular dist contents to outputDir
	angularAppBuildPath := filepath.Join("go_report_generator", "frontend", "dist", "ReportGenerator")
	if _, err := os.Stat(angularAppBuildPath); !os.IsNotExist(err) {
		fmt.Printf("Copying Angular app from %s to %s\n", angularAppBuildPath, r.outputDir)
		err := filepath.WalkDir(angularAppBuildPath, func(srcPath string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relPath, err := filepath.Rel(angularAppBuildPath, srcPath)
			if err != nil {
				return fmt.Errorf("failed to get relative path for '%s': %w", srcPath, err)
			}
			destPath := filepath.Join(r.outputDir, relPath)

			if d.IsDir() {
				if err := os.MkdirAll(destPath, d.Type().Perm()); err != nil {
					return fmt.Errorf("failed to create directory '%s': %w", destPath, err)
				}
			} else {
				srcFile, err := os.Open(srcPath)
				if err != nil {
					return fmt.Errorf("failed to open source file '%s': %w", srcPath, err)
				}
				defer srcFile.Close()

				destFile, err := os.Create(destPath)
				if err != nil {
					return fmt.Errorf("failed to create destination file '%s': %w", destPath, err)
				}
				defer destFile.Close()

				if _, err := io.Copy(destFile, srcFile); err != nil {
					return fmt.Errorf("failed to copy file from '%s' to '%s': %w", srcPath, destPath, err)
				}
				stat, err := os.Stat(srcPath)
				if err != nil {
					return fmt.Errorf("failed to stat source file '%s': %w", srcPath, err)
				}
				if err := os.Chmod(destPath, stat.Mode()); err != nil {
					return fmt.Errorf("failed to set permissions on destination file '%s': %w", destPath, err)
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to copy Angular app assets from '%s' to '%s': %w", angularAppBuildPath, r.outputDir, err)
		}
		fmt.Printf("Successfully copied Angular app assets to %s\n", r.outputDir)

		// 6. Modify index.html in outputDir
		indexHTMLPath := filepath.Join(r.outputDir, "index.html")
		indexHTMLContent, err := os.ReadFile(indexHTMLPath)
		if err != nil {
			return fmt.Errorf("failed to read index.html from '%s': %w", indexHTMLPath, err)
		}

		modifiedIndexHTMLContent := strings.Replace(string(indexHTMLContent), "assets/sampledata/data.js", "assets/report_data.js", 1)

		if err := os.WriteFile(indexHTMLPath, []byte(modifiedIndexHTMLContent), 0644); err != nil {
			return fmt.Errorf("failed to write modified index.html to '%s': %w", indexHTMLPath, err)
		}
		fmt.Printf("Successfully modified index.html to use assets/report_data.js\n")

	} else {
		fmt.Printf("Angular build directory not found at %s, skipping copy of frontend assets and index.html modification.\n", angularAppBuildPath)
	}

	// 7. Return nil on success
	return nil
}
