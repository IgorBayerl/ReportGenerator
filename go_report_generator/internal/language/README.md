# Creating a New Language Processor

This guide provides a comprehensive walkthrough for adding a new language-specific processor to the `go-report-generator` project.

## The Role of a Language Processor

Before writing code, it's essential to understand the application's data processing pipeline:

`Parse -> Process -> Analyze -> Report`

A language processor has two main responsibilities: it acts as a **beautician** for names and a **metric calculator** for source code. It takes the raw, unrefined data produced by a `Parser` and transforms it into a clean, human-readable, and semantically rich state before it is analyzed and passed to the `Reporters`.

#### What a Processor Does:

*   **Cleans up** compiler-generated names (e.g., C# async/await state machines, lambdas).
*   **Filters out** compiler-generated "noise" classes that shouldn't appear in the report.
*   **Standardizes** display names (e.g., converting C# generics like `List\`1` into `List<T>`).
*   **Categorizes** code elements based on language conventions (e.g., identifying a method as a `get_` or `set_` property accessor).
*   **Calculates** language-specific metrics (like cyclomatic complexity) that are not present in the input coverage file.

#### What a Processor **Does Not** Do:

*   It **does not** read or parse any input files. That is the job of a `Parser`.
*   It **does not** merge multiple reports. That is the job of the `Analyzer`.
*   It **does not** generate HTML, text summaries, or any other final output. That is the job of a `Reporter`.

The application framework uses the processor to refine and enrich the data immediately after parsing, ensuring that all subsequent steps work with a clean and consistent data model.

## The `Processor` Interface: Your Contract

Your new processor **must** implement the `Processor` interface defined in `internal/language/language.go`. This is the bridge between the application framework and your language-specific implementation.

| Method                                                                 | Responsibility                                                                                                                                                             | Implementation Notes                                                                                                                                                                                                                                                                                                                          |
| :--------------------------------------------------------------------- | :------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | :-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **`Name() string`**                                                    | Return the unique, human-readable name of your processor (e.g., "C#", "Go", "Java").                                                                                       | This name is used in logs to indicate which processor was auto-detected.                                                                                                                                                                                                                                                                  |
| **`Detect(filePath string) bool`**                                     | Quickly and efficiently determine if this processor is the correct one for a given source file path.                                                                       | **This is the most critical method for automatic detection.** It should check the file extension (e.g., `.cs`, `.go`, `.java`). The factory uses this to select the right processor for each file being processed.                                                                                                                   |
| **`GetLogicalClassName(rawClassName string) string`**                  | Determines the logical "parent" class name from a potentially compiler-generated raw name.                                                                                 | Crucial for parsers to group partial classes or nested helper classes correctly *before* creating the final `model.Class`. E.g., `MyClass+<Sub>d__0` should return `MyClass`.                                                                                                                                                            |
| **`FormatClassName(class *model.Class) string`**                       | Transforms the raw class name into a display-friendly version.                                                                                                             | The primary use case is handling language features like generics. If no formatting is needed, simply return `class.Name`.                                                                                                                                                                                                                 |
| **`FormatMethodName(method *model.Method, class *model.Class) string`**| Transforms a raw method name and signature into a display-friendly version.                                                                                                | This is a main beautification step. For C#, this is where you clean up `MoveNext()` from async methods. For Go, you might just return the raw name.                                                                                                                                                                                 |
| **`CategorizeCodeElement(method *model.Method) model.CodeElementType`**| Determines if a method is a standard method or a special language construct like a property accessor.                                                                      | The C# processor will check for `get_` and `set_` prefixes to return `PropertyElementType`. A Go or Java processor would likely always return `MethodElementType`.                                                                                                                                                               |
| **`IsCompilerGeneratedClass(class *model.Class) bool`**                | Determines if a class is a compiler-generated artifact that should be filtered out of the report entirely.                                                                   | This is crucial for languages like C# that create hidden helper classes for lambdas and async operations. If a class should be removed, this method must return `true`.                                                                                                                                                                |
| **`CalculateCyclomaticComplexity(...)`**                      | Calculates language-specific metrics like cyclomatic complexity for a given source file.                                                                                   | This is where you integrate tools like `gocyclo`. If the language does not support this metric (e.g., C# in this project), the implementation **must** return the sentinel error `language.ErrNotSupported`. A `nil` error indicates success. |

## Step-by-Step Guide: Creating a New Language Processor

Follow this structure to create a processor for a new language (e.g., "Go").

#### Step 1: Create the Processor Package

Create a new, self-contained directory inside `internal/language/`.

```
internal/language/
└── golang/                #<-- New directory for your processor
    └── processor.go       #<-- Your main processor logic will live here
```

#### Step 2: Implement the `Processor` Interface

In `processor.go`, create your processor struct and make it implement the `language.Processor` interface. This is a complete example for the Go language.

```go
// in: internal/language/golang/processor.go
package golang

import (
	"fmt"
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

func (p *GoProcessor) Name() string { return "Go" }
func (p *GoProcessor) Detect(filePath string) bool {
	return strings.HasSuffix(strings.ToLower(filePath), ".go")
}
func (p *GoProcessor) GetLogicalClassName(rawClassName string) string { return rawClassName }
func (p *GoProcessor) FormatClassName(class *model.Class) string       { return class.Name }
func (p *GoProcessor) FormatMethodName(method *model.Method, class *model.Class) string {
	return method.Name + method.Signature
}
func (p *GoProcessor) CategorizeCodeElement(method *model.Method) model.CodeElementType {
	return model.MethodElementType
}
func (p *GoProcessor) IsCompilerGeneratedClass(class *model.Class) bool { return false }

// CalculateCyclomaticComplexity uses the gocyclo library to analyze a Go source file.
func (p *GoProcessor) CalculateCyclomaticComplexity(filePath string) ([]model.MethodMetric, error) {
	stats := gocyclo.Analyze([]string{filePath}, nil)

	metrics := make([]model.MethodMetric, 0, len(stats))
	for _, s := range stats {
		metric := model.MethodMetric{
			Name: s.FuncName,
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
	return metrics, nil
}
```

#### Step 3: Register Your Processor

In the same `processor.go` file, add an `init()` function to register your processor with the central factory.

```go
// in: internal/language/golang/processor.go

func init() {
	// This line automatically tells the application about your new processor
	language.RegisterProcessor(NewGoProcessor())
}
```