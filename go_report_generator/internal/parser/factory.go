package parser

import "fmt"

var registeredParsers []IParser

// RegisterParser adds a parser to the list of available parsers.
// This should be called by each parser implementation in its init() function.
func RegisterParser(p IParser) {
	registeredParsers = append(registeredParsers, p)
}

// GetParsers returns all registered parsers.
// (May not be needed externally, but useful for debugging or dynamic selection).
func GetParsers() []IParser {
	return registeredParsers
}

// FindParserForFile attempts to find a suitable parser for the given file.
// It iterates through registered parsers and calls their SupportsFile method.
func FindParserForFile(filePath string) (IParser, error) {
	for _, p := range registeredParsers {
		if p.SupportsFile(filePath) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no suitable parser found for file: %s", filePath)
}
