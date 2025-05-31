// internal/analyzer/assembly.go
package analyzer

import (
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/inputxml"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/utils"
)

func processPackageXML(pkgXML inputxml.PackageXML, sourceDirs []string, uniqueFilePathsForGrandTotalLines map[string]int) (*model.Assembly, error) {
	assembly := model.Assembly{
		Name:    pkgXML.Name,
		Classes: []model.Class{},
	}
	assemblyProcessedFilePaths := make(map[string]struct{})

	classesXMLGrouped := make(map[string][]inputxml.ClassXML)
	for _, classXML := range pkgXML.Classes.Class {
		logicalName := logicalClassName(classXML.Name)
		classesXMLGrouped[logicalName] = append(classesXMLGrouped[logicalName], classXML)
	}

	for logicalName, classXMLGroup := range classesXMLGrouped {
		if isFilteredRawClassName(logicalName) {
			continue
		}
		classModel, err := processClassGroup(classXMLGroup, assembly.Name, sourceDirs, uniqueFilePathsForGrandTotalLines, assemblyProcessedFilePaths)
		if err != nil {
			continue
		}
		if classModel != nil {
			assembly.Classes = append(assembly.Classes, *classModel)
		}
	}

	// Aggregate assembly-level metrics
	var totalAsmBranchesCovered, totalAsmBranchesValid int
	hasAsmBranchData := false

	// Option 1: Keep direct sum (current Go code) - Recommended for simplicity here
	// for i := range assembly.Classes {
	// 	cls := &assembly.Classes[i]
	// 	assembly.LinesCovered += cls.LinesCovered
	// 	assembly.LinesValid += cls.LinesValid

	// 	if cls.BranchesCovered != nil && cls.BranchesValid != nil {
	// 		hasAsmBranchData = true
	// 		totalAsmBranchesCovered += *cls.BranchesCovered
	// 		totalAsmBranchesValid += *cls.BranchesValid
	// 	}
	// }

	// Option 2: Using utils.SafeSumInt (if strict overflow checking desired)

	var allClassLinesCovered []int
	var allClassLinesValid []int
	var allClassBranchesCovered []int
	var allClassBranchesValid []int

	for i := range assembly.Classes {
		cls := &assembly.Classes[i]
		allClassLinesCovered = append(allClassLinesCovered, cls.LinesCovered)
		allClassLinesValid = append(allClassLinesValid, cls.LinesValid)

		if cls.BranchesCovered != nil && cls.BranchesValid != nil {
			hasAsmBranchData = true
			allClassBranchesCovered = append(allClassBranchesCovered, *cls.BranchesCovered)
			allClassBranchesValid = append(allClassBranchesValid, *cls.BranchesValid)
		}
	}
	assembly.LinesCovered = utils.SafeSumInt(allClassLinesCovered)
	assembly.LinesValid = utils.SafeSumInt(allClassLinesValid)
	if hasAsmBranchData {
		bc := utils.SafeSumInt(allClassBranchesCovered)
		bv := utils.SafeSumInt(allClassBranchesValid)
		assembly.BranchesCovered = &bc
		assembly.BranchesValid = &bv
	}

	if hasAsmBranchData { // This part remains common to both options
		assembly.BranchesCovered = &totalAsmBranchesCovered
		assembly.BranchesValid = &totalAsmBranchesValid
	}

	for path := range assemblyProcessedFilePaths {
		if lineCount, ok := uniqueFilePathsForGrandTotalLines[path]; ok {
			assembly.TotalLines += lineCount // Direct sum fine for TotalLines
		}
	}

	return &assembly, nil
}
