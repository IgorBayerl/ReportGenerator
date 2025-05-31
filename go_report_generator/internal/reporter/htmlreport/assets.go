package htmlreport

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/utils"
)

var angularDistSourcePath = filepath.Join(utils.ProjectRoot(), "angular_frontend_spa", "dist")

// initializeAssets handles all aspects of asset setup and copying
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
		if cssFile == "" || runtimeJs == "" || mainJs == "" {
			return fmt.Errorf("missing critical Angular assets from index.html (css: '%s', runtime: '%s', main: '%s')", cssFile, runtimeJs, mainJs)
		}
	}

	b.angularCssFile = cssFile
	b.angularRuntimeJsFile = runtimeJs
	b.angularPolyfillsJsFile = polyfillsJs
	b.angularMainJsFile = mainJs
	return nil
}

// copyStaticAssets copies static assets from the source directory to the output directory
func (b *HtmlReportBuilder) copyStaticAssets() error {
	// Implementation for copying static assets
	return nil
}

// copyAngularAssets copies Angular assets to the output directory
func (b *HtmlReportBuilder) copyAngularAssets(outputDir string) error {
	// Implementation for copying Angular assets
	return nil
}

// parseAngularIndexHTML parses index.html and returns the script and style references
func (b *HtmlReportBuilder) parseAngularIndexHTML(indexPath string) (cssFile, runtimeJs, polyfillsJs, mainJs string, err error) {
	// Implementation for parsing Angular index.html
	return
}
