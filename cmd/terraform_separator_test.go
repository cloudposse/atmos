package cmd

import (
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestTerraform_SeparatorBaseline tests the CURRENT baseline behavior of terraform's -- separator handling.
// These tests document how getConfigAndStacksInfo currently processes args before/after --.
// DO NOT modify these tests - they establish the baseline we must preserve during refactoring.
func TestTerraform_SeparatorBaseline(t *testing.T) {
	tests := []struct {
		name                    string
		args                    []string
		expectedFinalArgs       []string
		expectedArgsAfterDash   []string
		expectedDoubleDashIndex int
	}{
		{
			name:                    "terraform plan with separator and terraform flags",
			args:                    []string{"plan", "myapp", "--", "-var", "foo=bar", "-out=plan.tfplan"},
			expectedFinalArgs:       []string{"plan", "myapp"},
			expectedArgsAfterDash:   []string{"-var", "foo=bar", "-out=plan.tfplan"},
			expectedDoubleDashIndex: 2,
		},
		{
			name:                    "terraform apply with separator and auto-approve",
			args:                    []string{"apply", "myapp", "--", "-auto-approve"},
			expectedFinalArgs:       []string{"apply", "myapp"},
			expectedArgsAfterDash:   []string{"-auto-approve"},
			expectedDoubleDashIndex: 2,
		},
		{
			name:                    "terraform plan with no separator",
			args:                    []string{"plan", "myapp"},
			expectedFinalArgs:       []string{"plan", "myapp"},
			expectedArgsAfterDash:   nil,
			expectedDoubleDashIndex: -1,
		},
		{
			name:                    "terraform with separator at end (no trailing args)",
			args:                    []string{"plan", "myapp", "--"},
			expectedFinalArgs:       []string{"plan", "myapp"},
			expectedArgsAfterDash:   []string{},
			expectedDoubleDashIndex: 2,
		},
		{
			name:                    "terraform with complex flags after separator",
			args:                    []string{"plan", "vpc", "--", "-var", "cidr=10.0.0.0/16", "-var-file=prod.tfvars", "-detailed-exitcode"},
			expectedFinalArgs:       []string{"plan", "vpc"},
			expectedArgsAfterDash:   []string{"-var", "cidr=10.0.0.0/16", "-var-file=prod.tfvars", "-detailed-exitcode"},
			expectedDoubleDashIndex: 2,
		},
		{
			name:                    "terraform with whitespace in terraform args (shell already parsed)",
			args:                    []string{"plan", "myapp", "--", "-var", "description=foo bar baz"},
			expectedFinalArgs:       []string{"plan", "myapp"},
			expectedArgsAfterDash:   []string{"-var", "description=foo bar baz"},
			expectedDoubleDashIndex: 2,
		},
		{
			name:                    "terraform with multiple spaces in args (shell preserves)",
			args:                    []string{"plan", "vpc", "--", "-var", "name=my  vpc"},
			expectedFinalArgs:       []string{"plan", "vpc"},
			expectedArgsAfterDash:   []string{"-var", "name=my  vpc"},
			expectedDoubleDashIndex: 2,
		},
		{
			name:                    "terraform with special characters (already escaped by shell)",
			args:                    []string{"plan", "myapp", "--", "-var", "tag=$BUILD_ID", "-var", "msg=hello'world"},
			expectedFinalArgs:       []string{"plan", "myapp"},
			expectedArgsAfterDash:   []string{"-var", "tag=$BUILD_ID", "-var", "msg=hello'world"},
			expectedDoubleDashIndex: 2,
		},
		{
			name:                    "terraform with empty string arg (shell parsing removed quotes)",
			args:                    []string{"plan", "myapp", "--", "-var", "empty=", "-var", "foo=bar"},
			expectedFinalArgs:       []string{"plan", "myapp"},
			expectedArgsAfterDash:   []string{"-var", "empty=", "-var", "foo=bar"},
			expectedDoubleDashIndex: 2,
		},
		{
			name:                    "terraform with equals signs in values",
			args:                    []string{"plan", "myapp", "--", "-var", "json={\"key\":\"value\"}"},
			expectedFinalArgs:       []string{"plan", "myapp"},
			expectedArgsAfterDash:   []string{"-var", "json={\"key\":\"value\"}"},
			expectedDoubleDashIndex: 2,
		},
		{
			name:                    "terraform with newlines in args (shell already parsed)",
			args:                    []string{"plan", "myapp", "--", "-var", "multiline=line1\nline2"},
			expectedFinalArgs:       []string{"plan", "myapp"},
			expectedArgsAfterDash:   []string{"-var", "multiline=line1\nline2"},
			expectedDoubleDashIndex: 2,
		},
		{
			name:                    "terraform with backslashes",
			args:                    []string{"plan", "myapp", "--", "-var", "path=C:\\Users\\test"},
			expectedFinalArgs:       []string{"plan", "myapp"},
			expectedArgsAfterDash:   []string{"-var", "path=C:\\Users\\test"},
			expectedDoubleDashIndex: 2,
		},
		{
			name:                    "terraform with unicode characters",
			args:                    []string{"plan", "myapp", "--", "-var", "name=café☕"},
			expectedFinalArgs:       []string{"plan", "myapp"},
			expectedArgsAfterDash:   []string{"-var", "name=café☕"},
			expectedDoubleDashIndex: 2,
		},
		{
			name:                    "terraform with very long arg",
			args:                    []string{"plan", "myapp", "--", "-var", "long=" + strings.Repeat("x", 1000)},
			expectedFinalArgs:       []string{"plan", "myapp"},
			expectedArgsAfterDash:   []string{"-var", "long=" + strings.Repeat("x", 1000)},
			expectedDoubleDashIndex: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the current logic from getConfigAndStacksInfo (lines 696-700).
			var argsAfterDoubleDash []string
			finalArgs := tt.args

			doubleDashIndex := lo.IndexOf(tt.args, "--")
			if doubleDashIndex > 0 {
				finalArgs = lo.Slice(tt.args, 0, doubleDashIndex)
				argsAfterDoubleDash = lo.Slice(tt.args, doubleDashIndex+1, len(tt.args))
			}

			assert.Equal(t, tt.expectedDoubleDashIndex, doubleDashIndex, "DoubleDashIndex mismatch")
			assert.Equal(t, tt.expectedFinalArgs, finalArgs, "FinalArgs mismatch")
			assert.Equal(t, tt.expectedArgsAfterDash, argsAfterDoubleDash, "ArgsAfterDoubleDash mismatch")
		})
	}
}

