package cmd

import (
	"testing"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// TestPacker_SeparatorBaseline tests the CURRENT baseline behavior of packer's -- separator handling.
// Packer uses getConfigAndStacksInfo just like terraform and helmfile, so it should have identical separator logic.
// DO NOT modify these tests - they establish the baseline we must preserve during refactoring.
func TestPacker_SeparatorBaseline(t *testing.T) {
	tests := []struct {
		name                    string
		args                    []string
		expectedFinalArgs       []string
		expectedArgsAfterDash   []string
		expectedDoubleDashIndex int
	}{
		{
			name: "packer build with separator and packer flags",
			args: []string{"build", "webapp", "--", "-var", "region=us-east-1"},
			expectedFinalArgs: []string{"build", "webapp"},
			expectedArgsAfterDash: []string{"-var", "region=us-east-1"},
			expectedDoubleDashIndex: 2,
		},
		{
			name: "packer validate with separator",
			args: []string{"validate", "myimage", "--", "-syntax-only"},
			expectedFinalArgs: []string{"validate", "myimage"},
			expectedArgsAfterDash: []string{"-syntax-only"},
			expectedDoubleDashIndex: 2,
		},
		{
			name: "packer with no separator",
			args: []string{"build", "webapp"},
			expectedFinalArgs: []string{"build", "webapp"},
			expectedArgsAfterDash: nil,
			expectedDoubleDashIndex: -1,
		},
		{
			name: "packer with separator at end",
			args: []string{"validate", "myimage", "--"},
			expectedFinalArgs: []string{"validate", "myimage"},
			expectedArgsAfterDash: []string{},
			expectedDoubleDashIndex: 2,
		},
		{
			name: "packer with multiple flags after separator",
			args: []string{"build", "ami", "--", "-var", "instance_type=t3.micro", "-var-file=prod.pkrvars.hcl", "-force"},
			expectedFinalArgs: []string{"build", "ami"},
			expectedArgsAfterDash: []string{"-var", "instance_type=t3.micro", "-var-file=prod.pkrvars.hcl", "-force"},
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

// TestPacker_SeparatorWithExtractSeparatedArgs tests that ExtractSeparatedArgs produces
// equivalent results to the current packer implementation.
func TestPacker_SeparatorWithExtractSeparatedArgs(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		osArgs            []string
		expectedBeforeSep []string
		expectedAfterSep  []string
	}{
		{
			name:   "packer build with separator",
			args:   []string{"build", "webapp", "--", "-var", "region=us-east-1"},
			osArgs: []string{"atmos", "packer", "build", "webapp", "--", "-var", "region=us-east-1"},
			expectedBeforeSep: []string{"build", "webapp"},
			expectedAfterSep:  []string{"-var", "region=us-east-1"},
		},
		{
			name:   "packer with no separator",
			args:   []string{"build", "webapp"},
			osArgs: []string{"atmos", "packer", "build", "webapp"},
			expectedBeforeSep: []string{"build", "webapp"},
			expectedAfterSep:  nil,
		},
		{
			name:   "packer with separator at end",
			args:   []string{"validate", "myimage", "--"},
			osArgs: []string{"atmos", "packer", "validate", "myimage", "--"},
			expectedBeforeSep: []string{"validate", "myimage"},
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
