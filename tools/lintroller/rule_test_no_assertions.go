package linters

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// TestNoAssertionsRule checks for test functions that contain no assertions.
type TestNoAssertionsRule struct{}

func (r *TestNoAssertionsRule) Name() string {
	return "test-no-assertions"
}

func (r *TestNoAssertionsRule) Doc() string {
	return "Checks for test functions with only t.Log() calls and no assertions, or tests that unconditionally skip (provides no coverage value)"
}

func (r *TestNoAssertionsRule) Check(pass *analysis.Pass, file *ast.File) error {
	// Only check test files.
	if !strings.HasSuffix(pass.Fset.File(file.Pos()).Name(), "_test.go") {
		return nil
	}

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		// Only check Test functions (Test*, Benchmark*, Example*).
		if !isTestFunction(funcDecl) {
			return true
		}

		// Check if function body contains only logging and no assertions.
		if hasOnlyLoggingNoAssertions(funcDecl.Body) {
			pass.Reportf(funcDecl.Pos(),
				"Test function '%s' contains only t.Log() calls with no assertions. "+
					"Tests without assertions provide no coverage value and won't catch regressions. "+
					"Either add assertions (t.Error, t.Fatal, assert.*, require.*) or remove the test.",
				funcDecl.Name.Name)
		}

		// Check if function unconditionally skips.
		if skipCall, isUnconditional := hasUnconditionalSkip(funcDecl.Body); isUnconditional {
			pass.Reportf(skipCall.Pos(),
				"Test function '%s' unconditionally skips. "+
					"Tests that always skip provide no coverage value. "+
					"Either fix the test condition or remove the test entirely.",
				funcDecl.Name.Name)
		}

		return true
	})

	return nil
}

// isTestFunction checks if a function is a test, benchmark, or example function.
func isTestFunction(funcDecl *ast.FuncDecl) bool {
	name := funcDecl.Name.Name
	return strings.HasPrefix(name, "Test") ||
		strings.HasPrefix(name, "Benchmark") ||
		strings.HasPrefix(name, "Example")
}

// hasOnlyLoggingNoAssertions checks if a function body contains only logging calls and no assertions.
func hasOnlyLoggingNoAssertions(body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}

	hasLogging := false
	hasAssertion := false

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check for logging calls.
		if isLoggingCall(call) {
			hasLogging = true
			return true
		}

		// Check for assertion calls.
		if isAssertionCall(call) {
			hasAssertion = true
			return false // Stop inspection early if we found an assertion.
		}

		return true
	})

	// Report if function has logging but no assertions.
	return hasLogging && !hasAssertion
}

// isLoggingCall checks if a call is a logging function (t.Log, t.Logf, etc.).
func isLoggingCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	// Check for t.Log, t.Logf, b.Log, b.Logf.
	logMethods := []string{"Log", "Logf"}
	for _, method := range logMethods {
		if sel.Sel.Name == method {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "t" || ident.Name == "b" {
					return true
				}
			}
		}
	}

	return false
}

// isAssertionCall checks if a call is an assertion or test failure function.
func isAssertionCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	// Check for testing.T methods that indicate assertions.
	assertionMethods := []string{
		"Error", "Errorf", "Fatal", "Fatalf", "Fail", "FailNow",
		"Skip", "Skipf", "SkipNow",
	}

	for _, method := range assertionMethods {
		if sel.Sel.Name == method {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "t" || ident.Name == "b" {
					return true
				}
			}
		}
	}

	// Check for testify assert/require calls (assert.*, require.*).
	if ident, ok := sel.X.(*ast.Ident); ok {
		if ident.Name == "assert" || ident.Name == "require" {
			return true
		}
	}

	return false
}

// hasUnconditionalSkip checks if a function unconditionally calls t.Skip/t.Skipf/t.SkipNow.
// Returns the skip call and true if the skip is unconditional, nil and false otherwise.
func hasUnconditionalSkip(body *ast.BlockStmt) (*ast.CallExpr, bool) {
	if body == nil || len(body.List) == 0 {
		return nil, false
	}

	var skipCall *ast.CallExpr

	// Look for skip calls in the function body.
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check if this is a skip call.
		if isSkipCall(call) {
			skipCall = call
			return false // Found one, stop searching.
		}

		return true
	})

	if skipCall == nil {
		return nil, false
	}

	// Check if the skip is unconditional by analyzing if it's always executed.
	return skipCall, isAlwaysExecuted(body, skipCall)
}

// isSkipCall checks if a call is t.Skip, t.Skipf, or t.SkipNow.
func isSkipCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	skipMethods := []string{"Skip", "Skipf", "SkipNow"}
	for _, method := range skipMethods {
		if sel.Sel.Name == method {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "t" || ident.Name == "b" {
					return true
				}
			}
		}
	}

	return false
}

// isAlwaysExecuted checks if a skip call is always executed (not inside if/for/switch/select).
func isAlwaysExecuted(body *ast.BlockStmt, skipCall *ast.CallExpr) bool {
	var isConditional bool

	ast.Inspect(body, func(n ast.Node) bool {
		// If we find the skip call, check if it's inside a conditional structure.
		if n == skipCall {
			return false // Found it, stop searching.
		}

		// Check for conditional structures.
		switch node := n.(type) {
		case *ast.IfStmt:
			// Check if skip is inside this if statement.
			if containsNode(node.Body, skipCall) || (node.Else != nil && containsNode(node.Else, skipCall)) {
				// Check if the condition is obviously true.
				if !isTrueLiteral(node.Cond) {
					isConditional = true
					return false
				}
			}
		case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			// If skip is inside any loop or switch, it's conditional.
			if containsNode(node, skipCall) {
				isConditional = true
				return false
			}
		}

		return true
	})

	return !isConditional
}

// containsNode checks if a parent node contains a child node.
func containsNode(parent, child ast.Node) bool {
	found := false
	ast.Inspect(parent, func(n ast.Node) bool {
		if n == child {
			found = true
			return false
		}
		return true
	})
	return found
}

// isTrueLiteral checks if an expression is the literal value "true".
func isTrueLiteral(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "true"
}
