package analyzer

import (
	"fmt"
	"time"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser" // For parser.ParserResult
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/parser/filtering"
)

type MergerConfig interface {
    // Let's analyze what it actually needs. Looking at your code,
    // it uses SourceDirectories(). Let's assume it might also need filters.
    // If it needs nothing, the interface would be empty or the parameter could be removed.
    SourceDirectories() []string
    AssemblyFilters() filtering.IFilter 
}


// MergeParserResults takes multiple ParserResult objects (potentially from different files
// or even different parser types) and merges them into a single, unified model.SummaryResult.
// The config is needed to access global settings or filters if they are applied at merge time
// (though ideally filters are applied within each parser).
func MergeParserResults(results []*parser.ParserResult, config MergerConfig) (*model.SummaryResult, error) {
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
				// Instead of just appending classes, also sum up the simple stats.
				// This is still not a full deep merge but better than current.
				existingAsm.LinesCovered += asmCopy.LinesCovered
				existingAsm.LinesValid += asmCopy.LinesValid // This might overcount if same lines are in both
				// Consider making LinesValid a calculation based on unique lines after merging.

				if asmCopy.BranchesCovered != nil {
					if existingAsm.BranchesCovered == nil {
						bc := *asmCopy.BranchesCovered
						existingAsm.BranchesCovered = &bc
					} else {
						*existingAsm.BranchesCovered += *asmCopy.BranchesCovered
					}
				}
				if asmCopy.BranchesValid != nil {
					if existingAsm.BranchesValid == nil {
						bv := *asmCopy.BranchesValid
						existingAsm.BranchesValid = &bv
					} else {
						// This logic for TotalBranches/LinesValid needs to be careful
						// not to double-count if the underlying entities (lines with branches)
						// are the same but reported in multiple files. A true deep merge
						// handles this by merging at the line level.
						// For a simple sum, this might lead to inflated TotalBranches if not careful.
						// A safer bet if deep merge isn't done is to just use stats from the first file,
						// which is what it implicitly does now for the assembly stats, and accept undercounting.
						// OR, ensure the Cobertura files being merged do not have overlapping assembly/class/file definitions.
						*existingAsm.BranchesValid += *asmCopy.BranchesValid
					}
				}

				// TODO: Still need to handle merging of classes, files, lines properly for detailed views.
				// For now, appending classes will lead to duplicates if the same class is in multiple reports for this assembly.
				existingAsm.Classes = append(existingAsm.Classes, asmCopy.Classes...)
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
