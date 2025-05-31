package filereader

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/IgorBayerl/ReportGenerator/go_report_generator/internal/utils"
	"golang.org/x/text/transform"
)

// CountLinesInFile counts the number of physical lines in a file.
func CountLinesInFile(filePath string) (int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
	}
	return lineCount, scanner.Err()
}

// ReadLinesInFile reads all lines from a file and returns them as a slice of strings.
func ReadLinesInFile(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Attempt to detect encoding
	detectedEncoding, err := utils.DetectEncoding(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not detect encoding for %s: %v. Assuming UTF-8.\n", filePath, err)
	}

	var reader io.Reader = file
	if detectedEncoding != nil {
		// Rewind file to beginning as DetectEncoding reads a few bytes
		_, seekErr := file.Seek(0, io.SeekStart)
		if seekErr != nil {
			return nil, fmt.Errorf("failed to seek file %s after encoding detection: %w", filePath, seekErr)
		}
		decoder := detectedEncoding.NewDecoder()
		reader = transform.NewReader(file, decoder)
	}

	var lines []string
	scanner := bufio.NewScanner(reader) // Use the potentially transformed reader
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
