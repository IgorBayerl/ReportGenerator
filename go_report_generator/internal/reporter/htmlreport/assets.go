package htmlreport

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/utils"
	"golang.org/x/net/html"
)

var (
	assetsDir             = filepath.Join(utils.ProjectRoot(), "assets", "htmlreport")
	angularDistSourcePath = filepath.Join(utils.ProjectRoot(), "angular_frontend_spa", "dist")
)

// initializeAssets handles all aspects of asset setup and copying
func (b *HtmlReportBuilder) initializeAssets() error {
	// These copy functions might succeed
	if err := b.copyStaticAssets(); err != nil {
		return fmt.Errorf("failed to copy static assets: %w", err)
	}
	if err := b.copyAngularAssets(b.OutputDir); err != nil {
		return fmt.Errorf("failed to copy angular assets: %w", err)
	}

	// The problem is likely here:
	angularIndexHTMLPath := filepath.Join(angularDistSourcePath, "index.html")
	cssFile, runtimeJs, polyfillsJs, mainJs, err := b.parseAngularIndexHTML(angularIndexHTMLPath)
	if err != nil {
		return fmt.Errorf("failed to parse Angular index.html: %w", err)
	}

	// This check is what's triggering your error message:
	if cssFile == "" || runtimeJs == "" || mainJs == "" {
		fmt.Fprintf(os.Stderr, "Warning: One or more Angular assets might be missing (css: %s, runtime: %s, polyfills: %s, main: %s)\n", cssFile, runtimeJs, polyfillsJs, mainJs)
		if cssFile == "" || runtimeJs == "" || mainJs == "" { // Check critical ones
			return fmt.Errorf("missing critical Angular assets from index.html (css: '%s', runtime: '%s', main: '%s')", cssFile, runtimeJs, mainJs)
		}
	}
	b.angularCssFile = cssFile
	b.angularRuntimeJsFile = runtimeJs
	b.angularPolyfillsJsFile = polyfillsJs // Note: polyfillsJs can be empty if not used by Angular build
	b.angularMainJsFile = mainJs
	return nil
}

