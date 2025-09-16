package coverage

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"

	"github.com/cloudposse/atmos/tools/gotcha/internal/tui"
	"github.com/cloudposse/atmos/tools/gotcha/pkg/output"
)

// displayFunctionCoverageTreeNew displays functions grouped by package using lipgloss/tree.
func displayFunctionCoverageTreeNew(functions []FunctionCoverageInfo, writer *output.Writer) {
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
	treeEnumeratorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))      // Dark gray for tree characters

	writer.PrintData("\n%s Function Coverage Report\n", tui.CoverageReportIndicator)

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
		switch {
		case strings.HasPrefix(pkg, "cmd/"):
			displayPkg = "cmd/" + filepath.Base(strings.TrimPrefix(pkg, "cmd/"))
		case strings.HasPrefix(pkg, "internal/"):
			parts := strings.Split(pkg, "/")
			if len(parts) > 2 {
				displayPkg = fmt.Sprintf("internal/%s", parts[1])
			}
		case strings.HasPrefix(pkg, "pkg/"):
			parts := strings.Split(pkg, "/")
			if len(parts) > 2 {
				displayPkg = fmt.Sprintf("pkg/%s", parts[1])
			}
		}

		// Create package header
		writer.PrintData("%s %s %s\n",
			pkgSymbol,
			packageStyle.Render(displayPkg),
			lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(fmt.Sprintf("(%.1f%%)", pkgAvg)))

		// Group functions by file
		fileGroups := make(map[string][]FunctionCoverageInfo)
		for _, fn := range funcs {
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

		// Build tree for this package
		packageTree := tree.New()

		for _, file := range files {
			fileFuncs := fileGroups[file]

			// Sort functions by coverage percentage (ascending - lowest coverage first)
			sort.Slice(fileFuncs, func(i, j int) bool {
				return fileFuncs[i].Coverage < fileFuncs[j].Coverage
			})

			// Create file node with its functions as children
			fileNode := tree.Root(fileStyle.Render(file))

			// Add functions as children
			for _, fn := range fileFuncs {
				coverageColor := getCoverageColor(fn.Coverage)
				coverageStyle := lipgloss.NewStyle().Foreground(coverageColor)

				// Format function name with padding
				funcName := fn.Function
				if len(funcName) > MaxFunctionNameLength {
					funcName = funcName[:FunctionNameTruncation] + "..."
				}

				// Create function display string
				funcDisplay := fmt.Sprintf("%-28s %s %s",
					funcName,
					lineStyle.Render(fmt.Sprintf(":%4d", fn.Line)),
					coverageStyle.Render(fmt.Sprintf("%6.1f%%", fn.Coverage)))

				fileNode = fileNode.Child(funcDisplay)
			}

			packageTree = packageTree.Child(fileNode)
		}

		// Configure and render the tree
		renderedTree := packageTree.
			Enumerator(tree.DefaultEnumerator).
			EnumeratorStyle(treeEnumeratorStyle).
			String()

		// Print the tree with proper indentation
		lines := strings.Split(renderedTree, "\n")
		for _, line := range lines {
			if line != "" {
				writer.PrintData("  %s\n", line)
			}
		}

		if len(packages) > 1 && pkg != packages[len(packages)-1] {
			writer.PrintData("\n")
		}
	}

	// Display summary
	avgCoverage := totalCoverage / float64(totalFunctions)
	writer.PrintData("\n%s\n", tui.GetDivider())
	writer.PrintData("📈 Summary: %d functions, %.1f%% average coverage, %d uncovered\n",
		totalFunctions, avgCoverage, uncoveredCount)
}