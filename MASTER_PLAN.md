# MASTER_PLAN.md: Porting ReportGenerator HTML Reporting to Go

## 🎯 Overall Project Goal
To port the HTML reporting functionality from the C# ReportGenerator tool to the Go version (`go_report_generator`). The initial focus is on processing Cobertura XML input and generating rich HTML reports, aiming for feature parity with the original C# tool's HTML output. The architecture should allow for future expansion to other report types and input formats.

---
## 📊 Current Status (Go Project)
* **Input Parsing:** Successfully parses Cobertura XML files.
* **Internal Data Model:** An internal data model exists, populated from the Cobertura input.
* **Reporting Engine:**
    * Basic reporter interface (`ReportBuilder`) is defined.
    * `TextSummary` report generation is implemented and functional.
* **CLI:** Basic command-line interface accepts input XML and report type.
---
## leveragedLeverage (Go Port vs. C# Original)
* **Cobertura Parsing (Go: Done):**
    `go_report_generator/internal/parser/cobertura.go` parses the XML into `inputxml.CoberturaRoot`.
    This is analogous to C#'s `CoberturaParser.cs`.
* **Internal Data Model (Go: Done, Needs Verification for Completeness):**
    `go_report_generator/internal/model/*` (e.g., `analysis.go`, `metric.go`, etc.) defines structs like `SummaryResult`, `Assembly`, `Class`, `CodeFile`, `Method`, `Line`, `Metric`, `CodeElement`.
    This is analogous to C#'s `ReportGenerator.Core/Parser/Analysis/*`.
    *Action:* Verify if all necessary fields from C#'s analysis model (especially those used by HTML rendering like `HistoricCoverage`, `TestMethod` links, detailed branch info per line) are present or can be easily added to the Go model. Your `model/analysis.go Line` struct already has `CoveredBranches` and `TotalBranches`, which is great. `HistoricCoverage` is also present.
* **Analyzer (Go: Done, Needs Verification for Completeness):**
    `go_report_generator/internal/analyzer/*` populates the Go internal data model from `inputxml.CoberturaRoot`.
    This is analogous to the processing logic within C# parsers that hydrate `Parser.Analysis.Assembly`, `Parser.Analysis.Class`, etc.
    *Action:* Similar to the data model, ensure the analyzer populates all data points required for the rich HTML report.
* **Reporter Abstraction (Go: Done):**
    `go_report_generator/internal/reporter/reporter.go` defines the `ReportBuilder` interface.
    `go_report_generator/internal/reporter/textsummary/reporter.go` is an existing implementation. This provides a pattern.
