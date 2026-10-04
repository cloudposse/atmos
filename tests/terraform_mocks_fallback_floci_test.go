package tests

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTerraformComponentMocksFallbackFlociS3 runs `--use-mocks` against a real S3 backend on the
// Floci emulator: a missing state object falls back to the mock, applied state wins in fallback
// mode, `always` ignores it, and an unreachable backend fails with the `--use-mocks=always` hint
// instead of being masked by the mock.
func TestTerraformComponentMocksFallbackFlociS3(t *testing.T) {
	endpoint := requireFlociEndpoint(t, "", "")
	// No terraform/tofu precondition: the emu stack declares opentofu in dependencies.tools, so the
	// Atmos toolchain installs it (the floci-go CI job does not provide a terraform binary).
	ensureFlociAtmosRunner(t)

	workdir := copyFlociScenarioFixture(t, filepath.Join("fixtures", "scenarios", "terraform-component-mocks-fallback"))
	testID := uniqueFlociTestID(t)
	bucket := "mocks-fallback-" + testID
	env := flociHarnessCommandEnv(t, endpoint, workdir, testID, false)
	env["ATMOS_TEST_MOCKS_BUCKET"] = bucket
	createFlociS3Bucket(t, newFlociS3Client(t, endpoint), bucket)

	describeApp := func(t *testing.T, env map[string]string, extraArgs ...string) (string, string, error) {
		t.Helper()
		args := append([]string{"describe", "component", "app", "-s", "emu", "--query", ".vars.inputs", "-f", "json"}, extraArgs...)
		return runFlociAtmosInDir(t, workdir, env, 10*time.Minute, args...)
	}

	t.Run("state object missing falls back to the mock", func(t *testing.T) {
		stdout, stderr, err := describeApp(t, env, "--use-mocks")
		require.NoError(t, err, stderr)
		assert.JSONEq(t, `{"state_vpc_id": "vpc-local", "output_vpc_id": "vpc-local"}`, stdout)
	})

	_, stderr, err := runFlociAtmosInDir(t, workdir, env, 10*time.Minute,
		"terraform", "apply", "vpc", "-s", "emu", "-auto-approve", "-i", "false")
	require.NoError(t, err, stderr)

	t.Run("applied state wins in fallback mode", func(t *testing.T) {
		stdout, stderr, err := describeApp(t, env, "--use-mocks")
		require.NoError(t, err, stderr)
		assert.JSONEq(t, `{"state_vpc_id": "vpc-real", "output_vpc_id": "vpc-real"}`, stdout)
	})

	t.Run("always mode ignores applied state", func(t *testing.T) {
		stdout, stderr, err := describeApp(t, env, "--use-mocks=always")
		require.NoError(t, err, stderr)
		assert.JSONEq(t, `{"state_vpc_id": "vpc-local", "output_vpc_id": "vpc-local"}`, stdout)
	})

	t.Run("unreachable backend fails with a hint instead of using the mock", func(t *testing.T) {
		unreachable := make(map[string]string, len(env)+2)
		for key, value := range env {
			unreachable[key] = value
		}
		// Port 1 is reserved and never listening; one attempt keeps the failure fast.
		unreachable["AWS_ENDPOINT_URL"] = "http://127.0.0.1:1"
		unreachable["AWS_MAX_ATTEMPTS"] = "1"

		stdout, stderr, err := describeApp(t, unreachable, "--use-mocks", "--skip=terraform.output")
		require.Error(t, err, stdout)
		assert.NotContains(t, stdout, "vpc-local")
		assert.Contains(t, stderr, "--use-mocks=always")
	})
}
