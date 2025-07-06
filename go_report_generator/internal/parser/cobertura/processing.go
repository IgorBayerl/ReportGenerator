package cobertura

import (
	"regexp"
)

// This file only contains the regular expressions specific to the Cobertura format.

// Cobertura-specific Regexes
var (
	// Based on: Palmmedia.ReportGenerator.Core.Parser.CoberturaParser.cs
	// Original C# Regex: private static readonly Regex BranchCoverageRegex = new Regex("\\((?<NumberOfCoveredBranches>\\d+)/(?<NumberOfTotalBranches>\\d+)\\)$", RegexOptions.Compiled);
	conditionCoverageRegexCobertura = regexp.MustCompile(`\((?P<NumberOfCoveredBranches>\d+)/(?P<NumberOfTotalBranches>\d+)\)$`)

	// Based on: Palmmedia.ReportGenerator.Core.Parser.CoberturaParser.cs
	// Original C# Regex: private static readonly Regex LambdaMethodNameRegex = new Regex("<.+>.+__", RegexOptions.Compiled);
	lambdaMethodNameRegexCobertura = regexp.MustCompile(`<.+>.+__`)

	// Based on: Palmmedia.ReportGenerator.Core.Parser.CoberturaParser.cs
	// Original C# Regex: private static readonly Regex CompilerGeneratedMethodNameRegex = new Regex(@"(?<ClassName>.+)(/|\.)<(?<CompilerGeneratedName>.+)>.+__.+MoveNext\(\)$", RegexOptions.Compiled);
	// Go version uses a non-capturing group for the separator: (?:/|\.)
	compilerGeneratedMethodNameRegexCobertura = regexp.MustCompile(`(?P<ClassName>.+)(?:/|\.)<(?P<CompilerGeneratedName>.+)>.+__.+MoveNext\(\)$`)

	// Based on: Palmmedia.ReportGenerator.Core.Parser.CoberturaParser.cs
	// Original C# Regex: private static readonly Regex LocalFunctionMethodNameRegex = new Regex(@"^.*(?<ParentMethodName><.+>).*__(?<NestedMethodName>[^\|]+)\|.*$", RegexOptions.Compiled);
	// Go version is adapted for submatch extraction focusing on NestedMethodName and optionally ParentMethodName.
	localFunctionMethodNameRegexCobertura = regexp.MustCompile(`(?:.*<(?P<ParentMethodName>[^>]+)>g__)?(?P<NestedMethodName>[^|]+)\|`)

	// Based on: Palmmedia.ReportGenerator.Core.Parser.Analysis.Class.cs (GenericClassRegex)
	// Original C# Regex: private static readonly Regex GenericClassRegex = new Regex("^(?<Name>.+)`(?<Number>\\d+)$", RegexOptions.Compiled);
	// Go version uses (?P<Name>...) and (?P<Number>...) for named capture groups.
	genericClassRegexCobertura = regexp.MustCompile("^(?P<Name>.+)`(?P<Number>\\d+)$")

	// This regex is an adaptation of string replacement logic found in C# ReportGenerator (e.g., in OpenCoverParser for FullName).
	// It's used here to normalize nested class separators for display purposes.
	// C# equivalent logic: .Replace('/', '.').Replace('+', '.')
	nestedTypeSeparatorRegexCobertura = regexp.MustCompile(`[+/]`)
)