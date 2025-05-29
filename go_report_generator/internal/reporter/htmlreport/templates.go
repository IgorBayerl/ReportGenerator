package htmlreport

import (
	"html"
	"html/template"
	"strings"
)

const baseLayoutTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{ .Title }}</title>
    <!-- Static CSS for overall page structure and theme -->
    <link rel="stylesheet" type="text/css" href="report.css">
    <!-- Chartist CSS for charts (if used by static part or Angular) -->
    <link rel="stylesheet" type="text/css" href="chartist.min.css">
    <!-- Angular App's CSS -->
    <link rel="stylesheet" type="text/css" href="{{ .AngularCssFile }}">
</head>
<body>
    <!-- Data for Angular -->
    <script>
        {{if .ClassDetailJSON}}window.classDetails = JSON.parse({{.ClassDetailJSON}});{{end}}
        window.assemblies = JSON.parse({{.AssembliesJSON}});
        window.riskHotspots = JSON.parse({{.RiskHotspotsJSON}});
        window.metrics = JSON.parse({{.MetricsJSON}});
        window.riskHotspotMetrics = JSON.parse({{.RiskHotspotMetricsJSON}});
        window.historicCoverageExecutionTimes = JSON.parse({{.HistoricCoverageExecutionTimesJSON}});
        window.translations = JSON.parse({{.TranslationsJSON}});

        window.branchCoverageAvailable = {{.BranchCoverageAvailable}};
        window.methodCoverageAvailable = {{.MethodCoverageAvailable}};
        window.maximumDecimalPlacesForCoverageQuotas = {{.MaximumDecimalPlacesForCoverageQuotas}};
    </script>

    <!-- Traditional report header - this might be removed or restyled if Angular takes over full page -->
    <h1>Report for {{ .ParserName }}</h1>
    <p>Generated on: {{ .GeneratedAt }}</p>
    
    <!-- Angular root component will be bootstrapped here -->
    <div id="content">
        <app-root></app-root>
    </div>

    <!-- Static JS for charts or other elements not handled by Angular -->
    <script type="text/javascript" src="chartist.min.js"></script>
    <script type="text/javascript" src="custom.js"></script>

    <!-- Angular App's JS files -->
    <script src="{{ .AngularRuntimeJsFile }}" type="module"></script>
    <script src="{{ .AngularPolyfillsJsFile }}" type="module"></script>
    <script src="{{ .AngularMainJsFile }}" type="module"></script>
