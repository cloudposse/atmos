package coverage

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/tools/gotcha/internal/tui"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/config"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/errors"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/types"
)

// ProcessCoverage processes coverage data based on configuration after tests complete.
func ProcessCoverage(coverProfile string, cfg config.CoverageConfig, logger *log.Logger) error {
	if !cfg.Enabled || coverProfile == "" {
		return nil
	}

	// Check if the coverage profile exists
	if _, err := os.Stat(coverProfile); err != nil {
		logger.Warn("Coverage profile not found", "file", coverProfile)
		return nil
	}

	// Parse the coverage profile
	excludeMocks := shouldExcludeMocks(cfg.Analysis.Exclude)
	coverageData, err := ParseCoverageProfile(coverProfile, excludeMocks)
	if err != nil {
		return fmt.Errorf("failed to parse coverage profile: %w", err)
	}

	// Show function coverage if requested
	if cfg.Analysis.Functions {
		if err := showFunctionCoverage(coverageData, cfg, logger); err != nil {
			logger.Warn("Failed to show function coverage", "error", err)
		}
	}

	// Show statement coverage if requested
	if cfg.Analysis.Statements {
		showStatementCoverage(coverageData, cfg, logger)
	}

	// Check thresholds
	if cfg.Thresholds.Total > 0 {
		if err := checkCoverageThresholds(coverageData, cfg.Thresholds, logger); err != nil {
			if cfg.Thresholds.FailUnder {
				return err // Fail the test run
			}
			logger.Warn("Coverage below threshold", "error", err)
		}
	}

	return nil
}

// FunctionCoverageInfo holds parsed coverage information for a function.
type FunctionCoverageInfo struct {
	Package  string
	File     string
	Function string
	Line     int
	Coverage float64
}

// ShowFunctionCoverageReport runs go tool cover -func and displays the output as a tree.
func ShowFunctionCoverageReport(profilePath string, logger *log.Logger) error {
	cmd := exec.Command("go", "tool", "cover", "-func", profilePath)
	output, err := cmd.Output()
	if err != nil {
		// Log the error but don't fail - statement coverage is still valuable
		logger.Debug("Function coverage unavailable", "error", err)
		return nil
	}

	// Parse and display as tree
	functions := parseFunctionCoverageOutput(string(output))
	displayFunctionCoverageTree(functions)
	return nil
}

// parseFunctionCoverageOutput parses the output from go tool cover -func.
func parseFunctionCoverageOutput(output string) []FunctionCoverageInfo {
	var functions []FunctionCoverageInfo
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "total:") {
			continue
		}

		// Parse line format: path/to/file.go:line:\t\tFunctionName\t\tcoverage%
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		// Extract file:line
		// Format is: path/to/file.go:linenum:\t\tfunction\t\tcoverage
		filePart := strings.TrimSuffix(parts[0], ":") // Remove trailing colon
		colonIdx := strings.LastIndex(filePart, ":")
		if colonIdx == -1 {
			continue
		}

		filePath := filePart[:colonIdx]
		lineStr := strings.TrimSpace(filePart[colonIdx+1:])
		lineNum, err := strconv.Atoi(lineStr)
		if err != nil {
			lineNum = 0
		}

		// Extract function name
		functionName := parts[1]

		// Extract coverage percentage
		coverageStr := parts[2]
		coverageStr = strings.TrimSuffix(coverageStr, "%")
		coverage, _ := strconv.ParseFloat(coverageStr, 64)

		// Extract package from path
		pkg := extractPackageFromPath(filePath)

		functions = append(functions, FunctionCoverageInfo{
			Package:  pkg,
			File:     filepath.Base(filePath), // Just the filename
			Function: functionName,
			Line:     lineNum,
			Coverage: coverage,
		})
	}

	return functions
}

// extractPackageFromPath extracts the package name from a file path.
func extractPackageFromPath(path string) string {
	// Remove github.com/cloudposse/atmos/tools/gotcha/ prefix
	const prefix = "github.com/cloudposse/atmos/tools/gotcha/"
	if strings.HasPrefix(path, prefix) {
		path = strings.TrimPrefix(path, prefix)
	}

	// Get directory part (package)
	dir := filepath.Dir(path)
	if dir == "." {
		return "main"
	}
	return dir
}

// getCoverageColor returns the appropriate color for a coverage percentage.
func getCoverageColor(coverage float64) lipgloss.Color {
	if coverage >= 80 {
		return lipgloss.Color("10") // Bright green
	} else if coverage >= 60 {
		return lipgloss.Color("11") // Yellow
	} else if coverage >= 40 {
		return lipgloss.Color("208") // Orange
	} else if coverage > 0 {
		return lipgloss.Color("9") // Bright red
	}
	return lipgloss.Color("245") // Gray for 0%
}