// copyStaticAssets copies static assets (like custom.css, report.css, custom.js)
// from the embedded/source assets directory to the report's output directory.
func (b *HtmlReportBuilder) copyStaticAssets() error {
	// Files directly in assetsDir that are not part of the Angular build but are needed by the non-Angular HTML reports.
	// This list usually includes the base report.css (if not fully replaced by Angular's) and any custom JS/CSS.
	filesToCopy := []string{
		"custom.css", // Example custom CSS for non-Angular parts
		"custom.js",  // Example custom JS for non-Angular parts
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
		srcPath := filepath.Join(assetsDir, fileName) // assetsDir is your 'assets/htmlreport'
		dstPath := filepath.Join(b.OutputDir, fileName)

		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for asset %s: %w", dstPath, err)
		}

		srcFile, err := os.Open(srcPath)
		if err != nil {
			// Don't fail hard if a non-critical custom file is missing, maybe just log.
			// But for core files like a base report.css, it should be an error.
			fmt.Fprintf(os.Stderr, "Warning: failed to open source asset %s (path: %s): %v. Skipping.\n", fileName, srcPath, err)
			continue // Or return err if critical
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

	// Special handling for report.css (as in original builder.go)
	// This combines custom.css and custom_dark.css into the final report.css
	// Ensure custom.css and custom_dark.css are in your assetsDir.
	customCSSBytes, err := os.ReadFile(filepath.Join(assetsDir, "custom.css"))
	if err != nil {
		// If custom.css is optional, log a warning. If critical, return error.
		fmt.Fprintf(os.Stderr, "Warning: failed to read custom.css for combining into report.css: %v\n", err)
	}

	customDarkCSSBytes, err := os.ReadFile(filepath.Join(assetsDir, "custom_dark.css"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to read custom_dark.css for combining into report.css: %v\n", err)
	}

	var combinedCSS []byte
	if len(customCSSBytes) > 0 {
		combinedCSS = append(combinedCSS, customCSSBytes...)
		combinedCSS = append(combinedCSS, []byte("\n")...) // Add a newline separator
	}
	if len(customDarkCSSBytes) > 0 {
		combinedCSS = append(combinedCSS, customDarkCSSBytes...)
	}

	if len(combinedCSS) > 0 {
		err = os.WriteFile(filepath.Join(b.OutputDir, "report.css"), combinedCSS, 0644)
		if err != nil {
			return fmt.Errorf("failed to write combined report.css: %w", err)
		}
	} else {
		// If both were missing, you might want a default empty report.css or ensure one is copied.
		// For now, if both are missing, report.css won't be created by this specific logic block.
		// Consider copying a base report.css if one exists and this dynamic creation isn't the primary source.
		fmt.Fprintf(os.Stderr, "Warning: custom.css and custom_dark.css were not found; report.css may be missing or incomplete.\n")
	}

	return nil
}

// copyAngularAssets copies all files from the Angular app's dist folder
// (e.g., angular_frontend_spa/dist) to the report's output directory.
func (b *HtmlReportBuilder) copyAngularAssets(outputDir string) error {
	// angularDistSourcePath is your 'angular_frontend_spa/dist'
	srcInfo, err := os.Stat(angularDistSourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("angular source directory %s does not exist: %w. Make sure to build the Angular app first (e.g., 'npm run build' in 'angular_frontend_spa')", angularDistSourcePath, err)
		}
		return fmt.Errorf("failed to stat angular source directory %s: %w", angularDistSourcePath, err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("angular source path %s is not a directory", angularDistSourcePath)
	}

	// Walk the source directory (angularDistSourcePath)
	return filepath.WalkDir(angularDistSourcePath, func(srcPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Error accessing path, e.g. permission issue
			return fmt.Errorf("error accessing path %s during walk: %w", srcPath, walkErr)
		}

		// Get the relative path of the current file/dir with respect to the source root
		relPath, err := filepath.Rel(angularDistSourcePath, srcPath)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", srcPath, err)
		}

		// Construct the destination path in the output directory
		dstPath := filepath.Join(outputDir, relPath)

		if d.IsDir() {
			// If it's a directory, create it in the destination
			// MkdirAll is idempotent, so it's fine if it already exists
			if err := os.MkdirAll(dstPath, 0755); err != nil { // Use 0755 for directory permissions
				return fmt.Errorf("failed to create directory %s: %w", dstPath, err)
			}
		} else {
			// If it's a file, copy it
			srcFile, err := os.Open(srcPath)
			if err != nil {
				return fmt.Errorf("failed to open source file %s: %w", srcPath, err)
			}
			defer srcFile.Close()

			// Ensure destination directory exists (it should if MkdirAll worked above for parent)
			// but good practice for files directly under outputDir if angularDistSourcePath has files at root
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

			// Attempt to set same permissions as source file
			srcFileInfo, statErr := d.Info()
			if statErr == nil {
				os.Chmod(dstPath, srcFileInfo.Mode()) // This might fail on some systems/permissions, but try
			}
		}
		return nil
	})
}

// parseAngularIndexHTML parses index.html and returns the script and style references
func (b *HtmlReportBuilder) parseAngularIndexHTML(angularIndexHTMLPath string) (cssFile, runtimeJs, polyfillsJs, mainJs string, err error) {
	file, err := os.Open(angularIndexHTMLPath)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to open Angular index.html at %s: %w", angularIndexHTMLPath, err)
	}
	defer file.Close()

	doc, err := html.Parse(file) // Using golang.org/x/net/html
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
					cssFile = href // Found CSS file
				}
			} else if n.Data == "script" {
				var src string
				isModule := false // Angular scripts are typically type="module"
				for _, a := range n.Attr {
					if a.Key == "src" {
						src = a.Val
					}
					if a.Key == "type" && a.Val == "module" {
						isModule = true
					}
				}
				if src != "" && isModule { // Only consider module scripts for runtime, polyfills, main
					// filepath.Base(src) is important if src contains paths like "static/js/runtime.js"
					baseSrc := filepath.Base(src)
					if strings.HasPrefix(baseSrc, "runtime.") && strings.HasSuffix(baseSrc, ".js") {
						runtimeJs = src
					} else if strings.HasPrefix(baseSrc, "polyfills.") && strings.HasSuffix(baseSrc, ".js") {
						polyfillsJs = src
					} else if strings.HasPrefix(baseSrc, "main.") && strings.HasSuffix(baseSrc, ".js") {
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
	return // cssFile, runtimeJs, polyfillsJs, mainJs, nil (err is already nil)
}
