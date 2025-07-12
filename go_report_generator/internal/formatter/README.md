# Creating a New Language Formatter

This guide provides a comprehensive walkthrough for adding a new language-specific formatter to the `go-report-generator` project.

## The Role of a Language Formatter

Before writing code, it's essential to understand the application's data processing pipeline:

`Parse -> Format -> Analyze -> Report`

A formatter's single responsibility is to act as a **beautician** or **finisher**. It takes the raw, unrefined data produced by a `Parser` and transforms it into a clean, human-readable, and semantically correct state before it is analyzed and passed to the `Reporters`.

#### What a Formatter Does:

*   **Cleans up** compiler-generated names (e.g., C# async/await state machines, lambdas).
*   **Filters out** compiler-generated "noise" classes that shouldn't appear in the report.
*   **Standardizes** display names (e.g., converting C# generics like `List\`1` into `List<T>`).
*   **Categorizes** code elements based on language conventions (e.g., identifying a method as a `get_` or `set_` property accessor).

#### What a Formatter **Does Not** Do:

*   It **does not** read or parse any input files. That is the job of a `Parser`.
*   It **does not** merge multiple reports. That is the job of the `Analyzer`.
*   It **does not** generate HTML, text summaries, or any other final output. That is the job of a `Reporter`.

The application framework uses the formatter to refine the data immediately after parsing, ensuring that all subsequent steps work with a clean and consistent data model.

## The `LanguageFormatter` Interface: Your Contract

Your new formatter **must** implement the `LanguageFormatter` interface defined in `internal/formatter/formatter.go`. This is the bridge between the application framework and your language-specific implementation.

| Method                                               | Responsibility                                                                                                                                                                          | Implementation Notes                                                                                                                                                                                                                                                                 |
| :--------------------------------------------------- | :-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | :--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **`Name() string`**                                  | Return the unique, human-readable name of your formatter (e.g., "C#", "Go", "Java").                                                                                                     | This name is used in logs to indicate which formatter was auto-detected.                                                                                                                                                                                                                 |
| **`Detect(filePath string) bool`** | Quickly and efficiently determine if this formatter is the correct one for a given source file path. | **This is the most critical method for automatic detection.** It should check the file extension (e.g., `.cs`, `.go`, `.java`). The factory uses this to select the right formatter for each file being processed. |
| **`GetLogicalClassName(rawClassName string) string`** | Determines the logical "parent" class name from a potentially compiler-generated raw name. | Crucial for parsers to group partial classes or nested helper classes correctly *before* creating the final `model.Class`. E.g., `MyClass+<Sub>d__0` should return `MyClass`. |
| **`FormatClassName(class *model.Class) string`**     | Transforms the raw class name into a display-friendly version.                                                                                                                          | The primary use case is handling language features like generics. If no formatting is needed, simply return `class.Name`.                                                                                                                                                                |
| **`FormatMethodName(method *model.Method, class *model.Class) string`** | Transforms a raw method name and signature into a display-friendly version. | This is the main beautification step. For C#, this is where you clean up `MoveNext()` from async methods. For Go, you might just return the raw name.                                                                                                                                 |
| **`CategorizeCodeElement(method *model.Method) model.CodeElementType`** | Determines if a method is a standard method or a special language construct like a property accessor.                                                                 | The C# formatter will check for `get_` and `set_` prefixes to return `PropertyElementType`. A Go or Java formatter would likely always return `MethodElementType`.                                                                                                                     |
| **`IsCompilerGeneratedClass(class *model.Class) bool`** | Determines if a class is a compiler-generated artifact that should be filtered out of the report entirely.                                                                            | This is crucial for languages like C# that create hidden helper classes for lambdas and async operations. If a class should be removed, this method must return `true`.                                                                                                                     |


## Step-by-Step Guide: Creating a New Language Formatter

Follow this structure to create a formatter for a new language (e.g., "Go").

#### Step 1: Create the Formatter Package

Create a new, self-contained directory inside `internal/formatter/`.

```
internal/formatter/
└── golang/                #<-- New directory for your formatter
    └── formatter.go       #<-- Your main formatter logic will live here
```

#### Step 2: Implement the `LanguageFormatter` Interface

In `formatter.go`, create your formatter struct and make it implement the `formatter.LanguageFormatter` interface.

```go
// in: internal/formatter/golang/formatter.go
package golang

import (
	"strings"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/formatter"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
)

type GoFormatter struct{}

func NewGoFormatter() formatter.LanguageFormatter { return &GoFormatter{} }
func (f *GoFormatter) Name() string { return "Go" }
func (f *GoFormatter) Detect(filePath string) bool { return strings.HasSuffix(strings.ToLower(filePath), ".go") }
func (f *GoFormatter) GetLogicalClassName(rawClassName string) string { return rawClassName }
func (f *GoFormatter) FormatClassName(class *model.Class) string { return class.Name }
func (f *GoFormatter) FormatMethodName(method *model.Method, class *model.Class) string { return method.Name + method.Signature }
func (f *GoFormatter) CategorizeCodeElement(method *model.Method) model.CodeElementType { return model.MethodElementType }
func (f *GoFormatter) IsCompilerGeneratedClass(class *model.Class) bool { return false }
```

#### Step 3: Register Your Formatter

In the same `formatter.go` file, add an `init()` function to register your formatter with the central factory.

```go
// in: internal/formatter/golang/formatter.go

func init() {
	// This line automatically tells the application about your new formatter
	formatter.RegisterFormatter(NewGoFormatter())
}
```

---

## 2. Concrete Refactoring Plan

Here is the step-by-step plan to refactor the codebase according to the new architecture.

### Step 1: Create the New `formatter` Interface and Factory

Create/replace the file `internal/formatter/formatter.go` with the new interface definition and a factory to find the appropriate formatter.

```go
// In: internal/formatter/formatter.go
package formatter

import (
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
)

// LanguageFormatter defines the contract for language-specific processing,
// such as cleaning up method names, classifying code elements, and filtering
// out compiler-generated artifacts.
type LanguageFormatter interface {
	// Name returns the unique, human-readable name of the formatter (e.g., "C#", "Go").
	Name() string

 	// Detect checks if this formatter can handle the given file path,
    // typically by checking the file extension.
    Detect(filePath string) bool

 	// GetLogicalClassName determines the grouping key for a class. For C#,
    // this simplifies compiler-generated names (e.g., "MyClass+<Sub>d__0")
    // to their parent ("MyClass"), allowing the parser to merge them correctly.
    GetLogicalClassName(rawClassName string) string

    // FormatClassName transforms a raw class name into a display-friendly version.
    FormatClassName(class *model.Class) string

	// FormatMethodName transforms a raw method name and signature into a display-friendly version.
	// This is where logic for cleaning up async methods, local functions, or properties lives.
	FormatMethodName(method *model.Method, class *model.Class) string

	// CategorizeCodeElement determines if a method is a standard method or a special
	// type, like a property accessor, based on its name and language conventions.
	CategorizeCodeElement(method *model.Method) model.CodeElementType

	// IsCompilerGeneratedClass determines if a class is a compiler-generated artifact
	// that should be filtered out from the final report (e.g., C# display classes for lambdas).
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