// getCoverageSymbol returns a symbol representing the coverage level.
func getCoverageSymbol(coverage float64) string {
	if coverage >= 80 {
		return "●"
	} else if coverage >= 60 {
		return "◐"
	} else if coverage > 0 {
		return "○"
	}
	return "◌"
}

// displayFunctionCoverageTree displays functions grouped by package in a tree format.
func displayFunctionCoverageTree(functions []FunctionCoverageInfo) {
	if len(functions) == 0 {
		return
	}

	// Group functions by package
	packageGroups := make(map[string][]FunctionCoverageInfo)
	for _, fn := range functions {
		packageGroups[fn.Package] = append(packageGroups[fn.Package], fn)
	}

	// Sort packages
	var packages []string
	for pkg := range packageGroups {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)

	// Define styles
	packageStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")) // Bright blue
	fileStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))              // Gray
	lineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))              // Darker gray
	treeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))              // Very dark gray for tree characters

	// Now we can use the extracted getCoverageColor and getCoverageSymbol functions

	fmt.Printf("\n%s Function Coverage Report\n", tui.CoverageReportIndicator)

	// Calculate totals
	var totalFunctions int
	var totalCoverage float64
	var uncoveredCount int

	for _, pkg := range packages {
		funcs := packageGroups[pkg]

		// Sort functions by file and line
		sort.Slice(funcs, func(i, j int) bool {
			if funcs[i].File != funcs[j].File {
				return funcs[i].File < funcs[j].File
			}
			return funcs[i].Line < funcs[j].Line
		})

		// Calculate package average
		var pkgTotal float64
		for _, fn := range funcs {
			pkgTotal += fn.Coverage
			totalCoverage += fn.Coverage
			totalFunctions++
			if fn.Coverage == 0 {
				uncoveredCount++
			}
		}
		pkgAvg := pkgTotal / float64(len(funcs))

		// Display package header with average
		pkgColor := getCoverageColor(pkgAvg)
		pkgSymbol := lipgloss.NewStyle().Foreground(pkgColor).Render(getCoverageSymbol(pkgAvg))

		// Shorten package path for display
		displayPkg := pkg
		if strings.HasPrefix(pkg, "cmd/") {
			displayPkg = "cmd/" + filepath.Base(strings.TrimPrefix(pkg, "cmd/"))
		} else if strings.HasPrefix(pkg, "internal/") {
			parts := strings.Split(pkg, "/")
			if len(parts) > 2 {
				displayPkg = fmt.Sprintf("internal/%s", parts[1])
			}
		} else if strings.HasPrefix(pkg, "pkg/") {
			parts := strings.Split(pkg, "/")
			if len(parts) > 2 {
				displayPkg = fmt.Sprintf("pkg/%s", parts[1])
			}
		}

		fmt.Printf("%s %s %s\n",
			pkgSymbol,
			packageStyle.Render(displayPkg),
			lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(fmt.Sprintf("(%.1f%%)", pkgAvg)))

		// No need for extra spacing - will be handled by file display

		// Group functions by file (without any line numbers in the key)
		fileGroups := make(map[string][]FunctionCoverageInfo)
		for _, fn := range funcs {
			// Clean up the filename to remove any line numbers that might have leaked in
			cleanFile := fn.File
			if idx := strings.Index(cleanFile, ":"); idx != -1 {
				cleanFile = cleanFile[:idx]
			}
			fileGroups[cleanFile] = append(fileGroups[cleanFile], fn)
		}

		// Sort files
		var files []string
		for file := range fileGroups {
			files = append(files, file)
		}
		sort.Strings(files)

		for fileIdx, file := range files {
			fileFuncs := fileGroups[file]

			// Sort functions by coverage percentage (ascending - lowest coverage first)
			sort.Slice(fileFuncs, func(i, j int) bool {
				return fileFuncs[i].Coverage < fileFuncs[j].Coverage
			})

			// Add spacing before first file
			if fileIdx == 0 {
				fmt.Printf("  %s\n", treeStyle.Render("│"))
			}

			// Determine if this is the last file in package
			isLastFile := fileIdx == len(files)-1
			fileTreeChar := "├─"
			funcPrefix := "│  "
			if isLastFile {
				fileTreeChar = "└─"
				funcPrefix = "   "
			}

			// Display file
			fmt.Printf("  %s %s\n", treeStyle.Render(fileTreeChar), fileStyle.Render(file))

			// Only add vertical connector if there are functions to display
			if len(fileFuncs) > 0 {
				// Add vertical connector line after file name
				if !isLastFile {
					fmt.Printf("  %s\n", treeStyle.Render("│  "))
				} else {
					fmt.Printf("  %s\n", treeStyle.Render("   │"))
				}
			}

			// Display functions with proper indentation
			for i, fn := range fileFuncs {
				coverageColor := getCoverageColor(fn.Coverage)
				coverageStyle := lipgloss.NewStyle().Foreground(coverageColor)

				// Determine tree character
				treeChar := "├──"
				if i == len(fileFuncs)-1 {
					treeChar = "└──"
				}

				// Format function name with padding
				funcName := fn.Function
				if len(funcName) > 28 {
					funcName = funcName[:25] + "..."
				}

				// Display function with line number and aligned coverage on the right
				fmt.Printf("  %s%s %-28s %s %s\n",
					treeStyle.Render(funcPrefix),
					treeStyle.Render(treeChar),
					funcName,
					lineStyle.Render(fmt.Sprintf(":%4d", fn.Line)),
					coverageStyle.Render(fmt.Sprintf("%6.1f%%", fn.Coverage)))
			}

			// Add spacing between files (except for last file)
			if !isLastFile {
				fmt.Printf("  %s\n", treeStyle.Render("│"))
			}
		}
		if len(packages) > 1 && pkg != packages[len(packages)-1] {
			fmt.Println()
		}
	}

	// Display summary
	avgCoverage := totalCoverage / float64(totalFunctions)
	fmt.Printf("\n%s\n", tui.GetDivider())
	fmt.Printf("📈 Summary: %d functions, %.1f%% average coverage, %d uncovered\n",
		totalFunctions, avgCoverage, uncoveredCount)
}