</body>
</html>`

const classDetailLayoutTemplate = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1.0" />
<meta http-equiv="X-UA-Compatible" content="IE=EDGE,chrome=1" />
<link href="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAMAAABEpIrGAAAAn1BMVEUAAADCAAAAAAA3yDfUAAA3yDfUAAA8PDzr6+sAAAD4+Pg3yDeQkJDTAADt7e3V1dU3yDdCQkIAAADbMTHUAABBykHUAAA2yDY3yDfr6+vTAAB3diDR0dGYcHDUAAAjhiPSAAA3yDeuAADUAAA3yDf////OCALg9+BLzktBuzRelimzKgv87+/dNTVflSn1/PWz6rO126g5yDlYniy0KgwjJ0TyAAAAI3RSTlMABAj0WD6rJcsN7X1HzMqUJyYW+/X08+bltqSeaVRBOy0cE+citBEAAADBSURBVDjLlczXEoIwFIThJPYGiL0XiL3r+z+bBOJs9JDMuLffP8v+Gxfc6aIyDQVjQcnqnvRDEQwLJYtXpZT+YhDHKIjLbS+OUeT4TjkKi6OwOArq+yeKXD9uDqQQbcOjyCy0e6bTojZSftX+U6zUQ7OuittDu1k0WHqRFfdXQijgjKfF6ZwAikvmKD6OQjmKWUcDigkztm5FZN05nMON9ZcoinlBmTNnAUdBnRbUUbgdBZwWbkcBpwXcVsBtxfjb31j1QB5qeebOAAAAAElFTkSuQmCC" rel="icon" type="image/x-icon" />
<title>{{.Class.Name}} - {{.ReportTitle}}</title>
<link rel="stylesheet" type="text/css" href="report.css" />
<link rel="stylesheet" type="text/css" href="{{.AngularCssFile}}">
</head>
<body>
    <script>
        window.classDetails = JSON.parse({{.ClassDetailJSON}});
        window.assemblies = JSON.parse({{.AssembliesJSON}});
        window.translations = JSON.parse({{.TranslationsJSON}});
        window.branchCoverageAvailable = {{.BranchCoverageAvailable}};
        window.methodCoverageAvailable = {{.MethodCoverageAvailable}};
        window.maximumDecimalPlacesForCoverageQuotas = {{.MaximumDecimalPlacesForCoverageQuotas}};
        window.riskHotspots = JSON.parse({{.RiskHotspotsJSON}}); // Ensure these are valid JSON, e.g., "[]" or "{}" if empty
        window.metrics = JSON.parse({{.MetricsJSON}});
        window.riskHotspotMetrics = JSON.parse({{.RiskHotspotMetricsJSON}});
        window.historicCoverageExecutionTimes = JSON.parse({{.HistoricCoverageExecutionTimesJSON}});
    </script>

    <div class="container">
        <div class="containerleft">
            <h1><a href="index.html" class="back"><</a> {{.Translations.Summary}}</h1>

            <div class="card-group">
                <div class="card">
                    <div class="card-header">{{.Translations.Information}}</div>
                    <div class="card-body">
                        <div class="table">
                            <table>
                                <tr><th>{{.Translations.Class}}:</th><td class="limit-width" title="{{.Class.Name}}">{{.Class.Name}}</td></tr>
                                <tr><th>{{.Translations.Assembly}}:</th><td class="limit-width" title="{{.Class.AssemblyName}}">{{.Class.AssemblyName}}</td></tr>
                                <tr><th>{{.Translations.Files3}}:</th><td class="overflow-wrap">
                                    {{$filesLen := len .Class.Files}}
                                    {{$lastFileIdx := sub $filesLen 1}}
                                    {{range $idx, $file := .Class.Files}}
                                        <a href="#{{$file.ShortPath}}" class="navigatetohash">{{$.Translations.File}} {{$idx | inc}}: {{$file.Path}}</a>{{if ne $idx $lastFileIdx}}<br />{{end}}
                                    {{else}}
                                        No files found.
                                    {{end}}
                                </td></tr>
                                {{if .Tag}}
                                <tr><th>{{.Translations.Tag}}:</th><td class="limit-width" title="{{.Tag}}">{{.Tag}}</td></tr>
                                {{end}}
                            </table>
                        </div>
                    </div>
                </div>
            </div>

            <div class="card-group">
                <div class="card">
                    <div class="card-header">{{.Translations.LineCoverage}}</div>
                    <div class="card-body">
                        <div class="large cardpercentagebar cardpercentagebar{{.Class.CoveragePercentageBarValue}}">{{.Class.CoveragePercentageForDisplay}}</div>
                        <div class="table">
                            <table>
                                <tr><th>{{.Translations.CoveredLines}}:</th><td class="limit-width right" title="{{.Class.CoveredLines}}">{{.Class.CoveredLines}}</td></tr>
                                <tr><th>{{.Translations.UncoveredLines}}:</th><td class="limit-width right" title="{{.Class.UncoveredLines}}">{{.Class.UncoveredLines}}</td></tr>
                                <tr><th>{{.Translations.CoverableLines}}:</th><td class="limit-width right" title="{{.Class.CoverableLines}}">{{.Class.CoverableLines}}</td></tr>
                                <tr><th>{{.Translations.TotalLines}}:</th><td class="limit-width right" title="{{.Class.TotalLines}}">{{.Class.TotalLines}}</td></tr>
                                <tr><th>{{.Translations.LineCoverage}}:</th><td class="limit-width right" title="{{.Class.CoveredLines}} of {{.Class.CoverableLines}}">{{.Class.CoverageRatioTextForDisplay}}</td></tr>
                            </table>
                        </div>
                    </div>
                </div>
                {{if .BranchCoverageAvailable}}
                <div class="card">
                    <div class="card-header">{{.Translations.BranchCoverage}}</div>
                    <div class="card-body">
                        <div class="large cardpercentagebar cardpercentagebar{{.Class.BranchCoveragePercentageBarValue}}">{{.Class.BranchCoveragePercentageForDisplay}}</div>
                        <div class="table">
                            <table>
                                <tr><th>{{.Translations.CoveredBranches2}}:</th><td class="limit-width right" title="{{.Class.CoveredBranches}}">{{.Class.CoveredBranches}}</td></tr>
                                <tr><th>{{.Translations.TotalBranches}}:</th><td class="limit-width right" title="{{.Class.TotalBranches}}">{{.Class.TotalBranches}}</td></tr>
                                <tr><th>{{.Translations.BranchCoverage}}:</th><td class="limit-width right" title="{{.Class.CoveredBranches}} of {{.Class.TotalBranches}}">{{.Class.BranchCoverageRatioTextForDisplay}}</td></tr>
                            </table>
                        </div>
                    </div>
                </div>
                {{end}}
                 <div class="card">
                    <div class="card-header">{{.Translations.MethodCoverage}}</div>
                    <div class="card-body">
                        {{if .MethodCoverageAvailable}}
                        <div class="large cardpercentagebar cardpercentagebar{{.Class.MethodCoveragePercentageBarValue}}">{{.Class.MethodCoveragePercentageForDisplay}}</div>
                        <div class="table">
                            <table>
                                <tr><th>{{.Translations.CoveredCodeElements}}:</th><td class="limit-width right" title="{{.Class.CoveredMethods}}">{{.Class.CoveredMethods}}</td></tr>
                                <tr><th>{{.Translations.FullCoveredCodeElements}}:</th><td class="limit-width right" title="{{.Class.FullyCoveredMethods}}">{{.Class.FullyCoveredMethods}}</td></tr>
                                <tr><th>{{.Translations.TotalCodeElements}}:</th><td class="limit-width right" title="{{.Class.TotalMethods}}">{{.Class.TotalMethods}}</td></tr>
                                <tr><th>{{.Translations.CodeElementCoverageQuota2}}:</th><td class="limit-width right" title="{{.Class.CoveredMethods}} of {{.Class.TotalMethods}}">{{.Class.MethodCoverageRatioTextForDisplay}}</td></tr>
                                <tr><th>{{.Translations.FullCodeElementCoverageQuota2}}:</th><td class="limit-width right" title="{{.Class.FullyCoveredMethods}} of {{.Class.TotalMethods}}">{{.Class.FullMethodCoverageRatioTextForDisplay}}</td></tr>
                            </table>
                        </div>
                        {{else}}
                        <div class="center">
                            <p>{{.Translations.MethodCoverageProVersion}}</p>
                            <a class="pro-button" href="https://reportgenerator.io/pro" target="_blank">{{.Translations.MethodCoverageProButton}}</a>
                        </div>
                        {{end}}
                    </div>
                </div>
            </div>

            {{if .Class.FilesWithMetrics}}
            <h1>{{.Translations.Metrics}}</h1>
            <div class="table-responsive">
                <table class="overview table-fixed">
                    <colgroup>
                        <col class="column-min-200" />
                        {{range .Class.MetricsTable.Headers}}
                        <col class="column105" />
                        {{end}}
                    </colgroup>
                    <thead><tr><th>{{$.Translations.Method}}</th>
                        {{range .Class.MetricsTable.Headers}}
                        <th>{{.Name}} {{if .ExplanationURL}}<a href="{{.ExplanationURL}}" target="_blank"><i class="icon-info-circled"></i></a>{{end}}</th>
                        {{end}}
                    </tr></thead>
                    <tbody>
                        {{range .Class.MetricsTable.Rows}}
                        <tr><td title="{{.FullName}}"><a href="#{{.FileShortPath}}_line{{.Line}}" class="navigatetohash">{{if $.Class.IsMultiFile}}File {{.FileIndexPlus1}}: {{end}}{{.Name}}</a></td>
                            {{range .MetricValues}}<td>{{.}}</td>{{end}}
                        </tr>
                        {{end}}
                    </tbody>
                </table>
            </div>
            {{end}}

            <h1>{{.Translations.Files3}}</h1>
            {{range $fileIdx, $file := .Class.Files}}
            <h2 id="{{$file.ShortPath}}">{{$file.Path}}</h2>
            <div class="table-responsive">
                <table class="lineAnalysis">
                    <thead><tr><th></th><th>#</th><th>{{$.Translations.Line}}</th><th></th><th>{{$.Translations.LineCoverage}}</th></tr></thead>
                    <tbody>
                    {{range $file.Lines}}
                        <tr class="{{if ne .LineVisitStatus "gray"}}coverableline{{end}}" title="{{.Tooltip}}" data-coverage="{{.DataCoverage}}">
                            <td class="{{.LineVisitStatus}}"> </td>
                            <td class="leftmargin rightmargin right">{{if ne .LineVisitStatus "gray"}}{{.Hits}}{{end}}</td>
                            <td class="rightmargin right"><a id="{{$file.ShortPath}}_line{{.LineNumber}}"></a><code>{{.LineNumber}}</code></td>
                            {{if .IsBranch}}
                            <td class="percentagebar percentagebar{{.BranchBarValue}}"><i class="icon-fork"></i></td>
                            {{else}}
                            <td></td>
                            {{end}}
                            <td class="light{{.LineVisitStatus}}"><code>{{.LineContent | SanitizeSourceLine}}</code></td>
                        </tr>
                    {{end}}
                    </tbody>
                </table>
            </div>
            {{else}}
                <p>{{.Translations.NoFilesFound}}</p>
            {{end}}

            <div class="footer">{{.Translations.GeneratedBy}} ReportGenerator {{.AppVersion}}<br />{{.CurrentDateTime}}<br /><a href="https://github.com/danielpalme/ReportGenerator">GitHub</a> | <a href="https://reportgenerator.io">reportgenerator.io</a></div>
        </div> 

        {{if .Class.SidebarElements}}
        <div class="containerright">
            <div class="containerrightfixed">
                <h1>{{.Translations.MethodsProperties}}</h1>
                {{range .Class.SidebarElements}}
                <a href="#{{.FileShortPath}}_line{{.Line}}" class="navigatetohash percentagebar percentagebar{{.CoverageBarValue}}" title="{{if $.Class.IsMultiFile}}File {{.FileIndexPlus1}}: {{end}}{{.CoverageTitle}} - {{.Name}}"><i class="icon-{{.Icon}}"></i>{{.Name}}</a><br />
                {{end}}
                <br/>
            </div>
        </div>
        {{end}}
    </div> 

    <script type="text/javascript" src="custom.js"></script> 
    <script src="{{.AngularRuntimeJsFile}}" type="module"></script>
    {{if .AngularPolyfillsJsFile}}<script src="{{.AngularPolyfillsJsFile}}" type="module"></script>{{end}}
    <script src="{{.AngularMainJsFile}}" type="module"></script>
</body>
</html>`


