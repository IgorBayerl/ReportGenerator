package defaultFormatter

import (
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/language"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
)

// DefaultProcessor implements the language.Processor interface.
// It serves as a fallback for languages that are not explicitly handled,
// performing no special name formatting or filtering and supporting no metrics.
type DefaultProcessor struct{}

func init() {
	// Register this default processor with the central factory.
	language.RegisterProcessor(NewDefaultProcessor())
}

// NewDefaultProcessor creates a new, stateless DefaultProcessor.
func NewDefaultProcessor() language.Processor {
	return &DefaultProcessor{}
}

// Name returns the unique, human-readable name of the processor.
func (p *DefaultProcessor) Name() string {
	return "Default"
}

// Detect always returns false. The factory logic is responsible for choosing
// this processor as a fallback if no other specific processor detects a match.
func (p *DefaultProcessor) Detect(filePath string) bool {
	return false
}

func (p *DefaultProcessor) GetLogicalClassName(rawClassName string) string {
	return rawClassName
}

// FormatClassName performs no formatting and returns the raw class name.
func (p *DefaultProcessor) FormatClassName(class *model.Class) string {
	return class.Name
}

// FormatMethodName performs no formatting and returns the raw method name and signature.
func (p *DefaultProcessor) FormatMethodName(method *model.Method, class *model.Class) string {
	return method.Name + method.Signature
}

// CategorizeCodeElement assumes all code elements are standard methods.
func (p *DefaultProcessor) CategorizeCodeElement(method *model.Method) model.CodeElementType {
	return model.MethodElementType
}

// IsCompilerGeneratedClass assumes no classes are compiler-generated.
func (p *DefaultProcessor) IsCompilerGeneratedClass(class *model.Class) bool {
	return false
}

// CalculateCyclomaticComplexity returns an error indicating this feature is not supported.
func (p *DefaultProcessor) CalculateCyclomaticComplexity(filePath string) ([]model.MethodMetric, error) {
	return nil, language.ErrNotSupported
}
