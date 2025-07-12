package golang

import (
	"strings"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/formatter"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
)

// GoFormatter implements the formatter.LanguageFormatter interface for Go.
// Its primary roles are to correctly identify reports from Go projects and
// to ensure that no C#-specific formatting is applied to Go's clean names.
type GoFormatter struct{}

func init() {
	// Register this formatter with the central factory via its init function.
	formatter.RegisterFormatter(NewGoFormatter())
}

// NewGoFormatter creates a new, stateless GoFormatter.
func NewGoFormatter() formatter.LanguageFormatter {
	return &GoFormatter{}
}

// Name returns the unique, human-readable name of the formatter.
func (f *GoFormatter) Name() string {
	return "Go"
}

// Detect checks if the file path has a .go extension, which is the
// most reliable indicator of a Go source file.
func (f *GoFormatter) Detect(filePath string) bool {
	return strings.HasSuffix(strings.ToLower(filePath), ".go")
}

// GetLogicalClassName returns the raw class name. For Go reports converted to
// Cobertura, the "class name" is the package path, which is already the
// correct logical grouping.
func (f *GoFormatter) GetLogicalClassName(rawClassName string) string {
	return rawClassName
}

// FormatClassName performs no special formatting for Go. The raw package path
// is the desired display name for a "class".
func (f *GoFormatter) FormatClassName(class *model.Class) string {
	return class.Name
}

// FormatMethodName performs no special formatting for Go method names. The raw
// name from the coverage report is typically clean and does not include signatures.
func (f *GoFormatter) FormatMethodName(method *model.Method, class *model.Class) string {
	// Go method names from coverage tools do not include signatures,
	// so concatenating Name and an empty Signature is safe and correct.
	return method.Name + method.Signature
}

// CategorizeCodeElement consistently identifies elements from a Go report as
// standard methods, as Go does not have language features like C# properties
// that require special categorization.
func (f *GoFormatter) CategorizeCodeElement(method *model.Method) model.CodeElementType {
	return model.MethodElementType
}

// IsCompilerGeneratedClass always returns false. Go does not produce compiler-generated
// helper classes (for features like lambdas or async/await) in a way that
// would appear as separate classes in a coverage report.
func (f *GoFormatter) IsCompilerGeneratedClass(class *model.Class) bool {
	return false
}
