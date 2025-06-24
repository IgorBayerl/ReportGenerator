// Package glob provides functionality for finding files and directories
// by matching their path names against a pattern. It is a port of
// C#'s Glob.cs (from a project similar to ReportGenerator or its dependencies),
// aiming to support similar globbing features including:
//   - `?`: Matches any single character in a file or directory name.
//   - `*`: Matches zero or more characters in a file or directory name.
//   - `**`: Matches zero or more recursive directories.
//   - `[...]`: Matches a set of characters in a name (e.g., `[abc]`, `[a-z]`).
//   - `{group1,group2,...}`: Matches any of the pattern groups.
//
// Case-insensitivity is the default behavior for matching.
package glob

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var (
	// globCharacters are special characters used in glob patterns.
	globCharacters = []rune{'*', '?', '[', ']', '{', '}'}

	// regexSpecialChars are characters that have special meaning in regular expressions.
	// Used when converting glob patterns to regex to know which characters to escape.
	regexSpecialChars = map[rune]bool{
		'[': true, '\\': true, '^': true, '$': true, '.': true, '|': true,
		'?': true, '*': true, '+': true, '(': true, ')': true, '{': true, '}': true,
	}

	// regexOrStringCache caches compiled regular expressions or literal strings
	// derived from glob pattern segments.
	// Key: pattern string + "|" + case-sensitivity_flag (e.g., "pat?tern*|true")
	regexOrStringCache = make(map[string]*RegexOrString)
	// cacheMutex protects concurrent access to regexOrStringCache.
	cacheMutex = &sync.Mutex{}
)

// RegexOrString holds either a compiled regex for glob matching or a literal string pattern
// if the glob segment contained no wildcards.
type RegexOrString struct {
	// CompiledRegex is the compiled regular expression if the pattern segment contains wildcards.
	CompiledRegex *regexp.Regexp
	// IsRegex indicates if CompiledRegex is used for matching.
	IsRegex bool
	// LiteralPattern is the original glob pattern segment if it's treated as a literal.
	LiteralPattern string
	// IgnoreCase indicates if matching should be case-insensitive for literal patterns.
	IgnoreCase bool
	// OriginalRegexPattern stores the regex string pattern before compilation (for debugging or special checks).
	OriginalRegexPattern string
}

// IsMatch checks if the input string matches this RegexOrString.
// For regex patterns, it uses the compiled regex.
// For literal patterns, it performs a string comparison, respecting IgnoreCase.
func (ros *RegexOrString) IsMatch(input string) bool {
	if ros.IsRegex {
		return ros.CompiledRegex.MatchString(input)
	}
	if ros.IgnoreCase {
		return strings.EqualFold(ros.LiteralPattern, input)
	}
	return ros.LiteralPattern == input
}

// Glob holds the glob pattern and matching options.
type Glob struct {
	// OriginalPattern is the glob pattern string as provided by the user.
	OriginalPattern string
	// IgnoreCase specifies whether path matching should be case-insensitive. Defaults to true.
	IgnoreCase bool
}

// NewGlob creates a new Glob instance with the given pattern.
// By default, IgnoreCase is true, mimicking the C# original behavior.
func NewGlob(pattern string) *Glob {
	return &Glob{
		OriginalPattern: pattern,
		IgnoreCase:      true,
	}
}

// String returns the original pattern string.
func (g *Glob) String() string {
	return g.OriginalPattern
}

// ExpandNames performs a pattern match and returns the matched path names as strings.
// This is equivalent to the C# `Glob.ExpandNames()` which returns `IEnumerable<string>`.
// Paths returned are absolute.
func (g *Glob) ExpandNames() ([]string, error) {
	// In Go, os.FileInfo doesn't store the full path. We will modify `expandInternal`
	// to return full string paths directly.
	return g.expandInternal(g.OriginalPattern, false)
}

// Expand performs a pattern match.
// In C#, this returns IEnumerable<FileSystemInfo>.
// In Go, we will return a slice of full string paths, as constructing FileSystemInfo-like
// objects for each match is less idiomatic and often not what's directly needed.
// If FileSystemInfo-like behavior is truly needed, the caller can os.Stat each path.
// Paths returned are absolute.
func (g *Glob) Expand() ([]string, error) {
	return g.expandInternal(g.OriginalPattern, false)
}

