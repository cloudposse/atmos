package linters

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestLogLevelChecksRule(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantError bool
	}{
		{
			name: "allowed in logger package",
			code: `package logger
func foo(atmosConfig *Config) {
	if atmosConfig.Logs.Level == "Debug" {
		// OK: in logger package
	}
}`,
			wantError: false,
		},
		{
			name: "disallowed comparison with LogLevelTrace",
			code: `package main
import u "utils"
func foo(atmosConfig *Config) {
	if atmosConfig.Logs.Level == u.LogLevelTrace {
		// Not OK: comparing log levels
	}
}`,
			wantError: true,
		},
		{
			name: "disallowed comparison with LogLevelDebug",
			code: `package main
import u "utils"
func foo(atmosConfig *Config) {
	if atmosConfig.Logs.Level == u.LogLevelDebug {
		// Not OK: comparing log levels
	}
}`,
			wantError: true,
		},
		{
			name: "disallowed OR comparison",
			code: `package main
import u "utils"
func foo(atmosConfig *Config) {
	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		// Not OK: comparing log levels
	}
}`,
			wantError: true,
		},
		{
			name: "disallowed access without comparison",
			code: `package main
func foo(atmosConfig *Config) {
	level := atmosConfig.Logs.Level
	// Not OK: accessing log level
}`,
			wantError: true,
		},
		{
			name: "allowed unrelated code",
			code: `package main
func foo(config *Config) {
	if config.Something.Level == "test" {
		// OK: not accessing Logs.Level
	}
}`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test.go", tt.code, parser.AllErrors)
			if err != nil {
				t.Fatalf("Failed to parse test code: %v", err)
			}

			// Track diagnostics reported.
			var diagnostics []analysis.Diagnostic

			// Create a minimal analysis.Pass.
			pass := &analysis.Pass{
				Fset:  fset,
				Files: []*ast.File{file},
				Report: func(d analysis.Diagnostic) {
					diagnostics = append(diagnostics, d)
				},
			}

			rule := &LogLevelChecksRule{}
			err = rule.Check(pass, file)
			if err != nil {
				t.Fatalf("Check returned error: %v", err)
			}

			// Verify diagnostic count matches expectation.
			if tt.wantError {
				if len(diagnostics) == 0 {
					t.Errorf("Expected at least one diagnostic, got none")
				}
			} else {
				if len(diagnostics) > 0 {
					t.Errorf("Expected no diagnostics, got %d:", len(diagnostics))
					for _, d := range diagnostics {
						t.Errorf("  - %s", d.Message)
					}
				}
			}
		})
	}
}
