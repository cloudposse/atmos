package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExpandTestCaseEnv covers expandTestCaseEnv (tests/cli_test.go), the helper that lets a
// YAML test-case's env: values reference a process env var computed at runtime -- e.g.
// ATMOS_TEST_GITHUB_MOCK_URL, exported by TestMain for the process-wide httpmock GitHub
// facade -- since YAML test-cases otherwise only support static string values.
func TestExpandTestCaseEnv(t *testing.T) {
	t.Setenv("ATMOS_TEST_GITHUB_MOCK_URL", "http://127.0.0.1:54321")
	// expandTestCaseEnv uses os.ExpandEnv, which treats an unset variable the same as an empty
	// one, so t.Setenv("", "") is sufficient here (and self-restoring on test cleanup) --
	// unlike os.Unsetenv, it doesn't depend on the host's environment not already defining
	// this var.
	t.Setenv("THIS_VAR_IS_DEFINITELY_NOT_SET_ANYWHERE", "")

	env := map[string]string{
		"GITHUB_SERVER_URL": "${ATMOS_TEST_GITHUB_MOCK_URL}",
		"GITHUB_API_URL":    "${ATMOS_TEST_GITHUB_MOCK_URL}/api/v3",
		"SHORT_FORM":        "$ATMOS_TEST_GITHUB_MOCK_URL",
		"NO_REFERENCE":      "plain-value",
		"UNSET_VAR":         "${THIS_VAR_IS_DEFINITELY_NOT_SET_ANYWHERE}",
	}

	expandTestCaseEnv(env)

	assert.Equal(t, "http://127.0.0.1:54321", env["GITHUB_SERVER_URL"])
	assert.Equal(t, "http://127.0.0.1:54321/api/v3", env["GITHUB_API_URL"])
	assert.Equal(t, "http://127.0.0.1:54321", env["SHORT_FORM"])
	assert.Equal(t, "plain-value", env["NO_REFERENCE"])
	assert.Empty(t, env["UNSET_VAR"], "an unset variable reference expands to empty, matching os.ExpandEnv")
}

// TestExpandTestCaseEnv_EmptyMap verifies that expandTestCaseEnv is a no-op on an empty map,
// neither panicking nor populating it with entries.
func TestExpandTestCaseEnv_EmptyMap(t *testing.T) {
	env := map[string]string{}
	expandTestCaseEnv(env)
	assert.Empty(t, env)
}