// createRegexOrString compiles a glob pattern segment into a RegexOrString instance.
// It uses a cache to store and retrieve compiled regexes or literal patterns
// to avoid redundant compilations.
func (g *Glob) createRegexOrString(patternSegment string) (*RegexOrString, error) {
	cacheKey := patternSegment + "|" + fmt.Sprintf("%t", g.IgnoreCase)

	cacheMutex.Lock()
	if cached, found := regexOrStringCache[cacheKey]; found {
		cacheMutex.Unlock()
		return cached, nil
	}
	cacheMutex.Unlock()

	// If patternSegment contains no actual glob wildcard characters (excluding '{', '}' which are handled by ungroup),
	// treat it as a literal string for faster matching.
	// C# Glob.cs compiles even literal segments if they are not in cache, but this check can be an optimization.
	if !strings.ContainsAny(patternSegment, "*?[]") {
		ros := &RegexOrString{
			IsRegex:        false,
			LiteralPattern: patternSegment,
			IgnoreCase:     g.IgnoreCase,
		}
		cacheMutex.Lock()
		regexOrStringCache[cacheKey] = ros
		cacheMutex.Unlock()
		return ros, nil
	}

	regexPatternStr, err := globToRegexPattern(patternSegment, g.IgnoreCase)
	if err != nil {
		return nil, fmt.Errorf("failed to convert glob segment '%s' to regex pattern: %w", patternSegment, err)
	}

	re, err := regexp.Compile(regexPatternStr)
	if err != nil {
		// This might happen if globToRegexPattern produces an invalid regex.
		return nil, fmt.Errorf("failed to compile regex '%s' from glob segment '%s': %w", regexPatternStr, patternSegment, err)
	}

	ros := &RegexOrString{
		CompiledRegex:        re,
		IsRegex:              true,
		LiteralPattern:       patternSegment,  // Store original for reference
		IgnoreCase:           g.IgnoreCase,    // Stored in regex via (?i)
		OriginalRegexPattern: regexPatternStr, // Store the regex string itself
	}

	cacheMutex.Lock()
	regexOrStringCache[cacheKey] = ros
	cacheMutex.Unlock()
	return ros, nil
}

