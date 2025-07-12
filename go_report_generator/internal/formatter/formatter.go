package formatter

import (
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
)

// LanguageFormatter defines the contract for language-specific processing.
type LanguageFormatter interface {
	// Name returns the unique, human-readable name of the formatter (e.g., "C#", "Go").
	Name() string

	// Detect checks if this formatter should be used for a given source file path.
	Detect(filePath string) bool

	// GetLogicalClassName determines the grouping key for a class from a raw name.
	// E.g., for C#, it simplifies "MyClass+<Sub>d__0" to "MyClass".
	// For Go, it would likely return the input as-is.
	GetLogicalClassName(rawClassName string) string

	// FormatClassName transforms a raw class name into a display-friendly version.
	// E.g., for C#, it handles generics: "List`1" -> "List<T>".
	FormatClassName(class *model.Class) string

	// FormatMethodName transforms a raw method name and signature into a display-friendly version.
	FormatMethodName(method *model.Method, class *model.Class) string

	// CategorizeCodeElement determines if a method is a standard method, property, etc.
	CategorizeCodeElement(method *model.Method) model.CodeElementType

	// IsCompilerGeneratedClass determines if a class is a compiler-generated artifact
	// that should be filtered out from the final report.
	IsCompilerGeneratedClass(class *model.Class) bool
}

var registeredFormatters []LanguageFormatter

// RegisterFormatter adds a formatter to the list of available formatters.
// This should be called by each formatter implementation in its init() function.
func RegisterFormatter(f LanguageFormatter) {
	registeredFormatters = append(registeredFormatters, f)
}

// FindFormatterForFile iterates through registered formatters to find one that
// can handle the given file path. It is guaranteed to return a valid formatter,
// falling back to the "Default" formatter.
func FindFormatterForFile(filePath string) LanguageFormatter {
	var defaultFormatter LanguageFormatter

	for _, f := range registeredFormatters {
		if f.Name() == "Default" {
			defaultFormatter = f
			continue
		}
		if f.Detect(filePath) {
			return f
		}
	}

	if defaultFormatter != nil {
		return defaultFormatter
	}

	panic("FATAL: Default language formatter was not registered.")
}
