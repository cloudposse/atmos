package cmd

import (
	"testing"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestHelmfile_SeparatorBaseline tests the CURRENT baseline behavior of helmfile's -- separator handling.
// Helmfile uses getConfigAndStacksInfo just like terraform, so it should have identical separator logic.
// DO NOT modify these tests - they establish the baseline we must preserve during refactoring.
func TestHelmfile_SeparatorBaseline(t *testing.T) {
	tests := []struct {
		name                    string
		args                    []string
		expectedFinalArgs       []string
		expectedArgsAfterDash   []string
		expectedDoubleDashIndex int
	}{
		{
			name: "helmfile apply with separator and helmfile flags",
			args: []string{"apply", "echo-server", "--", "--set", "image.tag=v1.0"},
			expectedFinalArgs: []string{"apply", "echo-server"},
			expectedArgsAfterDash: []string{"--set", "image.tag=v1.0"},
			expectedDoubleDashIndex: 2,
		},
		{
			name: "helmfile sync with separator",
			args: []string{"sync", "myapp", "--", "--environment", "production"},
			expectedFinalArgs: []string{"sync", "myapp"},
			expectedArgsAfterDash: []string{"--environment", "production"},
			expectedDoubleDashIndex: 2,
		},
		{
			name: "helmfile with no separator",
			args: []string{"apply", "echo-server"},
			expectedFinalArgs: []string{"apply", "echo-server"},
			expectedArgsAfterDash: nil,
			expectedDoubleDashIndex: -1,
		},
		{
			name: "helmfile with separator at end",
			args: []string{"diff", "myapp", "--"},
			expectedFinalArgs: []string{"diff", "myapp"},
			expectedArgsAfterDash: []string{},
			expectedDoubleDashIndex: 2,
		},
		{
			name: "helmfile with multiple helmfile flags after separator",
			args: []string{"apply", "webapp", "--", "--selector", "tier=frontend", "--args", "--timeout=600s"},
			expectedFinalArgs: []string{"apply", "webapp"},
			expectedArgsAfterDash: []string{"--selector", "tier=frontend", "--args", "--timeout=600s"},
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

// TestHelmfile_SeparatorWithExtractSeparatedArgs tests that ExtractSeparatedArgs produces
// equivalent results to the current helmfile implementation.
func TestHelmfile_SeparatorWithExtractSeparatedArgs(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		osArgs            []string
		expectedBeforeSep []string
		expectedAfterSep  []string
	}{
		{
			name:   "helmfile apply with separator",
			args:   []string{"apply", "echo-server", "--", "--set", "image.tag=v1.0"},
			osArgs: []string{"atmos", "helmfile", "apply", "echo-server", "--", "--set", "image.tag=v1.0"},
			expectedBeforeSep: []string{"apply", "echo-server"},
			expectedAfterSep:  []string{"--set", "image.tag=v1.0"},
		},
		{
			name:   "helmfile with no separator",
			args:   []string{"apply", "echo-server"},
			osArgs: []string{"atmos", "helmfile", "apply", "echo-server"},
			expectedBeforeSep: []string{"apply", "echo-server"},
			expectedAfterSep:  nil,
		},
		{
			name:   "helmfile with separator at end",
			args:   []string{"diff", "myapp", "--"},
			osArgs: []string{"atmos", "helmfile", "diff", "myapp", "--"},
			expectedBeforeSep: []string{"diff", "myapp"},
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
