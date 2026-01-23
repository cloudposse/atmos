package tests

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestDoubleHyphenSeparator tests that args after "--" are correctly passed through
// to the underlying terraform command without being parsed by Atmos.
// This is the fix for GitHub issue #1967.
func TestDoubleHyphenSeparator(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	// Skip if there's a skip reason.
	if skipReason != "" {
		t.Skipf("Skipping test: %s", skipReason)
	}

	// Use t.Setenv for proper test isolation - automatically restored after test.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	t.Setenv("ATMOS_BASE_PATH", "")

	// Use the workflows fixture which has a mock component.
	workDir := "fixtures/scenarios/workflows"
	t.Chdir(workDir)

	tests := []struct {
		name           string
		args           []string
		expectedStack  string
		shouldSucceed  bool
		stderrContains string
	}{
		{
			name:          "stack before double-hyphen is parsed correctly",
			args:          []string{"terraform", "plan", "mock", "--stack", "nonprod", "--", "-input=false"},
			expectedStack: "nonprod",
			shouldSucceed: false, // May still fail due to missing state, but flag parsing succeeds.
			// The important thing is the stack should be "nonprod" not corrupted.
			// Use partial match to handle both "Switched to workspace" and "doesn't exist" messages.
			stderrContains: "workspace \"nonprod\"",
		},
		{
			name:           "short stack flag before double-hyphen",
			args:           []string{"terraform", "plan", "mock", "-s", "nonprod", "--", "-lock=false"},
			expectedStack:  "nonprod",
			shouldSucceed:  false,
			stderrContains: "workspace \"nonprod\"",
		},
		{
			name:           "stack=value syntax before double-hyphen",
			args:           []string{"terraform", "plan", "mock", "--stack=nonprod", "--", "-no-color"},
			expectedStack:  "nonprod",
			shouldSucceed:  false,
			stderrContains: "workspace \"nonprod\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := atmosRunner.Command(tt.args...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()

			// Log output for debugging.
			t.Logf("stdout:\n%s", stdout.String())
			t.Logf("stderr:\n%s", stderr.String())

			if tt.shouldSucceed {
				assert.NoError(t, err, "command should succeed")
			}

			// The key assertion: stack should be correctly parsed.
			// We check combined output for the workspace message (case-insensitive).
			combinedOutput := stdout.String() + stderr.String()
			if tt.stderrContains != "" {
				assert.Contains(t, strings.ToLower(combinedOutput), strings.ToLower(tt.stderrContains),
					"output should mention the expected workspace")
			}

			// Ensure stack is NOT corrupted (the bug from issue #1967).
			// The bug would show the stack as something like "olidate-warnings=false"
			// instead of the actual stack value.
			assert.NotContains(t, combinedOutput, "olidate-warnings",
				"stack should not be corrupted to contain terraform flag fragments")
		})
	}
}

// TestDoubleHyphenWithConsolidateWarnings specifically tests the exact scenario
// from GitHub issue #1967 where -consolidate-warnings=false caused stack corruption.
func TestDoubleHyphenWithConsolidateWarnings(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	if skipReason != "" {
		t.Skipf("Skipping test: %s", skipReason)
	}

	// Use t.Setenv for proper test isolation - automatically restored after test.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	t.Setenv("ATMOS_BASE_PATH", "")

	workDir := "fixtures/scenarios/workflows"
	t.Chdir(workDir)

	// This is the exact command pattern from issue #1967.
	cmd := atmosRunner.Command(
		"terraform", "plan", "mock",
		"--stack", "nonprod",
		"--", "-consolidate-warnings=false",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = cmd.Run() // We expect this to fail because -consolidate-warnings is not a real TF flag.

	combinedOutput := stdout.String() + stderr.String()
	t.Logf("Output:\n%s", combinedOutput)

	// The key assertion: stack should be "nonprod", not corrupted.
	// Before the fix, the stack would be "olidate-warnings=false".
	// Use case-insensitive partial match to handle both "Switched to workspace" and "doesn't exist".
	assert.Contains(t, strings.ToLower(combinedOutput), strings.ToLower("workspace \"nonprod\""),
		"stack should be correctly parsed as 'nonprod'")

	// Verify no stack corruption occurred.
	assert.NotContains(t, combinedOutput, "olidate-warnings=false",
		"stack should not be corrupted")
	assert.NotContains(t, combinedOutput, "stack olidate",
		"stack should not contain corrupted value")
}

// TestDoubleHyphenStackNotOverwritten ensures that args after -- don't
// accidentally overwrite the stack value.
func TestDoubleHyphenStackNotOverwritten(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	if skipReason != "" {
		t.Skipf("Skipping test: %s", skipReason)
	}

	// Use t.Setenv for proper test isolation - automatically restored after test.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	t.Setenv("ATMOS_BASE_PATH", "")

	workDir := "fixtures/scenarios/workflows"
	t.Chdir(workDir)

	// Test that -s after -- doesn't get parsed as the stack flag.
	cmd := atmosRunner.Command(
		"terraform", "plan", "mock",
		"--stack", "nonprod",
		"--", "-s", "wrongstack",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = cmd.Run()

	combinedOutput := stdout.String() + stderr.String()
	t.Logf("Output:\n%s", combinedOutput)

	// Stack should still be "nonprod", not "wrongstack".
	// Use case-insensitive partial match to handle both "Switched to workspace" and "doesn't exist".
	assert.Contains(t, strings.ToLower(combinedOutput), strings.ToLower("workspace \"nonprod\""),
		"stack should remain 'nonprod' even when -s appears after --")

	// Verify the -s after -- didn't change the stack.
	assert.NotContains(t, strings.ToLower(combinedOutput), "switched to workspace \"wrongstack\"",
		"-s after -- should not affect the stack")
}