// shouldExcludeMocks checks if mock files should be excluded based on patterns.
func shouldExcludeMocks(excludePatterns []string) bool {
	for _, pattern := range excludePatterns {
		if strings.Contains(pattern, "mock") {
			return true
		}
	}
	return false
}

// showFunctionCoverage displays function coverage based on configuration.
func showFunctionCoverage(data *types.CoverageData, cfg config.CoverageConfig, logger *log.Logger) error {
	functions := data.FunctionCoverage

	// Apply filtering based on config
	if cfg.Analysis.Uncovered {
		functions = filterUncoveredFunctions(functions)
	}

	// Format output based on terminal config
	switch cfg.Output.Terminal.Format {
	case "detailed":
		showDetailedFunctionCoverage(functions, logger)
	case "summary":
		showFunctionCoverageSummary(functions, cfg.Output.Terminal.ShowUncovered, logger)
	case "none":
		// Don't show anything
	default:
		showFunctionCoverageSummary(functions, 5, logger) // Default top 5
	}

	return nil
}

// filterUncoveredFunctions returns only functions with 0% coverage.
func filterUncoveredFunctions(functions []types.CoverageFunction) []types.CoverageFunction {
	var uncovered []types.CoverageFunction
	for _, fn := range functions {
		if fn.Coverage == 0.0 {
			uncovered = append(uncovered, fn)
		}
	}
	return uncovered
}

// showDetailedFunctionCoverage shows all function coverage details.
func showDetailedFunctionCoverage(functions []types.CoverageFunction, logger *log.Logger) {
	if len(functions) == 0 {
		return
	}

	fmt.Printf("\n%s Function Coverage (Detailed):\n", tui.CoverageReportIndicator)
	fmt.Println(tui.GetDivider())

	for _, fn := range functions {
		coverageIcon := "✅"
		if fn.Coverage < 80 {
			coverageIcon = "⚠️"
		}
		if fn.Coverage == 0 {
			coverageIcon = "❌"
		}

		fmt.Printf("%s %-40s %6.1f%%  %s\n",
			coverageIcon,
			truncateString(fn.Function, 40),
			fn.Coverage,
			shortenPath(fn.File))
	}
	fmt.Println(tui.GetDivider())
}

