package tests

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// describeIdentityCommandTimeout bounds each subprocess run in this file. These commands
// are local-only (no terraform/network work), so a generous-but-finite timeout turns a
// subprocess hang - e.g. an unexpected outbound call blocking on IMDS/network in an
// environment with no route to it - into a clear, attributable test failure instead of
// hanging the entire test binary until go test's global -timeout kills it and dumps every
// goroutine (see TestDescribeCommandsWithoutAuthWork's "list components" hang).
const describeIdentityCommandTimeout = 30 * time.Second

// runDescribeIdentityCommand runs an atmos command bounded by describeIdentityCommandTimeout
// and returns its combined stdout/stderr and error.
func runDescribeIdentityCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), describeIdentityCommandTimeout)
	defer cancel()

	cmd := atmosRunner.CommandContext(ctx, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	// A deadline-killed command also returns a non-nil error from Run(), which would
	// otherwise satisfy assert.Error in the "should fail gracefully" callers below - letting
	// a hang silently pass as an "expected failure" instead of surfacing as the attributable
	// failure this timeout exists to produce.
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("command %v timed out after %s", args, describeIdentityCommandTimeout)
	}

	return stdout.String() + stderr.String(), err
}

// TestDescribeCommandsWithIdentityFlag verifies that describe commands handle the --identity flag correctly.
// These tests cover the code paths where identity flag is parsed and CreateAuthManagerFromIdentity is called.
func TestDescribeCommandsWithIdentityFlag(t *testing.T) {
	ensureAtmosRunner(t)

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "describe component with non-existent identity should fail gracefully",
			args: []string{"describe", "component", "mycomponent", "--stack", "nonprod", "--identity", "nonexistent-identity"},
		},
		{
			name: "describe stacks with non-existent identity should fail gracefully",
			args: []string{"describe", "stacks", "--identity", "nonexistent-identity"},
		},
		{
			name: "describe affected with non-existent identity should fail gracefully",
			args: []string{"describe", "affected", "--identity", "nonexistent-identity"},
		},
		{
			name: "describe dependents with non-existent identity should fail gracefully",
			args: []string{"describe", "dependents", "mycomponent", "--stack", "nonprod", "--identity", "nonexistent-identity"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir("fixtures/scenarios/basic")

			combinedOutput, err := runDescribeIdentityCommand(t, tt.args...)

			// Should fail when given non-existent identity.
			assert.Error(t, err, "Command should fail with non-existent identity")

			// Should not show interactive selector when explicit identity is provided.
			assert.NotContains(t, combinedOutput, "Select an identity",
				"Should not show interactive selector with explicit identity")
		})
	}

	t.Run("describe component without identity flag should work normally", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/atmos-include-yaml-function")

		_, err := runDescribeIdentityCommand(t, "describe", "component", "component-1", "--stack", "nonprod")

		// Should succeed (component exists in test fixtures).
		assert.NoError(t, err, "describe component without identity should succeed")
	})
}

// TestDescribeCommandsWithoutAuthWork verifies that commands work normally without --identity flag.
func TestDescribeCommandsWithoutAuthWork(t *testing.T) {
	ensureAtmosRunner(t)

	t.Run("describe stacks without auth should work", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		_, err := runDescribeIdentityCommand(t, "describe", "stacks")

		// Should succeed when no identity flag is provided.
		assert.NoError(t, err, "describe stacks without identity flag should succeed")
	})

	t.Run("list components without auth should work", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		_, err := runDescribeIdentityCommand(t, "list", "components")

		// Should succeed when no identity flag is provided.
		assert.NoError(t, err, "list components without identity flag should succeed")
	})
}
