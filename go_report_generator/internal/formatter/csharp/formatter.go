package csharp

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/formatter"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
)

// C#-specific Regexes.
var (
	// the async method name inside brackets, and the specific "d__<number>/MoveNext()" pattern.
	compilerGeneratedMethodNameRegex = regexp.MustCompile(`^(?P<ClassName>.+)\+<(?P<CompilerGeneratedName>.+)>d__\d+\/MoveNext\(\)$`)

	// This regex correctly handles local functions, including those nested inside generic methods.
	localFunctionMethodNameRegex = regexp.MustCompile(`^(?:.*>g__)?(?P<NestedMethodName>[^|]+)\|`)

	genericClassRegex        = regexp.MustCompile("^(?P<Name>.+)`(?P<Number>\\d+)$")
	nestedTypeSeparatorRegex = regexp.MustCompile(`[+/]`)
)

// CSharpFormatter implements the formatter.LanguageFormatter interface for C#.
type CSharpFormatter struct{}

func init() {
	formatter.RegisterFormatter(NewCSharpFormatter())
}

func NewCSharpFormatter() formatter.LanguageFormatter {
	return &CSharpFormatter{}
}

func (f *CSharpFormatter) Name() string {
	return "C#"
}

// C# or F# source file.
func (f *CSharpFormatter) Detect(filePath string) bool {
	lowerPath := strings.ToLower(filePath)
	return strings.HasSuffix(lowerPath, ".cs") || strings.HasSuffix(lowerPath, ".fs")
}

func (f *CSharpFormatter) GetLogicalClassName(rawClassName string) string {
	if i := strings.IndexAny(rawClassName, "/$+"); i != -1 {
		return rawClassName[:i]
	}
	return rawClassName
}

func (f *CSharpFormatter) FormatClassName(class *model.Class) string {
	nameForDisplay := nestedTypeSeparatorRegex.ReplaceAllString(class.Name, ".")
	match := genericClassRegex.FindStringSubmatch(nameForDisplay)
	if match == nil {
		return nameForDisplay
	}

	baseDisplayName := findNamedGroup(genericClassRegex, match, "Name")
	numberStr := findNamedGroup(genericClassRegex, match, "Number")
	argCount, _ := strconv.Atoi(numberStr)

	if argCount > 0 {
		var sb strings.Builder
		sb.WriteString("<")
		for i := 1; i <= argCount; i++ {
			if i > 1 {
				sb.WriteString(", ")
			}
			sb.WriteString("T")
			if argCount > 1 {
				sb.WriteString(strconv.Itoa(i))
			}
		}
		sb.WriteString(">")
		return baseDisplayName + sb.String()
	}
	return baseDisplayName
}

func (f *CSharpFormatter) FormatMethodName(method *model.Method, class *model.Class) string {
	methodNamePlusSignature := method.Name + method.Signature
	combinedNameForContext := class.Name + "/" + methodNamePlusSignature

	// The '|' character is the most reliable indicator of a compiler-generated
	// local function name. The "g__" is not always present.
	if strings.Contains(methodNamePlusSignature, "|") {
		if match := localFunctionMethodNameRegex.FindStringSubmatch(methodNamePlusSignature); match != nil {
			if nestedName := findNamedGroup(localFunctionMethodNameRegex, match, "NestedMethodName"); nestedName != "" {
				return nestedName + "()"
			}
		}
	}

	// Handle async/await state machines (e.g., <MyAsyncMethod>d__0.MoveNext())
	if strings.HasSuffix(methodNamePlusSignature, "MoveNext()") {
		if match := compilerGeneratedMethodNameRegex.FindStringSubmatch(combinedNameForContext); match != nil {
			if compilerGenName := findNamedGroup(compilerGeneratedMethodNameRegex, match, "CompilerGeneratedName"); compilerGenName != "" {
				return compilerGenName + "()"
			}
		}
	}

	return methodNamePlusSignature
}

func (f *CSharpFormatter) CategorizeCodeElement(method *model.Method) model.CodeElementType {
	if strings.HasPrefix(method.DisplayName, "get_") || strings.HasPrefix(method.DisplayName, "set_") {
		return model.PropertyElementType
	}
	return model.MethodElementType
}

func (f *CSharpFormatter) IsCompilerGeneratedClass(class *model.Class) bool {
	rawName := class.Name
	if strings.Contains(rawName, "+<>c") || strings.Contains(rawName, "/<>c") || strings.HasPrefix(rawName, "<>c") || strings.Contains(rawName, ">d__") {
		return true
	}
	return false
}

func findNamedGroup(re *regexp.Regexp, match []string, groupName string) string {
	for i, name := range re.SubexpNames() {
		if i > 0 && i < len(match) && name == groupName {
			return match[i]
		}
	}
	return ""
}