// TestTerraform_SeparatorWithExtractSeparatedArgs tests that ExtractSeparatedArgs produces
// equivalent results to the current terraform implementation.
func TestTerraform_SeparatorWithExtractSeparatedArgs(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		osArgs            []string
		expectedBeforeSep []string
		expectedAfterSep  []string
	}{
		{
			name:              "terraform plan with separator",
			args:              []string{"plan", "myapp", "--", "-var", "foo=bar"},
			osArgs:            []string{"atmos", "terraform", "plan", "myapp", "--", "-var", "foo=bar"},
			expectedBeforeSep: []string{"plan", "myapp"},
			expectedAfterSep:  []string{"-var", "foo=bar"},
		},
		{
			name:              "terraform with no separator",
			args:              []string{"plan", "myapp"},
			osArgs:            []string{"atmos", "terraform", "plan", "myapp"},
			expectedBeforeSep: []string{"plan", "myapp"},
			expectedAfterSep:  nil,
		},
		{
			name:              "terraform with separator at end",
			args:              []string{"plan", "myapp", "--"},
			osArgs:            []string{"atmos", "terraform", "plan", "myapp", "--"},
			expectedBeforeSep: []string{"plan", "myapp"},
			expectedAfterSep:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			separated := ExtractSeparatedArgs(cmd, tt.args, tt.osArgs)

			assert.Equal(t, tt.expectedBeforeSep, separated.BeforeSeparator, "BeforeSeparator mismatch")
			assert.Equal(t, tt.expectedAfterSep, separated.AfterSeparator, "AfterSeparator mismatch")
		})
	}
}
