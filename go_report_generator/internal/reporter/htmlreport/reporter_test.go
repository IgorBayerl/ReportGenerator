package htmlreport

import (
	"fmt"
	"strings"
	"testing"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
)

func TestTransformSummaryResultToAngularData(t *testing.T) {
	// 3. Construct Sample Input Data
	branchesCoveredClass1 := 5
	branchesValidClass1 := 10
	branchesCoveredClass2 := 0
	branchesValidClass2 := 0 // No branches for this class

	summary := &model.SummaryResult{
		ParserName: "TestParser",
		Assemblies: []model.Assembly{
			{
				Name: "Assembly1",
				Classes: []model.Class{
					{
						Name:        "Namespace.Class1",
						DisplayName: "Class1Display",
						TotalLines:  100, // Total lines in file
						LinesCovered: 20,
						LinesValid:   30, // Coverable lines
						BranchesCovered: &branchesCoveredClass1,
						BranchesValid:   &branchesValidClass1,
						Methods: []model.Method{
							{Name: "Method1A", Signature: "()V", LineRate: 1.0, Complexity: 1, Lines: []model.Line{{Number: 1, Hits: 1}}}, // Fully covered
							{Name: "Method1B", Signature: "()I", LineRate: 0.5, Complexity: 2, Lines: []model.Line{{Number: 2, Hits: 1}, {Number: 3, Hits: 0}}}, // Partially
							{Name: "Method1C", Signature: "()S", LineRate: 0.0, Complexity: 3, Lines: []model.Line{{Number: 4, Hits: 0}}}, // Not covered
						},
					},
					{
						Name:        "Namespace.Class2",
						DisplayName: "", // Should use Name if DisplayName is empty
						TotalLines:  50,
						LinesCovered: 5,
						LinesValid:   10,
						BranchesCovered: &branchesCoveredClass2, // Will result in 0
						BranchesValid:   &branchesValidClass2,   // Will result in 0
						Methods: []model.Method{
							{Name: "Method2A", Signature: "()V", LineRate: 1.0, Complexity: 1, Lines: []model.Line{{Number: 10, Hits: 2}}}, // Fully
						},
					},
				},
			},
			{
				Name: "Assembly2.NoClasses", // Assembly with no classes
				Classes: []model.Class{},
			},
		},
	}

	// 4. Call Transformation Function
	angularData := transformSummaryResultToAngularData(summary)

	// 5. Assert Expected Output
	if len(angularData) != 2 {
		t.Errorf("Expected 2 assemblies, got %d", len(angularData))
		return // Avoid panics on further checks
	}

	// Assembly1 Checks
	asm1 := angularData[0]
	if asm1.Name != "Assembly1" {
		t.Errorf("Assembly1: Expected Name 'Assembly1', got '%s'", asm1.Name)
	}
	if len(asm1.Classes) != 2 {
		t.Errorf("Assembly1: Expected 2 classes, got %d", len(asm1.Classes))
		return
	}

	// Class1Display Checks
	class1 := asm1.Classes[0]
	if class1.Name != "Class1Display" {
		t.Errorf("Class1: Expected Name 'Class1Display', got '%s'", class1.Name)
	}
	expectedRp1 := fmt.Sprintf("%s_report.html", strings.ReplaceAll("Class1Display", "/", "_"))
	if class1.Rp != expectedRp1 {
		t.Errorf("Class1: Expected Rp '%s', got '%s'", expectedRp1, class1.Rp)
	}
	if class1.Cl != 20 {
		t.Errorf("Class1: Expected Cl (Covered Lines) 20, got %d", class1.Cl)
	}
	if class1.Cal != 30 {
		t.Errorf("Class1: Expected Cal (Coverable Lines) 30, got %d", class1.Cal)
	}
	if class1.Ucl != (30 - 20) { // LinesValid - LinesCovered
		t.Errorf("Class1: Expected Ucl (Uncovered Lines) %d, got %d", (30 - 20), class1.Ucl)
	}
	if class1.Tl != 100 {
		t.Errorf("Class1: Expected Tl (Total Lines) 100, got %d", class1.Tl)
	}
	if class1.Cb != branchesCoveredClass1 {
		t.Errorf("Class1: Expected Cb (Covered Branches) %d, got %d", branchesCoveredClass1, class1.Cb)
	}
	if class1.Tb != branchesValidClass1 {
		t.Errorf("Class1: Expected Tb (Total Branches) %d, got %d", branchesValidClass1, class1.Tb)
	}
	if class1.Tm != 3 { // Total methods
		t.Errorf("Class1: Expected Tm (Total Methods) 3, got %d", class1.Tm)
	}
	if class1.Cm != 2 { // Covered methods (LineRate > 0)
		t.Errorf("Class1: Expected Cm (Covered Methods) 2, got %d", class1.Cm)
	}
	if class1.Fcm != 1 { // Fully covered methods (LineRate == 1.0)
		t.Errorf("Class1: Expected Fcm (Fully Covered Methods) 1, got %d", class1.Fcm)
	}

	// Namespace.Class2 Checks (DisplayName was empty)
	class2 := asm1.Classes[1]
	if class2.Name != "Namespace.Class2" {
		t.Errorf("Class2: Expected Name 'Namespace.Class2', got '%s'", class2.Name)
	}
	expectedRp2 := fmt.Sprintf("%s_report.html", strings.ReplaceAll("Namespace.Class2", "/", "_"))
	if class2.Rp != expectedRp2 {
		t.Errorf("Class2: Expected Rp '%s', got '%s'", expectedRp2, class2.Rp)
	}
	if class2.Cl != 5 {
		t.Errorf("Class2: Expected Cl 5, got %d", class2.Cl)
	}
	if class2.Cal != 10 {
		t.Errorf("Class2: Expected Cal 10, got %d", class2.Cal)
	}
	if class2.Ucl != 5 {
		t.Errorf("Class2: Expected Ucl 5, got %d", class2.Ucl)
	}
	if class2.Tl != 50 {
		t.Errorf("Class2: Expected Tl 50, got %d", class2.Tl)
	}
	if class2.Cb != 0 { // BranchesCovered was *0
		t.Errorf("Class2: Expected Cb 0, got %d", class2.Cb)
	}
	if class2.Tb != 0 { // BranchesValid was *0
		t.Errorf("Class2: Expected Tb 0, got %d", class2.Tb)
	}
	if class2.Tm != 1 {
		t.Errorf("Class2: Expected Tm 1, got %d", class2.Tm)
	}
	if class2.Cm != 1 {
		t.Errorf("Class2: Expected Cm 1, got %d", class2.Cm)
	}
	if class2.Fcm != 1 {
		t.Errorf("Class2: Expected Fcm 1, got %d", class2.Fcm)
	}
	
	// Assembly2 Checks
	asm2 := angularData[1]
	if asm2.Name != "Assembly2.NoClasses" {
		t.Errorf("Assembly2: Expected Name 'Assembly2.NoClasses', got '%s'", asm2.Name)
	}
	if len(asm2.Classes) != 0 {
		t.Errorf("Assembly2: Expected 0 classes, got %d", len(asm2.Classes))
	}

	// Test with nil summary
	angularDataNil := transformSummaryResultToAngularData(nil)
	if angularDataNil != nil {
		t.Errorf("Expected nil when summary is nil, got %v", angularDataNil)
	}

	// Test with nil BranchesCovered/BranchesValid
	nilBranchSummary := &model.SummaryResult{
		Assemblies: []model.Assembly{
			{
				Name: "NilBranchAssembly",
				Classes: []model.Class{
					{
						Name:        "NilBranchClass",
						DisplayName: "NilBranchClass",
						LinesCovered: 1, LinesValid: 1, TotalLines: 1,
						BranchesCovered: nil, // Explicitly nil
						BranchesValid:   nil, // Explicitly nil
						Methods: []model.Method{},
					},
				},
			},
		},
	}
	angularDataNilBranch := transformSummaryResultToAngularData(nilBranchSummary)
	if len(angularDataNilBranch) != 1 || len(angularDataNilBranch[0].Classes) != 1 {
		t.Fatalf("NilBranchTest: Expected 1 assembly with 1 class")
	}
	nilBranchClass := angularDataNilBranch[0].Classes[0]
	if nilBranchClass.Cb != 0 {
		t.Errorf("NilBranchClass: Expected Cb 0 when BranchesCovered is nil, got %d", nilBranchClass.Cb)
	}
	if nilBranchClass.Tb != 0 {
		t.Errorf("NilBranchClass: Expected Tb 0 when BranchesValid is nil, got %d", nilBranchClass.Tb)
	}
}