* **Angular SPA (C#: Can be directly COPIED and BUILT):**
    The entire `src/AngularComponents` directory from the C# project.
    Its build process (`ng build` defined in `angular.json`) produces static assets (JS, CSS, `index.html` shell).
    *Key Insight:* The Go backend does not *run* Angular. It will *serve* or *integrate* the pre-compiled static assets from Angular's `dist` folder.
* **Static Assets (CSS/JS) (C#: Can be directly COPIED):**
    Files in `src/ReportGenerator.Core/Reporting/Builders/Rendering/resources/` (e.g., `custom.css`, `custom.js`, `chartist.min.css/js`) can be copied into an `assets/` directory in the Go project.
* **HTML Rendering Logic (C#: Needs TRANSLATION to Go):**
    C#'s `HtmlRenderer.cs` and `HtmlReportBuilderBase.cs` contain the core logic for:
    * Generating HTML structure (summary page, class detail pages).
    * Iterating through the data model to populate tables, lists, metrics.
    * Creating links between summary and detail pages.
    * Embedding or linking static/Angular assets.
    * Generating JavaScript data variables for the Angular app.
    This logic will be translated into Go, primarily using the `html/template` package and Go string manipulation.
* **Source File Reading (Go: Partially Exists, Needs Integration):**
    `go_report_generator/internal/filereader/filereader.go` exists for counting lines. It needs to be enhanced or complemented to read file content for display.
    C#'s `SourceFileRenderer.cs` (or similar logic within `HtmlRenderer`) handles reading source files and applying CSS classes. This part needs translation to Go.

---
## 🔑 Key Considerations & Principles
* **Reference C# Project:** The original C# `ReportGenerator` project is the primary source of truth for features, logic, and UI.
* **Expandability:** Design Go interfaces and structures (especially for reporters and data processing) to easily accommodate new report types (e.g., XML, JSON, Markdown Summary) or input formats in the future without major refactoring.
* **Modularity:** Keep components (parsing, analysis, report generation, asset handling) as decoupled as possible.
* **Configuration:** Plan for report generation configuration options (e.g., via CLI, `.json` file).
* **Translations:** The Angular app uses a `translations` JavaScript object. These strings need to be extracted from C#'s `ReportResources.resx` and made available to the Go-generated HTML.

---
## 🔄 Mapping: C# to Go (Assets & Logic)

This section tracks how components from the C# project are handled in the Go port.

| C# Component / Feature                      | Go Port Strategy                                       | Status / Notes                                                                 |
| :------------------------------------------ | :----------------------------------------------------- | :----------------------------------------------------------------------------- |
| **Core Logic & Data Structures** |                                                        |                                                                                |
| `CoberturaParser.cs`                        | Translate to Go (`internal/parser/cobertura.go`)       | DONE (Initial version)                                                         |
| `Parser/Analysis/*` (Data Model)            | Translate/Adapt to Go structs (`internal/model/*`)     | DONE (Initial version, ongoing verification for HTML needs)                    |
| Analyzer logic (populating model)           | Translate/Adapt to Go (`internal/analyzer/*`)          | DONE (Initial version, ongoing verification for HTML needs)                    |
| **HTML Rendering Engine** |                                                        |                                                                                |
| `HtmlRenderer.cs`                           | Translate to Go `html/template` & Go functions       | PENDING (Core of HTML generation logic)                                        |
| `HtmlReportBuilderBase.cs`                  | Adapt concepts for Go `HtmlReportBuilder`              | PENDING                                                                        |
| **Static Assets (Non-Angular)** |                                                        |                                                                                |
| `Reporting/Builders/Rendering/resources/*`  | **Copy** to `go_report_generator/assets/htmlreport/` | PENDING (CSS, JS, themes like `custom_dark.css`)                               |
| **Angular SPA** |                                                        |                                                                                |
| `src/AngularComponents/*`                   | **Copy** to `go_report_generator/angular_frontend_spa/`      | DONE (Entire Angular project)                                               |
| Angular Build Process (`ng build`)          | Replicate via `npm` scripts                            | PENDING                                                                        |
| Angular Data Injection (JS variables)       | Replicate via Go templates embedding JSON              | PENDING                                                                        |
| **Report Resources & Translations** |                                                        |                                                                                |
| `ReportResources.resx` / `*.Designer.cs`    | Extract strings, make available as JS object in Go     | PENDING (For Angular app's i18n)                                               |
| **File Handling** |                                                        |                                                                                |
| `LocalFileReader.cs` / `IFileReader.cs`     | Adapt/Translate to Go (`internal/filereader`)          | PARTIALLY DONE (Basic line counting exists, needs enhancement for content) |
| **Other Report Types (Future)** |                                                        |                                                                                |
| `TextSummaryReportBuilder.cs`               | Translated to Go (`internal/reporter/textsummary`)     | DONE                                                                           |
| Other builders (XML, Badge, Cobertura etc.) | Plan for future translation/adaptation                 | FUTURE                                                                         |

---
## ✅ Completed Milestones Log
* **[Date]** Initial Cobertura XML parsing implemented.
* **[Date]** Basic internal data model created.
* **[Date]** TextSummary report generation functional.
* **[Date]** CLI arguments for input/output and TextSummary report type.
* **[Date]** Phase 0: Pre-flight Checks & Setup - CLI argument for "Html" added.
* **2025-05-29** Phase 1: Basic HTML Structure & Asset Management - Completed all steps including HTML Report Builder setup, static assets integration, and Angular SPA integration.
* **2025-05-29** Phase 1, Step 1.1: HTML Report Builder and Basic Templates (Go) - Initial `HtmlReportBuilder` and `base_layout.gohtml` created.
* **2025-05-29** Phase 1, Step 1.2: Copy and Integrate Static Assets (CSS/JS from C# `resources/`) - Non-Angular CSS/JS assets integrated and linked in HTML templates.
* **2025-05-29** Phase 1, Step 1.3: Integrate Pre-compiled Angular SPA Assets - Angular project copied, build process established, and assets integrated with Go HTML generation.