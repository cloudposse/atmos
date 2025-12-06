package linters

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// PerfTrackRule checks for missing defer perf.Track() calls in public functions.
type PerfTrackRule struct{}

// Packages to exclude from perf.Track() checks (avoid infinite recursion or overhead).
var excludedPackages = []string{
	"/logger",        // Avoid infinite recursion.
	"/profiler",      // Profiling code shouldn't track itself.
	"/perf",          // Performance tracking shouldn't track itself.
	"/store",         // Store interfaces have many implementations.
	"/ui/theme",      // UI theme constants and helpers.
	"/ui",            // UI/TUI components and models.
	"/tui",           // Terminal UI components.
	"/schema",        // Schema data structures and simple getters/setters.
	"/terminal",      // Terminal utilities and TTY detection.
	"/atmos/errors",  // Error handling utilities to avoid overhead (root errors package only).
	"/filetype",      // Simple file type detection utilities.
	"/cmd/internal",  // Command registry initialization functions.
	"/pkg/utils",     // Simple utility functions (avoid bloat per CLAUDE.md).
	"/pkg/hcl",       // HCL parsing utilities.
	"/downloader",    // File/git downloading infrastructure.
	"/filesystem",    // Low-level filesystem abstraction.
	"/telemetry",     // Telemetry code shouldn't track itself.
	"/xdg",           // XDG directory utilities.
	"/homedir",       // Home directory utilities.
	"/cmd",           // Command providers and root cmd package are just CLI wiring/glue code.
	"/tests",         // Test packages and helpers.
	"/pkg/auth",      // Auth runs once per command, not in hot path.
	"/pkg/config",    // Config loading runs once per command, not in hot path.
	"/pkg/merge",     // Merge happens during config loading, not in hot path.
	"/pkg/list",      // List commands are one-shot operations.
	"/pkg/pager",     // Pager is UI concern, one-shot operation.
	"/pkg/retry",     // Retry logic, not in hot path.
	"/pkg/pro",       // Pro features, not in hot path.
	"/pkg/hooks",     // Hooks run once per command, not in hot path.
	"/pkg/git",       // Git operations are one-shot, not in hot path.
	"/datafetcher",   // Data fetching is one-time operation.
	"/filematch",     // File matching is one-time operation.
	"/pkg/aws",       // AWS operations are one-shot, not in hot path.
	"/pkg/convert",   // Simple conversion utilities.
	"/spinner",       // UI spinner utilities.
	"/vendor",        // Vendor operations are one-shot.
	"/workflow",      // Workflow utilities are one-shot.
	"/mock",          // Mock/test utilities.
	"/pkg/spacelift", // Spacelift generation is one-shot per command.
	"/pkg/validator", // Validation runs once per command.
	"/internal/gcp",  // GCP utilities would create import cycle with pkg/perf (used by pkg/store).
}

// Receiver types to exclude from perf.Track() checks.
var excludedReceivers = []string{
	"noopLogger",                // Noop logger implementations.
	"AtmosLogger",               // Logger methods would cause infinite recursion.
	"mockPerf",                  // Test mocks.
	"Mock",                      // General mocks.
	"modelSpinner",              // TUI spinner models.
	"modelVendor",               // TUI vendor models.
	"defaultTemplateRenderer",   // Simple template renderer.
	"realTerraformDocsRunner",   // Simple terraform docs runner.
	"ErrInvalidPattern",         // Error types.
	"DescribeConfigFormatError", // Error types.
	"DefaultStacksProcessor",    // Processor implementations.
	"AtmosFuncs",                // Template function wrappers (high-frequency).
}

// Functions to exclude from perf.Track() checks (by name).
var excludedFunctions = map[string]string{
	"GetStackNamePattern":           "Simple getter, just returns a field.",
	"FilterComputedFields":          "Data filtering utility, low overhead.",
	"NewFileCopier":                 "Constructor, one-time initialization.",
	"ClearBaseComponentConfigCache": "Test cleanup function.",
	"ClearJsonSchemaCache":          "Test cleanup function.",
	"ClearFileContentCache":         "Test cleanup function.",
}

func (r *PerfTrackRule) Name() string {
	return "perf-track"
}

func (r *PerfTrackRule) Doc() string {
	return "Checks that public functions have defer perf.Track() calls per coding guidelines"
}

func (r *PerfTrackRule) Check(pass *analysis.Pass, file *ast.File) error {
	filename := pass.Fset.Position(file.Pos()).Filename

	// Skip test files, mock files, test helpers, and generated files.
	if strings.HasSuffix(filename, "_test.go") ||
		strings.Contains(filename, "mock_") ||
		strings.HasSuffix(filename, "test_helpers.go") {
		return nil
	}

	// Skip specific utility files that are not in hot paths.
	if strings.HasSuffix(filename, "spinner_utils.go") ||
		strings.HasSuffix(filename, "workflow_utils.go") ||
		strings.HasSuffix(filename, "describe_config.go") ||
		filepath.Base(filename) == "vendor.go" ||
		strings.HasSuffix(filename, "helmfile.go") {
		return nil
	}

	// Check if package is in exclusion list.
	pkgPath := pass.Pkg.Path()
	for _, excluded := range excludedPackages {
		// Match only complete path segments to avoid false positives.
		// e.g., "/errors" should match "pkg/errors" but not "pkg/list/errors".
		if strings.HasSuffix(pkgPath, excluded) || strings.Contains(pkgPath, excluded+"/") {
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
			// Skip functions that are excluded (getters, constructors, test cleanup).
			if _, excluded := excludedFunctions[funcName]; excluded {
				return true
			}

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

			// Check if function has atmosConfig parameter.
			hasAtmosConfig := hasAtmosConfigParam(funcDecl)

			// Build suggested function name.
			suggestedName := buildPerfTrackName(pkgName, receiverType, funcName)

			// Build context-aware suggestion based on whether atmosConfig is available.
			var suggestion string
			if hasAtmosConfig {
				suggestion = fmt.Sprintf("defer perf.Track(atmosConfig, \"%s\")()", suggestedName)
			} else {
				suggestion = fmt.Sprintf("defer perf.Track(nil, \"%s\")()", suggestedName)
			}

			pass.Reportf(funcDecl.Pos(),
				"missing defer perf.Track() call at start of public function %s; add: %s",
				funcName, suggestion)
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

// hasAtmosConfigParam checks if a function has an atmosConfig parameter.
func hasAtmosConfigParam(funcDecl *ast.FuncDecl) bool {
	if funcDecl.Type == nil || funcDecl.Type.Params == nil {
		return false
	}

	for _, param := range funcDecl.Type.Params.List {
		for _, name := range param.Names {
			if name.Name == "atmosConfig" {
				return true
			}
		}
	}

	return false
}
