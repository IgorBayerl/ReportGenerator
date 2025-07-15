package csharp

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/language"
	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/model"
)

// C#-specific Regexes.
var (
	compilerGeneratedMethodNameRegex = regexp.MustCompile(`^(?P<ClassName>.+)\+<(?P<CompilerGeneratedName>.+)>d__\d+\/MoveNext\(\)$`)
	localFunctionMethodNameRegex     = regexp.MustCompile(`^(?:.*>g__)?(?P<NestedMethodName>[^|]+)\|`)
	genericClassRegex                = regexp.MustCompile("^(?P<Name>.+)`(?P<Number>\\d+)$")
	nestedTypeSeparatorRegex         = regexp.MustCompile(`[+/]`)
)

// CSharpProcessor implements the language.Processor interface for C#.
type CSharpProcessor struct{}

func NewCSharpProcessor() language.Processor {
	return &CSharpProcessor{}
}

func (p *CSharpProcessor) Name() string {
	return "C#"
}

// Detect checks for C# or F# source files.
func (p *CSharpProcessor) Detect(filePath string) bool {
	lowerPath := strings.ToLower(filePath)
	return strings.HasSuffix(lowerPath, ".cs") || strings.HasSuffix(lowerPath, ".fs")
}

func (p *CSharpProcessor) GetLogicalClassName(rawClassName string) string {
	if i := strings.IndexAny(rawClassName, "/$+"); i != -1 {
		return rawClassName[:i]
	}
	return rawClassName
}

func (p *CSharpProcessor) FormatClassName(class *model.Class) string {
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

func (p *CSharpProcessor) FormatMethodName(method *model.Method, class *model.Class) string {
	methodNamePlusSignature := method.Name + method.Signature
	combinedNameForContext := class.Name + "/" + methodNamePlusSignature

	if strings.Contains(methodNamePlusSignature, "|") {
		if match := localFunctionMethodNameRegex.FindStringSubmatch(methodNamePlusSignature); match != nil {
			if nestedName := findNamedGroup(localFunctionMethodNameRegex, match, "NestedMethodName"); nestedName != "" {
				return nestedName + "()"
			}
		}
	}

	if strings.HasSuffix(methodNamePlusSignature, "MoveNext()") {
		if match := compilerGeneratedMethodNameRegex.FindStringSubmatch(combinedNameForContext); match != nil {
			if compilerGenName := findNamedGroup(compilerGeneratedMethodNameRegex, match, "CompilerGeneratedName"); compilerGenName != "" {
				return compilerGenName + "()"
			}
		}
	}

	return methodNamePlusSignature
}

func (p *CSharpProcessor) CategorizeCodeElement(method *model.Method) model.CodeElementType {
	if strings.HasPrefix(method.DisplayName, "get_") || strings.HasPrefix(method.DisplayName, "set_") {
		return model.PropertyElementType
	}
	return model.MethodElementType
}

func (p *CSharpProcessor) IsCompilerGeneratedClass(class *model.Class) bool {
	rawName := class.Name
	if strings.Contains(rawName, "+<>c") || strings.Contains(rawName, "/<>c") || strings.HasPrefix(rawName, "<>c") || strings.Contains(rawName, ">d__") {
		return true
	}
	return false
}

// CalculateCyclomaticComplexity returns an error indicating this feature is not supported for C#.
func (p *CSharpProcessor) CalculateCyclomaticComplexity(filePath string) ([]model.MethodMetric, error) {
	return nil, language.ErrNotSupported
}

// findNamedGroup is a helper function to extract a named group from a regex match.
func findNamedGroup(re *regexp.Regexp, match []string, groupName string) string {
	for i, name := range re.SubexpNames() {
		if i > 0 && i < len(match) && name == groupName {
			return match[i]
		}
	}
	return ""
}
