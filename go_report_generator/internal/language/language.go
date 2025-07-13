package language

import (
	"errors"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
)

// ErrNotSupported is a sentinel error returned by a Processor when a feature
// like cyclomatic complexity is not applicable to that language.
var ErrNotSupported = errors.New("feature not supported for this language")

// Processor defines the contract for all language-specific logic.
type Processor interface {
	// Name returns the unique, human-readable name of the processor (e.g., "C#", "Go").
	Name() string

	// Detect checks if this processor should be used for a given source file path.
	Detect(filePath string) bool

	// GetLogicalClassName determines the grouping key for a class from a raw name.
	GetLogicalClassName(rawClassName string) string

	// FormatClassName transforms a raw class name into a display-friendly version.
	FormatClassName(class *model.Class) string

	// FormatMethodName transforms a raw method name and signature into a display-friendly version.
	FormatMethodName(method *model.Method, class *model.Class) string

	// CategorizeCodeElement determines if a method is a standard method, property, etc.
	CategorizeCodeElement(method *model.Method) model.CodeElementType

	// IsCompilerGeneratedClass determines if a class is a compiler-generated artifact
	// that should be filtered out from the final report.
	IsCompilerGeneratedClass(class *model.Class) bool

	// CalculateCyclomaticComplexity analyzes a file and returns the metric for each function.
	// If the language does not support this metric, it must return language.ErrNotSupported.
	CalculateCyclomaticComplexity(filePath string) ([]model.MethodMetric, error)
}

var registeredProcessors []Processor

// RegisterProcessor adds a processor to the list of available processors.
// This should be called by each processor implementation in its init() function.
func RegisterProcessor(p Processor) {
	registeredProcessors = append(registeredProcessors, p)
}

// FindProcessorForFile iterates through registered processors to find one that
// can handle the given file path. It is guaranteed to return a valid processor,
// falling back to the "Default" processor.
func FindProcessorForFile(filePath string) Processor {
	var defaultProcessor Processor

	for _, p := range registeredProcessors {
		if p.Name() == "Default" {
			defaultProcessor = p
			continue
		}
		if p.Detect(filePath) {
			return p
		}
	}

	if defaultProcessor != nil {
		return defaultProcessor
	}

	panic("FATAL: Default language processor was not registered.")
}
