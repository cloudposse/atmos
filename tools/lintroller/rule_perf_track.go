package linters

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// PerfTrackRule checks for missing defer perf.Track() calls in public functions.
type PerfTrackRule struct{}

// Packages to exclude from perf.Track() checks (avoid infinite recursion or overhead).
var excludedPackages = []string{
	"/logger",      // Avoid infinite recursion.
	"/profiler",    // Profiling code shouldn't track itself.
	"/perf",        // Performance tracking shouldn't track itself.
	"/store",       // Store interfaces have many implementations.
	"/ui/theme",    // UI theme constants and helpers.
	"/ui",          // UI/TUI components and models.
	"/tui",         // Terminal UI components.
}

// Receiver types to exclude from perf.Track() checks.
var excludedReceivers = []string{
	"noopLogger",                  // Noop logger implementations.
	"AtmosLogger",                 // Logger methods would cause infinite recursion.
	"mockPerf",                    // Test mocks.
	"Mock",                        // General mocks.
	"modelSpinner",                // TUI spinner models.
	"modelVendor",                 // TUI vendor models.
	"defaultTemplateRenderer",     // Simple template renderer.
	"realTerraformDocsRunner",     // Simple terraform docs runner.
	"ErrInvalidPattern",           // Error types.
	"DescribeConfigFormatError",   // Error types.
	"DefaultStacksProcessor",      // Processor implementations.
}

func (r *PerfTrackRule) Name() string {
	return "perf-track"
}

func (r *PerfTrackRule) Doc() string {
	return "Checks that public functions have defer perf.Track() calls per coding guidelines"
}

func (r *PerfTrackRule) Check(pass *analysis.Pass, file *ast.File) error {
	filename := pass.Fset.Position(file.Pos()).Filename

	// Skip test files, mock files, and generated files.
	if strings.HasSuffix(filename, "_test.go") ||
		strings.Contains(filename, "mock_") {
		return nil
	}

	// Check if package is in exclusion list.
	pkgPath := pass.Pkg.Path()
	for _, excluded := range excludedPackages {
		if strings.HasSuffix(pkgPath, excluded) {
			return nil
		}
	}

	// Track package name for error messages.
	pkgName := pkgPath

	// Inspect all function declarations.
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Body == nil {
			return true
		}

		// Only check public functions (exported names start with uppercase).
		funcName := funcDecl.Name.Name
		if !token.IsExported(funcName) {
			return true
		}

		// Check if function has defer perf.Track() call.
		hasPerfTrack := false
		if len(funcDecl.Body.List) > 0 {
			// Check first statement for defer perf.Track().
			if deferStmt, ok := funcDecl.Body.List[0].(*ast.DeferStmt); ok {
				if isPerfTrackCall(deferStmt.Call) {
					hasPerfTrack = true
				}
			}
		}

		if !hasPerfTrack {
			// Get receiver type if it's a method.
			receiverType := ""
			if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
				receiverType = formatReceiverType(funcDecl.Recv.List[0].Type)
				// Check if receiver type is in exclusion list.
				for _, excluded := range excludedReceivers {
					if receiverType == excluded || strings.HasPrefix(receiverType, "mock") || strings.HasPrefix(receiverType, "Mock") {
						return true
					}
				}
			}

			// Build suggested function name.
			suggestedName := buildPerfTrackName(pkgName, receiverType, funcName)

			pass.Reportf(funcDecl.Pos(),
				"missing defer perf.Track() call at start of public function %s; add: defer perf.Track(atmosConfig, \"%s\")()",
				funcName, suggestedName)
		}

		return true
	})

	return nil
}

// isPerfTrackCall checks if a call expression is perf.Track().
func isPerfTrackCall(call *ast.CallExpr) bool {
	// Check for perf.Track()().
	outerCall, ok := call.Fun.(*ast.CallExpr)
	if !ok {
		return false
	}

	sel, ok := outerCall.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Track" {
		return false
	}

	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == "perf"
}

// formatReceiverType formats a receiver type expression as a string.
func formatReceiverType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		// Pointer receiver: *Type.
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		// Value receiver: Type.
		return t.Name
	}
	return ""
}

// buildPerfTrackName constructs the suggested perf.Track name.
func buildPerfTrackName(pkgPath, receiverType, funcName string) string {
	// Extract last part of package path (e.g., "github.com/cloudposse/atmos/internal/exec" -> "exec").
	parts := strings.Split(pkgPath, "/")
	pkgName := parts[len(parts)-1]

	if receiverType != "" {
		// Method: "pkg.ReceiverType.FuncName".
		return pkgName + "." + receiverType + "." + funcName
	}

	// Function: "pkg.FuncName".
	return pkgName + "." + funcName
}