// expandInternal is the core recursive matching function.
// It returns a slice of absolute paths for matched files or directories.
// `path` is the current glob pattern being processed.
// `dirOnly` specifies if only directories should be matched.
func (g *Glob) expandInternal(path string, dirOnly bool) ([]string, error) {
	if path == "" {
		return []string{}, nil
	}

	// Optimization: if path has no glob characters and case is ignored (C# behavior).
	// Go's os.Stat is case-sensitive on Unix, case-insensitive on Windows by default.
	// For strict C# IgnoreCase=true mimicry, this direct check needs care.
	// However, if IgnoreCase is true, our RegexOrString will handle case-insensitivity.
	// This optimization is more about avoiding path splitting if it's a literal.
	isLiteralLookup := true
	for _, gc := range globCharacters {
		if strings.ContainsRune(path, gc) {
			isLiteralLookup = false
			break
		}
	}

	if isLiteralLookup {
		// To mimic C# FileSystemInfo.Exists which is case-insensitive on Windows,
		// and to align with g.IgnoreCase=true, we should probably not rely on os.Stat directly
		// if g.IgnoreCase is true for non-Windows.
		// However, if it's truly literal, `os.Stat` is the direct check.
		// If the path must be matched case-insensitively even if literal,
		// we'd need to list parent dir and compare.
		// For this port, if literal, we'll use os.Stat and let OS handle case.
		// The C# code has `if (this.IgnoreCase && path.IndexOfAny(GlobCharacters) < 0)`
		// which implies if IgnoreCase is false, it would still go through the main logic.
		// This seems slightly different than just checking FileSystemInfo.Exists.
		// For now, let's stick to the C# condition:
		if g.IgnoreCase && !strings.ContainsAny(path, string(globCharacters)) {
			absPath, err := filepath.Abs(path)
			if err != nil {
				// Path might be syntactically invalid
				return []string{}, nil
			}
			info, err := os.Stat(absPath)
			if err == nil { // File/Dir exists
				if !dirOnly || info.IsDir() {
					return []string{absPath}, nil
				}
			}
			return []string{}, nil // Does not exist or not matching type
		}
	}

	parent := filepath.Dir(path)
	child := filepath.Base(path)

	// In C#, Path.GetDirectoryName(path) can return null if path is a root (e.g. "C:\").
	// In Go, filepath.Dir("C:\\") is "C:\\"; filepath.Dir("C:") is "."
	// filepath.Dir("/") is "/"; filepath.Dir("foo") is "."
	// If parent is ".", it means child is relative to current working directory OR path was just a filename.
	if parent == "." && !strings.ContainsRune(path, filepath.Separator) { // path was like "file.txt" or "*.txt"
		cwd, _ := os.Getwd()
		parent = cwd
	} else if parent == path { // Happens for root paths like "/" or "C:\"
		// This means `path` is a root. The `child` is `path` itself.
		// We search in this root.
		// If `path` has no glob chars, the optimization above should catch it.
		// If `path` is `C:\*`, parent=`C:\`, child=`*`.
		// We treat `parent` as the directory to list, and `child` as the pattern.
	}

	// Handle C# specific `parent == string.Empty` which translates to current directory.
	// In Go, `filepath.Dir("file.txt")` is ".", so this might be covered.
	// If `path` was "file.txt", parent=".", child="file.txt".
	// If `path` was "/file.txt", parent="/", child="file.txt".
	if parent == "" { // Should not happen with filepath.Dir normally unless input `path` was strange.
		cwd, _ := os.Getwd()
		parent = cwd
	}

	// Handle groups that might span across path separators, e.g., "{a/b,c}/d"
	// The C# code checks `child.Count('}') > child.Count('{')`.
	// This implies that Path.GetFileName might have cut a group in half.
	// Example: path = "foo/{bar,baz/bing}/file.txt"
	// parent = "foo/{bar,baz/bing}" (if GetDirectoryName is smart enough, or if path was foo/{b,b/b} )
	// child = "file.txt"
	// The C# logic is: if `child` looks like an unterminated group (more '}' than '{'),
	// it means the group structure was part of the directory path, so `ungroup` the whole `path`.
	// This is complex to replicate perfectly without knowing how `Path.GetDirectoryName` behaves with such patterns.
	// Let's assume for Go that `filepath.Dir` and `filepath.Base` give sensible splits,
	// and `ungroup` primarily works on individual path segments.
	// The C# check `child.Count(c => c == '}') > child.Count(c => c == '{')` is tricky.
	// It suggests that `path` itself might be like `prefix{a,b}suffix/child_part_of_suffix`.
	// If GetFileName resulted in `child_part_of_suffix}` then the condition is met.
	// This implies `ungroup` should be called on the original `path` if such imbalance in `child` is detected.

	// For simplicity, we'll ungroup `child` first. If `path` itself needs ungrouping due to
	// separators inside braces, `ungroup` should handle that if it's robust enough.
	// The C# code specifically calls `this.Expand(group, dirOnly)` for each `group` from `Ungroup(path)`.
	// This suggests if the brace imbalance is detected, the entire path is re-processed by ungrouping.

	// Simplified check: if path itself contains { and / or \ within the group, it's complex.
	// The C# logic `if (child.Count(c => c == '}') > child.Count(c => c == '{'))` is a heuristic.
	// A more direct way: if Path.GetDirectoryName splits a brace group.
	// For now, we will attempt to handle this by ungrouping `path` if `child` shows imbalance.

	openBracesInChild := strings.Count(child, "{")
	closeBracesInChild := strings.Count(child, "}")

	if closeBracesInChild > openBracesInChild {
		// This indicates that a '}' in the child likely closes a '{' from the parent part.
		// So, the entire path needs to be ungrouped first.
		groups, err := ungroup(path)
		if err != nil {
			return nil, fmt.Errorf("error ungrouping path '%s': %w", path, err)
		}
		var allResults []string
		seenPaths := make(map[string]bool) // To make results distinct like C#'s DistinctBy
		for _, groupPattern := range groups {
			expanded, err := g.expandInternal(groupPattern, dirOnly)
			if err != nil {
				// Log or decide how to handle partial errors
				fmt.Printf("Warning: error expanding group pattern '%s': %v\n", groupPattern, err)
				continue
			}
			for _, p := range expanded {
				absP, _ := filepath.Abs(p) // Ensure distinctness with absolute paths
				if !seenPaths[absP] {
					allResults = append(allResults, absP)
					seenPaths[absP] = true
				}
			}
		}
		return allResults, nil
	}

	if child == "**" {
		// `parent` is the directory from which `**` should start.
		// `expandInternal(parent, true)` gets all directories matching `parent` pattern.
		parentDirs, err := g.expandInternal(parent, true)
		if err != nil {
			return nil, err
		}

		var allResults []string
		seenPaths := make(map[string]bool)

		for _, pDir := range parentDirs {
			// Add the parent directory itself if it matches dirOnly criteria
			// (it's already a dir because expandInternal(parent, true) was called)
			absPDir, _ := filepath.Abs(pDir)
			if !seenPaths[absPDir] {
				allResults = append(allResults, absPDir)
				seenPaths[absPDir] = true
			}

			// Now get all subdirectories recursively
			subItems, err := getRecursiveDirectoriesAndFiles(pDir, dirOnly)
			if err != nil {
				fmt.Printf("Warning: error during '**' recursion for '%s': %v\n", pDir, err)
				continue
			}
			for _, itemPath := range subItems {
				absItemPath, _ := filepath.Abs(itemPath)
				if !seenPaths[absItemPath] {
					allResults = append(allResults, absItemPath)
					seenPaths[absItemPath] = true
				}
			}
		}
		return allResults, nil
	}

	// Expand parent(s) first. This list should contain absolute paths to directories.
	expandedParentDirs, err := g.expandInternal(parent, true)
	if err != nil {
		return nil, err
	}

	// Ungroup the child segment (e.g., "{a,b}.txt" -> "a.txt", "b.txt")
	ungroupedChildSegments, err := ungroup(child)
	if err != nil {
		return nil, fmt.Errorf("error ungrouping child segment '%s': %w", child, err)
	}

	var childRegexes []*RegexOrString
	for _, segment := range ungroupedChildSegments {
		ros, err := g.createRegexOrString(segment)
		if err != nil {
			fmt.Printf("Warning: malformed glob segment '%s' in child, skipping: %v\n", segment, err)
			continue
		}
		childRegexes = append(childRegexes, ros)
	}

	var allMatches []string
	seenPaths := make(map[string]bool) // For DistinctBy equivalent

	for _, parentDir := range expandedParentDirs {
		absParentDir, err := filepath.Abs(parentDir)
		if err != nil { // Should not happen if expandInternal returns abs paths
			fmt.Printf("Warning: could not get absolute path for parentDir '%s', skipping\n", parentDir)
			continue
		}

		// List entries in parentDir
		entries, readDirErr := os.ReadDir(absParentDir)
		if readDirErr != nil {
			if os.IsNotExist(readDirErr) { // Parent dir from a previous glob part might not exist
				continue
			}
			// Log or decide how to handle permission errors, etc.
			fmt.Printf("Warning: error reading directory '%s': %v\n", absParentDir, readDirErr)
			continue
		}

		for _, entry := range entries {
			if !dirOnly || entry.IsDir() { // Check dirOnly constraint
				for _, ros := range childRegexes {
					if ros.IsMatch(entry.Name()) {
						absEntryPath := filepath.Join(absParentDir, entry.Name())
						if !seenPaths[absEntryPath] {
							allMatches = append(allMatches, absEntryPath)
							seenPaths[absEntryPath] = true
						}
						break // Matched one of the child regexes
					}
				}
			}
		}

		// Handle C# Glob.cs specific matching for "." and ".."
		// If childRegexes contains regexes that specifically match "." or "..".
		// `ros.OriginalRegexPattern` stores the regex string (e.g., "^\.$" or "^\.\.$").
		for _, ros := range childRegexes {
			if ros.OriginalRegexPattern == `^\.$` { // Matches "."
				if !seenPaths[absParentDir] { // Yield the parent directory itself
					allMatches = append(allMatches, absParentDir)
					seenPaths[absParentDir] = true
				}
			} else if ros.OriginalRegexPattern == `^\.\.$` { // Matches ".."
				grandParentDir := filepath.Dir(absParentDir)
				// Ensure grandParentDir is not same as absParentDir (e.g., if absParentDir is root)
				if grandParentDir != absParentDir {
					absGrandParentDir, _ := filepath.Abs(grandParentDir)
					if !seenPaths[absGrandParentDir] {
						allMatches = append(allMatches, absGrandParentDir)
						seenPaths[absGrandParentDir] = true
					}
				} else { // absParentDir is likely a root, ".." refers to itself or is invalid
					if !seenPaths[absParentDir] {
						allMatches = append(allMatches, absParentDir)
						seenPaths[absParentDir] = true
					}
				}
			}
		}
	}

	return allMatches, nil
}

