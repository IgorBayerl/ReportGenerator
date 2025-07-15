package parser

import "fmt"

// ParserFactory holds a list of available parsers and can find one for a given file.
type ParserFactory struct {
	parsers []IParser
}

// NewParserFactory creates a new factory with a specific list of parsers.
// The parsers are provided explicitly, removing the need for global registration.
func NewParserFactory(parsers ...IParser) *ParserFactory {
	return &ParserFactory{
		parsers: parsers,
	}
}

// FindParserForFile attempts to find a suitable parser for the given file
// from the list of parsers the factory was configured with.
func (f *ParserFactory) FindParserForFile(filePath string) (IParser, error) {
	for _, p := range f.parsers {
		if p.SupportsFile(filePath) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no suitable parser found for file: %s", filePath)
}
