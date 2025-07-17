package golang

import (
	"strings"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/language"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
	"github.com/fzipp/gocyclo"
)

// GoProcessor implements the language.Processor interface for Go.
type GoProcessor struct{}

// NewGoProcessor creates a new, stateless GoProcessor.
func NewGoProcessor() language.Processor {
	return &GoProcessor{}
}

// Name returns the unique, human-readable name of the processor.
func (p *GoProcessor) Name() string {
	return "Go"
}

// Detect checks if the file path has a .go extension.
func (p *GoProcessor) Detect(filePath string) bool {
	return strings.HasSuffix(strings.ToLower(filePath), ".go")
}

// GetLogicalClassName returns the raw class name. For Go, this is the package path.
func (p *GoProcessor) GetLogicalClassName(rawClassName string) string {
	return rawClassName
}

// FormatClassName performs no special formatting for Go package paths.
func (p *GoProcessor) FormatClassName(class *model.Class) string {
	return class.Name
}

// FormatMethodName performs no special formatting for Go method names.
func (p *GoProcessor) FormatMethodName(method *model.Method, class *model.Class) string {
	return method.Name + method.Signature
}

// CategorizeCodeElement consistently identifies elements from a Go report as standard methods.
func (p *GoProcessor) CategorizeCodeElement(method *model.Method) model.CodeElementType {
	return model.MethodElementType
}

// IsCompilerGeneratedClass always returns false for Go.
func (p *GoProcessor) IsCompilerGeneratedClass(class *model.Class) bool {
	return false
}

// CalculateCyclomaticComplexity uses the gocyclo library to analyze a Go source file.
func (p *GoProcessor) CalculateCyclomaticComplexity(filePath string) ([]model.MethodMetric, error) {
	// CORRECTED: gocyclo.Analyze expects a slice of paths and does not return an error.
	// We pass the single file path in a slice and nil for the optional ignore regex.
	stats := gocyclo.Analyze([]string{filePath}, nil)

	metrics := make([]model.MethodMetric, 0, len(stats))
	for _, s := range stats {
		metric := model.MethodMetric{
			Name: s.FuncName, // e.g., "(MyType).myFunc" or "myFunc"
			Line: s.Pos.Line,
			Metrics: []model.Metric{
				{
					Name:   "Cyclomatic complexity",
					Value:  float64(s.Complexity),
					Status: model.StatusOk,
				},
			},
		}
		metrics = append(metrics, metric)
	}

	// Since gocyclo doesn't return an error for analysis, we return nil on success.
	return metrics, nil
}
