package filtering

import (
	"fmt"
	"regexp"
	"strings"
)

// IFilter defines an interface for filtering elements.
type IFilter interface {
	IsElementIncludedInReport(name string) bool
	HasCustomFilters() bool
}

// DefaultFilter is the default implementation of IFilter.
type DefaultFilter struct {
	includeFilters []*regexp.Regexp
	excludeFilters []*regexp.Regexp
	hasCustom      bool
}

// NewDefaultFilter creates a new DefaultFilter.
// osIndependantPathSeparator is optional, defaults to false.
func NewDefaultFilter(filters []string, osIndependantPathSeparator ...bool) (IFilter, error) {
	osPathSep := false
	if len(osIndependantPathSeparator) > 0 {
		osPathSep = osIndependantPathSeparator[0]
	}

	df := &DefaultFilter{}
	var errs []string

	for _, f := range filters {
		if strings.HasPrefix(f, "+") {
			re, err := createFilterRegex(f, osPathSep)
			if err != nil {
				errs = append(errs, fmt.Sprintf("invalid include filter '%s': %v", f, err))
				continue
			}
			df.includeFilters = append(df.includeFilters, re)
		} else if strings.HasPrefix(f, "-") {
			re, err := createFilterRegex(f, osPathSep)
			if err != nil {
				errs = append(errs, fmt.Sprintf("invalid exclude filter '%s': %v", f, err))
				continue
			}
			df.excludeFilters = append(df.excludeFilters, re)
		} else if f != "" { // Ignore empty filters, but error on malformed ones
			errs = append(errs, fmt.Sprintf("filter '%s' must start with '+' or '-'", f))
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("error creating default filter: %s", strings.Join(errs, "; "))
	}

	df.hasCustom = len(df.includeFilters) > 0 || len(df.excludeFilters) > 0

	// If no include filters are specified, default to including everything.
	if len(df.includeFilters) == 0 {
		re, _ := createFilterRegex("+*", false) // Default include all
		df.includeFilters = append(df.includeFilters, re)
	}

	return df, nil
}

// IsElementIncludedInReport checks if the given name matches the filter rules.
func (df *DefaultFilter) IsElementIncludedInReport(name string) bool {
	for _, excludeRe := range df.excludeFilters {
		if excludeRe.MatchString(name) {
			return false
		}
	}

	for _, includeRe := range df.includeFilters {
		if includeRe.MatchString(name) {
			return true
		}
	}
	return false
}

// HasCustomFilters returns true if any include or exclude filters were specified.
func (df *DefaultFilter) HasCustomFilters() bool {
	return df.hasCustom
}

// createFilterRegex converts a filter string (e.g., "+MyNamespace.*") to a regex.
// Based on: Palmmedia.ReportGenerator.Core.Parser.Filtering.DefaultFilter.cs (CreateFilterRegex method)
// Original C# logic involves Regex.Escape and specific replacements for '*'
// This Go version uses regexp.QuoteMeta and similar replacements.
func createFilterRegex(filter string, osIndependantPathSeparator bool) (*regexp.Regexp, error) {
	if len(filter) == 0 {
		return nil, fmt.Errorf("empty filter string")
	}
	pattern := filter[1:] // Remove '+' or '-'

	// Escape regex special characters first
	pattern = regexp.QuoteMeta(pattern)

	// Then convert glob-like wildcards '*' and '?'
	// C# original: filter = filter.Replace("*", "$$$*"); ... filter = Regex.Escape(filter); filter = filter.Replace(@"\$\$\$\*", ".*");
	// Go: QuoteMeta escapes '*', so we replace `\*` with `.*`
	pattern = strings.ReplaceAll(pattern, `\*`, ".*")
	pattern = strings.ReplaceAll(pattern, `\?`, ".") // QuoteMeta escapes '?', so replace `\?` with `.`

	if osIndependantPathSeparator {
		// C# original: filter.Replace("/", "$$$pathseparator$$$").Replace("\\", "$$$pathseparator$$$"); ... filter.Replace(@"\$\$\$pathseparator\$\$\$", @"[/\\]");
		// Go: After QuoteMeta, '/' might be unescaped, '\' becomes '\\'.
		// We need to replace them with a regex class that matches either.
		pattern = strings.ReplaceAll(pattern, "/", `[/\\]`)
		pattern = strings.ReplaceAll(pattern, `\\`, `[/\\]`) // For Windows paths that became \\ after QuoteMeta
	}

	return regexp.Compile("(?i)^" + pattern + "$") // Case-insensitive, anchored
}
