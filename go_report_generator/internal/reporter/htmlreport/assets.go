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

// initializeAssets initializes and sets up all required assets for the HTML report.
// It copies static and Angular assets to the output directory and parses the Angular
// index.html to extract critical CSS and JavaScript file references.
// Returns an error if any critical operation fails.
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

// copyStaticAssets copies static asset files from the embedded/source assets directory
// to the report's output directory. It handles CSS, JavaScript, and other static files
// required by the HTML reports. It also combines custom CSS files into a single report.css.
// Returns an error if any critical file operation fails.
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
		srcPath := filepath.Join(assetsDir, fileName)
		dstPath := filepath.Join(b.OutputDir, fileName)

		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory for asset %s: %w", dstPath, err)
		}

		srcFile, err := os.Open(srcPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to open source asset %s (path: %s): %v. Skipping.\n", fileName, srcPath, err)
			continue
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
		fmt.Fprintf(os.Stderr, "Warning: failed to read custom.css for combining into report.css: %v\n", err)
	}

	customDarkCSSBytes, err := os.ReadFile(filepath.Join(assetsDir, "custom_dark.css"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to read custom_dark.css for combining into report.css: %v\n", err)
	}

	var combinedCSS []byte
	if len(customCSSBytes) > 0 {
		combinedCSS = append(combinedCSS, customCSSBytes...)
		combinedCSS = append(combinedCSS, []byte("\n")...)
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
		fmt.Fprintf(os.Stderr, "Warning: custom.css and custom_dark.css were not found; report.css may be missing or incomplete.\n")
	}

	return nil
}

// copyAngularAssets recursively copies all files from the Angular app's dist folder
// to the report's output directory, preserving the directory structure and file permissions.
// Returns an error if the source directory doesn't exist or if any file operation fails.
func (b *HtmlReportBuilder) copyAngularAssets(outputDir string) error {
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
				if chmodErr := os.Chmod(dstPath, srcFileInfo.Mode()); chmodErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to set permissions on %s: %v\n", dstPath, chmodErr)
				}
			}
		}
		return nil
	})
}

// parseAngularIndexHTML parses the Angular index.html file and extracts references to
// critical assets including CSS and JavaScript files (runtime, polyfills, and main).
// Returns the file paths for CSS, runtime JS, polyfills JS, and main JS files.
// Returns an error if the file cannot be opened or parsed.
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
				isModule := false
				for _, a := range n.Attr {
					if a.Key == "src" {
						src = a.Val
					}
					if a.Key == "type" && a.Val == "module" {
						isModule = true
					}
				}
				if src != "" && isModule {
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
	return
}
