package analyzer

import (
	"fmt"
	"time"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser" // For parser.ParserResult
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/reportconfig"
)

// MergeParserResults takes multiple ParserResult objects (potentially from different files
// or even different parser types) and merges them into a single, unified model.SummaryResult.
// The config is needed to access global settings or filters if they are applied at merge time
// (though ideally filters are applied within each parser).
func MergeParserResults(results []*parser.ParserResult, config reportconfig.IReportConfiguration) (*model.SummaryResult, error) {
	if len(results) == 0 {
		return nil, fmt.Errorf("no parser results to merge")
	}

	finalSummary := &model.SummaryResult{
		Assemblies: []model.Assembly{},
		// Initialize other fields like ParserName, Timestamps later
	}

	// Determine overall parser name
	parserNames := make(map[string]struct{})
	for _, res := range results {
		if res.ParserName != "" {
			parserNames[res.ParserName] = struct{}{}
		}
	}
	if len(parserNames) == 1 {
		for name := range parserNames {
			finalSummary.ParserName = name
			break
		}
	} else if len(parserNames) > 1 {
		finalSummary.ParserName = "MultiReport" // Or concatenate, e.g., "Cobertura, OpenCover"
	} else {
		finalSummary.ParserName = "Unknown"
	}

	// Determine overall min/max timestamps
	var minTs, maxTs *time.Time
	for _, res := range results {
		if res.MinimumTimeStamp != nil {
			if minTs == nil || res.MinimumTimeStamp.Before(*minTs) {
				minTs = res.MinimumTimeStamp
			}
		}
		if res.MaximumTimeStamp != nil {
			if maxTs == nil || res.MaximumTimeStamp.After(*maxTs) {
				maxTs = res.MaximumTimeStamp
			}
		}
	}
	if minTs != nil {
		finalSummary.Timestamp = minTs.Unix() // Or handle range if min/max differ significantly
	}

	// Collect all source directories
	allSourceDirsSet := make(map[string]struct{})
	for _, res := range results {
		for _, dir := range res.SourceDirectories {
			allSourceDirsSet[dir] = struct{}{}
		}
	}
	for dir := range allSourceDirsSet {
		finalSummary.SourceDirs = append(finalSummary.SourceDirs, dir)
	}

	// Merge assemblies
	mergedAssembliesMap := make(map[string]*model.Assembly)
	for _, res := range results {
		for _, asmFromParser := range res.Assemblies {
			asmCopy := asmFromParser // Work with a copy to avoid modifying original parser result data
			if existingAsm, ok := mergedAssembliesMap[asmCopy.Name]; ok {
				// TODO: Implement robust merge logic for assembly
				// This involves merging classes, and then recursively files, lines, methods.
				// For now, simple append and re-aggregate (will lead to duplicates if not careful)
				// See C# Assembly.Merge(), Class.Merge(), CodeFile.Merge() for inspiration.
				// This is a placeholder for complex merge:
				existingAsm.Classes = append(existingAsm.Classes, asmCopy.Classes...) // Simplistic
			} else {
				mergedAssembliesMap[asmCopy.Name] = &asmCopy
			}
		}
	}

	// Convert map to slice and re-aggregate stats for merged assemblies
	uniqueFilesForGrandTotal := make(map[string]int)
	for _, asm := range mergedAssembliesMap {
		// Re-aggregate assembly stats *after* its classes might have been merged from multiple sources
		// This is a complex step. For now, the stats on `asm` are from its last `ParserResult`.
		// A true merge would sum up counts from constituent parts and recalculate.
		// Placeholder for re-aggregation of asm.LinesCovered, asm.LinesValid, etc.
		// and asm.TotalLines based on unique files within this merged assembly.
		finalSummary.Assemblies = append(finalSummary.Assemblies, *asm)
	}

	// Aggregate global stats for finalSummary
	var globalLinesCovered, globalLinesValid, globalTotalLines int
	var globalBranchesCovered, globalBranchesValid int
	hasBranchData := false

	for i := range finalSummary.Assemblies {
		asm := &finalSummary.Assemblies[i] // Operate on the final list
		globalLinesCovered += asm.LinesCovered
		globalLinesValid += asm.LinesValid
		// TotalLines for summary needs to be from unique file paths across *all* assemblies
		for _, cls := range asm.Classes {
			for _, f := range cls.Files {
				if _, exists := uniqueFilesForGrandTotal[f.Path]; !exists && f.TotalLines > 0 {
					uniqueFilesForGrandTotal[f.Path] = f.TotalLines
					globalTotalLines += f.TotalLines
				}
			}
		}

		if asm.BranchesCovered != nil && asm.BranchesValid != nil {
			hasBranchData = true
			globalBranchesCovered += *asm.BranchesCovered
			globalBranchesValid += *asm.BranchesValid
		}
	}

	finalSummary.LinesCovered = globalLinesCovered
	finalSummary.LinesValid = globalLinesValid
	finalSummary.TotalLines = globalTotalLines
	if hasBranchData {
		finalSummary.BranchesCovered = &globalBranchesCovered
		finalSummary.BranchesValid = &globalBranchesValid
	}

	return finalSummary, nil
}

// NOTE: The old `analyzer.Analyze` function should be DELETED.
// The files `analyzer/assembly.go`, `analyzer/class.go`, `analyzer/codefile.go`,
// `analyzer/line.go`, `analyzer/method.go` should also be DELETED or have their
// Cobertura-specific logic moved to `internal/parser/cobertura/`.
// The `analyzer` package will now primarily focus on the `MergeParserResults` logic.
// If there are generic analysis utilities (like calculating overall method coverage percentages
// from a `SummaryResult`), they can remain or be added here.