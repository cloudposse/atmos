package tests

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/tests/testhelpers"
	"github.com/stretchr/testify/assert"
)

const doubleHyphenWorkDir = "fixtures/scenarios/workflows"

func requireTerraformOrTofu(t *testing.T) {
	t.Helper()

	if _, lookErr := exec.LookPath("tofu"); lookErr != nil {
		if _, lookErr2 := exec.LookPath("terraform"); lookErr2 != nil {
			t.Skip("skipping: neither 'tofu' nor 'terraform' binary found in PATH")
		}
	}
}

func setupDoubleHyphenSandbox(t *testing.T) {
	t.Helper()

	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	t.Setenv("ATMOS_BASE_PATH", "")

	sandbox, err := testhelpers.SetupSandbox(t, doubleHyphenWorkDir)
	if err != nil {
		t.Fatalf("failed to setup sandbox for %q: %v", doubleHyphenWorkDir, err)
	}
	t.Cleanup(sandbox.Cleanup)

	for key, value := range sandbox.GetEnvironmentVariables() {
		t.Setenv(key, value)
	}

	t.Chdir(doubleHyphenWorkDir)
}

// TestDoubleHyphenSeparator tests that args after "--" are correctly passed through
// to the underlying terraform command without being parsed by Atmos.
// This is the fix for GitHub issue #1967.
func TestDoubleHyphenSeparator(t *testing.T) {
	ensureAtmosRunner(t)
	requireTerraformOrTofu(t)

	// Skip if there's a skip reason.
	if skipReason != "" {
		t.Skipf("Skipping test: %s", skipReason)
	}

	tests := []struct {
		name           string
		args           []string
		shouldSucceed  bool
		outputContains string
	}{
		{
			name:           "stack before double-hyphen is parsed correctly",
			args:           []string{"terraform", "plan", "mock", "--stack", "nonprod", "--", "-input=false"},
			shouldSucceed:  false, // May still fail in environments without Terraform state.
			outputContains: `stage           = "nonprod"`,
		},
		{
			name:           "short stack flag before double-hyphen",
			args:           []string{"terraform", "plan", "mock", "-s", "nonprod", "--", "-lock=false"},
			shouldSucceed:  false,
			outputContains: `stage           = "nonprod"`,
		},
		{
			name:           "stack=value syntax before double-hyphen",
			args:           []string{"terraform", "plan", "mock", "--stack=nonprod", "--", "-no-color"},
			shouldSucceed:  false,
			outputContains: `stage           = "nonprod"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupDoubleHyphenSandbox(t)

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
			// We check combined output for stack-specific Terraform output.
			combinedOutput := stdout.String() + stderr.String()
			if tt.outputContains != "" {
				assert.Contains(t, strings.ToLower(combinedOutput), strings.ToLower(tt.outputContains),
					"output should prove the expected stack context")
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
	ensureAtmosRunner(t)
	requireTerraformOrTofu(t)

	if skipReason != "" {
		t.Skipf("Skipping test: %s", skipReason)
	}

	setupDoubleHyphenSandbox(t)

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

	assert.Contains(t, strings.ToLower(combinedOutput), "flag provided but not defined: -consolidate-warnings",
		"terraform should receive the native flag after --")

	// Verify no stack corruption occurred.
	assert.NotContains(t, combinedOutput, "olidate-warnings=false",
		"stack should not be corrupted")
	assert.NotContains(t, combinedOutput, "stack olidate",
		"stack should not contain corrupted value")
}

// TestDoubleHyphenStackNotOverwritten ensures that args after -- don't
// accidentally overwrite the stack value.
func TestDoubleHyphenStackNotOverwritten(t *testing.T) {
	ensureAtmosRunner(t)
	requireTerraformOrTofu(t)

	if skipReason != "" {
		t.Skipf("Skipping test: %s", skipReason)
	}

	setupDoubleHyphenSandbox(t)

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

	assert.Contains(t, strings.ToLower(combinedOutput), "flag provided but not defined: -s",
		"terraform should receive -s after -- as a native argument")

	// Verify the -s after -- didn't change the stack.
	assert.NotContains(t, strings.ToLower(combinedOutput), "switched to workspace \"wrongstack\"",
		"-s after -- should not affect the stack")
}