// globToRegexPattern converts a glob pattern segment to a Go regular expression string.
// This is used by createRegexOrString.
func globToRegexPattern(globSegment string, ignoreCase bool) (string, error) {
	var regex strings.Builder
	if ignoreCase {
		regex.WriteString("(?i)") // Case-insensitive matching
	}
	regex.WriteRune('^') // Anchor at the beginning

	inCharClass := false
	for _, r := range globSegment {
		if inCharClass {
			if r == ']' {
				inCharClass = false
			}
			// Go's regex engine handles character classes like [abc] or [a-z] directly.
			// Special chars inside [...] usually don't need escaping, except for '-' if not at start/end,
			// and ']' if it's the first char after '['. Go's regexp is mostly POSIX ERE.
			regex.WriteRune(r)
			continue
		}

		switch r {
		case '*':
			regex.WriteString(".*") // Matches zero or more characters
		case '?':
			regex.WriteRune('.') // Matches any single character
		case '[':
			inCharClass = true
			regex.WriteRune(r)
		default:
			if _, isSpecial := regexSpecialChars[r]; isSpecial {
				regex.WriteRune('\\') // Escape regex metacharacters
			}
			regex.WriteRune(r)
		}
	}

	if inCharClass { // Unterminated character class
		return "", fmt.Errorf("unterminated character class in glob segment: %s", globSegment)
	}
	regex.WriteRune('$') // Anchor at the end
	return regex.String(), nil
}

