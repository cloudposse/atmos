package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
)

// runValidateSchema runs the `atmos validate schema` command in the current working
// directory, dogfooding the real CLI command rather than the schema-validation library.
// It returns the highest exit code requested via errUtils.OsExit (the command signals
// invalid YAML by calling OsExit(1) rather than returning an error) alongside any error
// returned from cmd.Execute(). A non-zero exit code OR a non-nil error means failure.
func runValidateSchema(t *testing.T) (exitCode int, err error) {
	t.Helper()

	original := errUtils.OsExit
	t.Cleanup(func() { errUtils.OsExit = original })
	errUtils.OsExit = func(code int) {
		if code > exitCode {
			exitCode = code
		}
	}

	cmd.RootCmd.SetArgs([]string{"validate", "schema"})
	err = cmd.Execute()
	return exitCode, err
}

// TestTestCaseSchemaValidation proves that the `atmos validate schema` command can
// validate the test-case YAML files (tests/test-cases/*.yaml) against their schema
// (tests/test-cases/schema.json).
//
// It exercises the actual CLI command — declaring a `schemas:` entry in atmos.yaml and
// running `atmos validate schema` — so it demonstrates that Atmos's own validate command
// does the job, not just an embedded JSON Schema library. A negative sub-test confirms the
// command actually rejects a schema-invalid file (i.e., it is not a silent no-op).
func TestTestCaseSchemaValidation(t *testing.T) {
	// Derive the test-cases dir from this source file's location (via runtime.Caller)
	// so the test is independent of the working directory — the subtests below t.Chdir
	// into temp dirs, so a CWD-relative base path would not survive.
	_, callerFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller(0) must succeed")
	testCasesDir, err := filepath.Abs(filepath.Join(filepath.Dir(callerFile), "test-cases"))
	require.NoError(t, err, "Failed to resolve test-cases dir")

	schemaPath := filepath.Join(testCasesDir, "schema.json")
	require.FileExists(t, schemaPath, "test-cases/schema.json must exist")

	files, err := filepath.Glob(filepath.Join(testCasesDir, "*.yaml"))
	require.NoError(t, err, "Failed to find test case files")
	require.NotEmpty(t, files, "No test case YAML files found")

	t.Run("valid/all test cases pass `atmos validate schema`", func(t *testing.T) {
		dir := t.TempDir()
		atmosYAML := "schemas:\n  test-cases:\n    schema: " + schemaPath + "\n" +
			"    matches:\n      - " + filepath.Join(testCasesDir, "*.yaml") + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(atmosYAML), 0o644))

		t.Chdir(dir)
		exitCode, err := runValidateSchema(t)
		require.NoError(t, err, "`atmos validate schema` must not error on valid files")
		require.Zero(t, exitCode,
			"all test-case files must validate against schema.json via `atmos validate schema`")
	})

	t.Run("negative/schema-invalid file is rejected", func(t *testing.T) {
		dir := t.TempDir()
		// `tests` must be a list of test objects per schema.json; a scalar violates it.
		require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.yaml"),
			[]byte("tests: \"must be a list of test objects\"\n"), 0o644))
		atmosYAML := "schemas:\n  test-cases:\n    schema: " + schemaPath + "\n" +
			"    matches:\n      - bad.yaml\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(atmosYAML), 0o644))

		t.Chdir(dir)
		exitCode, err := runValidateSchema(t)
		require.Error(t, err, "`atmos validate schema` must surface an error for a schema-invalid file")
		require.ErrorIs(t, err, exec.ErrInvalidYAML,
			"the failure must be invalid-YAML, not an unrelated CLI/config error")
		require.Equal(t, 1, exitCode,
			"`atmos validate schema` must exit non-zero for a schema-invalid file")
	})
}