// showFunctionCoverageSummary shows a summary of function coverage.
func showFunctionCoverageSummary(functions []types.CoverageFunction, showUncovered int, logger *log.Logger) {
	if len(functions) == 0 {
		return
	}

	// Calculate statistics
	var totalCoverage float64
	var uncoveredFuncs []types.CoverageFunction

	for _, fn := range functions {
		totalCoverage += fn.Coverage
		if fn.Coverage == 0.0 {
			uncoveredFuncs = append(uncoveredFuncs, fn)
		}
	}

	avgCoverage := totalCoverage / float64(len(functions))

	// Create styles for the summary
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00FFFF")) // Cyan
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#B0B0B0"))             // Light gray
	valueStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))  // White
	coverageStyle := lipgloss.NewStyle().Bold(true).Foreground(getCoverageColor(avgCoverage))
	// Use same white color as Total Functions for consistency, or amber if high count
	uncoveredStyle := valueStyle                // Use same white style as other values for consistency
	if len(uncoveredFuncs) > len(functions)/4 { // If more than 25% uncovered, use amber
		uncoveredStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")) // Yellow/amber
	}

	fmt.Printf("\n%s %s\n", tui.CoverageReportIndicator, headerStyle.Render("Function Coverage Summary:"))
	fmt.Printf("   %s %s\n", labelStyle.Render("Total Functions:"), valueStyle.Render(fmt.Sprintf("%d", len(functions))))
	fmt.Printf("   %s %s\n", labelStyle.Render("Average Coverage:"), coverageStyle.Render(fmt.Sprintf("%.1f%% (avg of functions)", avgCoverage)))
	fmt.Printf("   %s %s\n", labelStyle.Render("Uncovered Functions:"), uncoveredStyle.Render(fmt.Sprintf("%d", len(uncoveredFuncs))))

	// Show top uncovered functions if requested
	if showUncovered > 0 && len(uncoveredFuncs) > 0 {
		fmt.Printf("\n   🔴 Top Uncovered Functions:\n")
		limit := showUncovered
		if limit > len(uncoveredFuncs) {
			limit = len(uncoveredFuncs)
		}

		// Calculate column widths for nice alignment
		maxFuncLen := 0
		for i := 0; i < limit; i++ {
			if len(uncoveredFuncs[i].Function) > maxFuncLen {
				maxFuncLen = len(uncoveredFuncs[i].Function)
			}
		}
		// Cap the max length to prevent too wide columns
		if maxFuncLen > 30 {
			maxFuncLen = 30
		}

		// Display in column format with bullet points for compatibility
		for i := 0; i < limit; i++ {
			funcName := uncoveredFuncs[i].Function
			if len(funcName) > 30 {
				funcName = funcName[:27] + "..."
			}
			// Use padding for alignment while keeping bullet format for tests
			fmt.Printf("      • %-*s in %s\n",
				maxFuncLen,
				funcName,
				shortenPath(uncoveredFuncs[i].File))
		}

		if len(uncoveredFuncs) > limit {
			fmt.Printf("      ... and %d more\n", len(uncoveredFuncs)-limit)
		}
	}
}

// showStatementCoverage displays statement coverage information.
func showStatementCoverage(data *types.CoverageData, cfg config.CoverageConfig, logger *log.Logger) {
	if data.StatementCoverage == "" {
		return
	}

	// Show coverage with mock exclusion info
	if len(data.FilteredFiles) > 0 && cfg.Output.Terminal.Format != "none" {
		fmt.Printf("\n Statement Coverage: %s (excluding %d mocks)\n", data.StatementCoverage, len(data.FilteredFiles))
	} else {
		fmt.Printf("\n Statement Coverage: %s\n", data.StatementCoverage)
	}
}

// checkCoverageThresholds checks if coverage meets configured thresholds.
func checkCoverageThresholds(data *types.CoverageData, thresholds config.CoverageThresholds, logger *log.Logger) error {
	// Extract percentage from statement coverage string (e.g., "85.2%" -> 85.2)
	coverageStr := data.StatementCoverage
	var coverage float64
	if _, err := fmt.Sscanf(coverageStr, "%f%%", &coverage); err != nil {
		logger.Debug("Could not parse coverage percentage", "coverage", coverageStr)
		return nil
	}

	// Check total threshold
	if thresholds.Total > 0 && coverage < thresholds.Total {
		return fmt.Errorf("%w: %.1f%% is below threshold %.1f%%", errors.ErrCoverageBelowThreshold, coverage, thresholds.Total)
	}

	// TODO: Implement per-package threshold checking
	// This would require parsing the coverage profile in more detail

	return nil
}

// Helper functions

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func shortenPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 3 {
		return ".../" + strings.Join(parts[len(parts)-2:], "/")
	}
	return path
}

// OpenBrowser opens the HTML coverage report in the default browser.
func OpenBrowser(htmlPath string, logger *log.Logger) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", htmlPath)
	case "linux":
		cmd = exec.Command("xdg-open", htmlPath)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", htmlPath)
	default:
		return fmt.Errorf("%w: %s", errors.ErrUnsupportedPlatform, runtime.GOOS)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}

	logger.Info("Coverage report opened in browser", "file", htmlPath)
	return nil
}
