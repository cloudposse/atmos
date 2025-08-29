package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// parseCoverageProfile parses a coverage profile and returns coverage data.
func parseCoverageProfile(profileFile string, excludeMocks bool) (*CoverageData, error) {
	// Validate profile file exists.
	profileFile = filepath.Clean(profileFile)
	if _, err := os.Stat(profileFile); err != nil {
		return nil, fmt.Errorf("coverage profile not found: %w", err)
	}

	statementCoverage, filteredFiles, err := parseStatementCoverage(profileFile, excludeMocks)
	if err != nil {
		return nil, err
	}

	functionCoverage, err := getFunctionCoverage(profileFile, excludeMocks)
	if err != nil {
		return nil, err
	}

	return &CoverageData{
		StatementCoverage: statementCoverage,
		FunctionCoverage:  functionCoverage,
		FilteredFiles:     filteredFiles,
	}, nil
}

// parseStatementCoverage calculates statement coverage from profile.
func parseStatementCoverage(profileFile string, excludeMocks bool) (string, []string, error) {
	file, err := os.Open(profileFile)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()

	var totalStatements, coveredStatements int
	var filteredFiles []string

	scanner := bufio.NewScanner(file)
	// Skip the first line (mode line).
	scanner.Scan()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		coverageLine, err := parseCoverageLine(line)
		if err != nil {
			continue // Skip invalid lines.
		}

		// Check if we should exclude mock files.
		if excludeMocks && isMockFile(coverageLine.Filename) {
			filteredFiles = append(filteredFiles, coverageLine.Filename)
			continue
		}

		totalStatements += coverageLine.Statements
		coveredStatements += coverageLine.Covered
	}

	coverage := calculateStatementCoverage(totalStatements, coveredStatements)
	return coverage, filteredFiles, nil
}

// parseCoverageLine parses a single coverage profile line.
func parseCoverageLine(line string) (*CoverageLine, error) {
	// Parse line: "file:startline.col,endline.col numstmt count".
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return nil, ErrInvalidCoverageLineFormat
	}

	// Extract filename from "file:line.col,line.col".
	colonIndex := strings.Index(parts[0], ":")
	if colonIndex == -1 {
		return nil, ErrInvalidFileFormat
	}
	filename := parts[0][:colonIndex]

	// Parse statement count.
	statements, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, err
	}

	// Parse execution count.
	execCount, err := strconv.Atoi(parts[2])
	if err != nil {
		return nil, err
	}

	// If executed at least once, statements are covered.
	covered := 0
	if execCount > 0 {
		covered = statements
	}

	return &CoverageLine{
		Filename:   filename,
		Statements: statements,
		Covered:    covered,
	}, nil
}

// isMockFile checks if a filename represents a mock file.
func isMockFile(filename string) bool {
	base := filepath.Base(filename)
	dir := filepath.Dir(filename)

	// Check various mock patterns.
	return strings.HasPrefix(base, "mock_") ||
		strings.HasSuffix(base, "_mock.go") ||
		strings.Contains(dir, "/mock/") ||
		strings.Contains(dir, "\\mock\\")
}

// calculateStatementCoverage calculates coverage percentage.
func calculateStatementCoverage(total, covered int) string {
	if total == 0 {
		return "0.0%"
	}
	coverage := float64(covered) / float64(total) * percentageMultiplier
	return fmt.Sprintf("%.1f%%", coverage)
}

// getFunctionCoverage gets function-level coverage information.
func getFunctionCoverage(profileFile string, excludeMocks bool) ([]CoverageFunction, error) {
	// Validate profileFile exists and is a regular file for security.
	if _, err := os.Stat(profileFile); err != nil {
		return nil, fmt.Errorf("invalid profile file: %w", err)
	}

	// Use filepath.Clean to sanitize the path.
	cleanPath := filepath.Clean(profileFile)
	cmd := exec.Command("go", "tool", "cover", "-func="+cleanPath)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get function coverage: %w", err)
	}

	var functions []CoverageFunction
	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "total:") {
			continue
		}

		function, err := parseFunctionCoverageLine(line)
		if err != nil {
			continue // Skip invalid lines.
		}

		// Filter out mock files if requested.
		if excludeMocks && isMockFile(function.File) {
			continue
		}

		functions = append(functions, function)
	}

	return functions, nil
}

// parseFunctionCoverageLine parses a function coverage line.
func parseFunctionCoverageLine(line string) (CoverageFunction, error) {
	// Parse line: "file:line:	function	coverage%".
	// Use regex to extract parts more reliably.
	re := regexp.MustCompile(`^(.+?):\d+:\s+(.+?)\s+(\d+(?:\.\d+)?)%$`)
	matches := re.FindStringSubmatch(line)

	if len(matches) < regexMatchGroups-1 { // We expect 4 groups (including full match).
		return CoverageFunction{}, ErrInvalidFunctionCoverageFormat
	}

	coverage, err := strconv.ParseFloat(matches[3], floatBitSize)
	if err != nil {
		return CoverageFunction{}, err
	}

	return CoverageFunction{
		File:     matches[1],
		Function: matches[2],
		Coverage: coverage,
	}, nil
}
