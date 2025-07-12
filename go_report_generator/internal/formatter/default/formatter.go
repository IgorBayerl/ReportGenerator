package defaultFormatter

import (
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/formatter"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
)

// DefaultFormatter implements the formatter.LanguageFormatter interface.
// It serves as a fallback for languages that are not explicitly handled,
// performing no special name formatting or filtering.
type DefaultFormatter struct{}

func init() {
	// Register this default formatter with the central factory.
	formatter.RegisterFormatter(NewDefaultFormatter())
}

// NewDefaultFormatter creates a new, stateless DefaultFormatter.
func NewDefaultFormatter() formatter.LanguageFormatter {
	return &DefaultFormatter{}
}

// Name returns the unique, human-readable name of the formatter.
func (f *DefaultFormatter) Name() string {
	return "Default"
}

// Detect always returns false. The factory logic is responsible for choosing
// this formatter as a fallback if no other specific formatter detects a match.
func (f *DefaultFormatter) Detect(filePath string) bool {
	return false
}

func (f *DefaultFormatter) GetLogicalClassName(rawClassName string) string {
	return rawClassName
}

// FormatClassName performs no formatting and returns the raw class name.
func (f *DefaultFormatter) FormatClassName(class *model.Class) string {
	return class.Name
}

// FormatMethodName performs no formatting and returns the raw method name and signature.
func (f *DefaultFormatter) FormatMethodName(method *model.Method, class *model.Class) string {
	return method.Name + method.Signature
}

// CategorizeCodeElement assumes all code elements are standard methods, as it has
// no language-specific knowledge of properties or other special types.
func (f *DefaultFormatter) CategorizeCodeElement(method *model.Method) model.CodeElementType {
	return model.MethodElementType
}

// IsCompilerGeneratedClass assumes no classes are compiler-generated, as it
// cannot know the conventions of an unknown language.
func (f *DefaultFormatter) IsCompilerGeneratedClass(class *model.Class) bool {
	return false
}