// Parse this new template
var classDetailTpl = template.Must(template.New("classDetail").Funcs(template.FuncMap{
	"inc": func(i int) int { return i + 1 },
	"sub": func(a, b int) int { return a - b },
	"SanitizeSourceLine": func(line string) template.HTML {
		// 1. HTML-escape first to get &lt;, &gt;, &amp; …
        escaped := html.EscapeString(line)

        // 2. Replace TABs with four real spaces first (so that step 3 sees them)
        escaped = strings.ReplaceAll(escaped, "\t", "    ")

        // 3. Turn every real space into &nbsp;
        escaped = strings.ReplaceAll(escaped, " ", "&nbsp;")

        return template.HTML(escaped)   // mark it safe – we built the HTML ourselves
	},
}).Parse(classDetailLayoutTemplate))




var (
	baseTpl = template.Must(template.New("base").Parse(baseLayoutTemplate))
)


/*
// getBaseLayoutTemplate provides access to the parsed base layout template.
// This is an alternative to using the global baseTpl directly.
func getBaseLayoutTemplate() (*template.Template, error) {
	// If baseTpl was not initialized with template.Must, it could be parsed here.
	// return template.New("base").Parse(baseLayoutTemplate)

	// Since baseTpl is initialized with template.Must, we can return a clone
	// if modification per-use is a concern, or the original if it's read-only.
	// For simplicity, returning the shared instance is fine if it's not modified.
	return baseTpl, nil // template.Must ensures no error here unless Parse fails at init
}
*/