// ungroup handles brace expansion, e.g., "{a,b}c" -> ["ac", "bc"].
// It supports nested braces and multiple groups.
func ungroup(path string) ([]string, error) {
	if !strings.Contains(path, "{") {
		return []string{path}, nil
	}

	// This is a common algorithm for brace expansion:
	// Find the first top-level {...} group.
	// Expand this group.
	// For each expansion, prepend the prefix and recursively call ungroup on the (expansion + suffix).

	var results []string
	level := 0
	firstOpenBrace := -1

	for i, char := range path {
		if char == '{' {
			if level == 0 {
				firstOpenBrace = i
			}
			level++
		} else if char == '}' {
			level--
			if level == 0 && firstOpenBrace != -1 { // Found a top-level group
				prefix := path[:firstOpenBrace]
				groupContent := path[firstOpenBrace+1 : i]
				suffix := path[i+1:]

				var groupParts []string
				partBuilder := strings.Builder{}
				subLevel := 0
				for _, gc := range groupContent {
					if gc == '{' {
						subLevel++
						partBuilder.WriteRune(gc)
					} else if gc == '}' {
						subLevel--
						partBuilder.WriteRune(gc)
					} else if gc == ',' && subLevel == 0 {
						groupParts = append(groupParts, partBuilder.String())
						partBuilder.Reset()
					} else {
						partBuilder.WriteRune(gc)
					}
				}
				groupParts = append(groupParts, partBuilder.String()) // Add the last part

				// Recursively expand suffix first, as it applies to all parts of the current group.
				expandedSuffixes, err := ungroup(suffix)
				if err != nil {
					return nil, err
				}

				for _, gp := range groupParts {
					// Each part of the current group might itself contain groups or be literal.
					// So, we form `prefix + gp` and then expand that, then combine with expandedSuffixes.
					currentCombinedPrefixPart := prefix + gp
					expandedPrefixParts, err := ungroup(currentCombinedPrefixPart)
					if err != nil {
						return nil, err
					}

					for _, epp := range expandedPrefixParts {
						for _, es := range expandedSuffixes {
							results = append(results, epp+es)
						}
					}
				}
				return results, nil // Processed the first top-level group
			}
		}
	}

	if level != 0 { // Unbalanced braces
		return nil, fmt.Errorf("unbalanced braces in pattern: %s", path)
	}

	// No top-level group found (e.g. "abc" or "a{b}c" where {b} was handled by inner recursion)
	return []string{path}, nil
}

// getRecursiveDirectoriesAndFiles is a helper for `**` when it's the last segment.
// It lists the root directory itself and all files/directories under it.
// If dirOnly is true, only directories are returned.
// Returns absolute paths.
func getRecursiveDirectoriesAndFiles(root string, dirOnly bool) ([]string, error) {
	var paths []string
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for '%s': %w", root, err)
	}

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Log or handle errors like permission denied
			fmt.Printf("Warning: error accessing '%s' during recursive search: %v\n", path, err)
			if os.IsPermission(err) { // Skip permission denied errors to continue walk
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			return err // Propagate other errors
		}

		if !dirOnly || d.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking directory '%s': %w", absRoot, err)
	}
	return paths, nil
}

// GetFiles is the public entry point for globbing, analogous to C#'s GlobbingFileSearch.GetFiles.
// It takes a glob pattern and returns a slice of absolute paths to matching files and directories.
// Errors encountered during parts of the expansion (e.g., unreadable directory) are logged as warnings,
// and the function attempts to return successfully found matches.
// A fundamental error (e.g., invalid pattern syntax) will be returned as an error.
func GetFiles(pattern string) ([]string, error) {
	if pattern == "" {
		return []string{}, nil
	}

	g := NewGlob(pattern)
	// Call ExpandNames which uses expandInternal.
	// expandInternal is designed to return errors for fundamental issues.
	return g.ExpandNames()
}
