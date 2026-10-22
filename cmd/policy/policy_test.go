package policy

import (
	"bytes"
	"encoding/json"
	"errors"
	stdio "io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

const testPolicyManifest = `apiVersion: atmos/v1
kind: Policy
metadata: {name: commands}
spec:
  policies:
    deny:
      on: command
      action: deny
      rule: "true"
`

type testStreams struct {
	stdout stdio.Writer
	stderr stdio.Writer
}

func (s *testStreams) Input() stdio.Reader     { return &bytes.Buffer{} }
func (s *testStreams) Output() stdio.Writer    { return s.stdout }
func (s *testStreams) Error() stdio.Writer     { return s.stderr }
func (s *testStreams) RawOutput() stdio.Writer { return s.stdout }
func (s *testStreams) RawError() stdio.Writer  { return s.stderr }

// captureData routes the data channel to a buffer for the duration of a test.
func captureData(t *testing.T) *bytes.Buffer {
	t.Helper()
	stdout, _ := captureOutput(t)
	return stdout
}

func captureOutput(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(&testStreams{stdout: stdout, stderr: stderr}))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
	t.Cleanup(data.Reset)
	t.Cleanup(ui.Reset)
	return stdout, stderr
}

// contextCommand returns a command carrying a live context, as cobra provides during execution.
func contextCommand(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	return cmd
}

func decodeResults(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var results []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &results))
	return results
}

// writePolicyDir creates a policy directory holding one valid manifest and points ATMOS_POLICY_DIR at it.
func writePolicyDir(t *testing.T) string {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "policy.d")
	require.NoError(t, os.Mkdir(directory, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(testPolicyManifest), 0o600))
	t.Setenv("ATMOS_POLICY_DIR", directory)
	return directory
}

func TestRunList(t *testing.T) {
	stdout := captureData(t)
	writePolicyDir(t)

	require.NoError(t, runList(nil, nil))
	var rules []map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &rules))
	require.Len(t, rules, 1)
	assert.Contains(t, stdout.String(), "deny")
}

func TestRunCheckFormats(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		args                  []string
		env, fallback, format string
	}{
		{name: "human readable by default"},
		{name: "explicit text", args: []string{"--format", "text"}},
		{name: "explicit json", args: []string{"--format", "json"}, format: "json"},
		{name: "explicit yaml", args: []string{"--format", "yaml"}, format: "yaml"},
		{name: "short flag", args: []string{"-f", "json"}, format: "json"},
		{name: "environment", env: "json", format: "json"},
		{name: "fallback environment", fallback: "yaml", format: "yaml"},
		{name: "canonical environment wins", env: "json", fallback: "yaml", format: "json"},
		{name: "canonical ignores invalid fallback", env: "json", fallback: "invalid", format: "json"},
		{name: "flag overrides environment", args: []string{"--format", "text"}, env: "json", fallback: "yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ATMOS_POLICY_CHECK_FORMAT", tc.env)
			t.Setenv("ATMOS_POLICY_FORMAT", tc.fallback)
			t.Setenv("NO_COLOR", "1")
			stdout, stderr := captureOutput(t)
			writePolicyDir(t)
			cmd := newCheckCommand()
			require.NoError(t, cmd.ParseFlags(tc.args))

			require.NoError(t, cmd.RunE(cmd, nil))

			switch tc.format {
			case "json":
				var checked map[string]any
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &checked))
				assert.Equal(t, map[string]any{"valid": true, "rules": float64(1), "hooks": float64(0)}, checked)
				assert.Empty(t, stderr.String())
			case "yaml":
				var checked map[string]any
				require.NoError(t, yaml.Unmarshal(stdout.Bytes(), &checked))
				assert.Equal(t, map[string]any{"valid": true, "rules": 1, "hooks": 0}, checked)
				assert.Empty(t, stderr.String())
			default:
				assert.Empty(t, stdout.String())
				assert.Contains(t, stderr.String(), "Policy bundle is valid")
				assert.Contains(t, stderr.String(), "Rules: 1")
				assert.Contains(t, stderr.String(), "Hooks: 0")
				assert.NotContains(t, stderr.String(), "\x1b[")
			}
		})
	}
}

func TestRunCheckRejectsInvalidFormat(t *testing.T) {
	for _, source := range []string{"flag", "environment", "fallback environment"} {
		t.Run(source, func(t *testing.T) {
			stdout, stderr := captureOutput(t)
			t.Setenv("ATMOS_POLICY_DIR", filepath.Join(t.TempDir(), "missing"))
			t.Setenv("ATMOS_POLICY_CHECK_FORMAT", "")
			t.Setenv("ATMOS_POLICY_FORMAT", "")
			cmd := newCheckCommand()
			switch source {
			case "flag":
				require.NoError(t, cmd.ParseFlags([]string{"--format", "invalid"}))
			case "environment":
				t.Setenv("ATMOS_POLICY_CHECK_FORMAT", "invalid")
				t.Setenv("ATMOS_POLICY_FORMAT", "json")
			default:
				t.Setenv("ATMOS_POLICY_FORMAT", "invalid")
			}

			require.ErrorIs(t, cmd.RunE(cmd, nil), errUtils.ErrInvalidFlagValue)
			assert.Empty(t, stdout.String())
			assert.Empty(t, stderr.String())
		})
	}
}

func TestCheckIgnoresProjectConfigurationAndGlobalFormat(t *testing.T) {
	stdout, stderr := captureOutput(t)
	writePolicyDir(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte("invalid: [\n"), 0o600))
	t.Chdir(dir)
	t.Setenv("ATMOS_POLICY_CHECK_FORMAT", "json")
	t.Setenv("ATMOS_POLICY_FORMAT", "yaml")
	originalFormat := viper.Get("format")
	viper.Set("format", "invalid-global-format")
	t.Cleanup(func() { viper.Set("format", originalFormat) })

	root := &cobra.Command{Use: "atmos", PersistentPreRunE: func(*cobra.Command, []string) error {
		t.Error("policy inspection must bypass project initialization")
		return errUtils.ErrPolicyInvalid
	}}
	root.AddCommand(newCommand())
	root.SetArgs([]string{"policy", "check"})
	require.NoError(t, root.ExecuteContext(t.Context()))

	var checked map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &checked))
	assert.Equal(t, map[string]any{"valid": true, "rules": float64(1), "hooks": float64(0)}, checked)
	assert.Empty(t, stderr.String())
	assert.Equal(t, "invalid-global-format", viper.GetString("format"))
}

func TestRunSchema(t *testing.T) {
	stdout := captureData(t)

	require.NoError(t, runSchema(nil, []string{"function"}))
	var schema map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &schema))
	assert.NotEmpty(t, schema)

	err := runSchema(nil, []string{"no-such-kind"})
	require.Error(t, err)
}

func TestRunTest(t *testing.T) {
	stdout := captureData(t)
	writePolicyDir(t)
	cases := filepath.Join(t.TempDir(), "cases.json")

	t.Run("matching expectations", func(t *testing.T) {
		require.NoError(t, os.WriteFile(cases, []byte(`[{"name":"deny","request":{"kind":"command","input":{}},"status":"deny"}]`), 0o600))
		stdout.Reset()
		require.NoError(t, runTest(contextCommand(t), []string{cases}))
		results := decodeResults(t, stdout)
		require.Len(t, results, 1)
		assert.Equal(t, true, results[0]["passed"])
		assert.Equal(t, "deny", results[0]["actual"])
	})

	t.Run("failed expectations still print results", func(t *testing.T) {
		require.NoError(t, os.WriteFile(cases, []byte(`[{"name":"deny","request":{"kind":"command","input":{}},"status":"allow"}]`), 0o600))
		stdout.Reset()
		err := runTest(contextCommand(t), []string{cases})
		require.ErrorIs(t, err, errUtils.ErrPolicyEvaluation)
		results := decodeResults(t, stdout)
		require.Len(t, results, 1)
		assert.Equal(t, false, results[0]["passed"])
		assert.Equal(t, "allow", results[0]["expected"])
		assert.Equal(t, "deny", results[0]["actual"])
	})

	t.Run("unreadable cases file prints nothing", func(t *testing.T) {
		stdout.Reset()
		err := runTest(contextCommand(t), []string{filepath.Join(t.TempDir(), "missing.json")})
		require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
		assert.Empty(t, stdout.String())
	})
}

func TestCommandsFailWithoutPolicyDirectory(t *testing.T) {
	captureData(t)
	t.Chdir(t.TempDir())
	t.Setenv("ATMOS_POLICY_DIR", "")

	for name, run := range map[string]func() error{
		"check": func() error { return runCheck(newCheckCommand(), nil) },
		"list":  func() error { return runList(nil, nil) },
		"test":  func() error { return runTest(&cobra.Command{}, []string{"cases.json"}) },
	} {
		t.Run(name, func(t *testing.T) {
			err := run()
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrPolicyInvalid))
			assert.Contains(t, err.Error(), "no policy directory")
		})
	}
}

func TestCommandsFailOnInvalidPolicy(t *testing.T) {
	captureData(t)
	directory := writePolicyDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "bad.yaml"), []byte("kind: Nope"), 0o600))

	for name, run := range map[string]func() error{
		"check": func() error { return runCheck(newCheckCommand(), nil) },
		"list":  func() error { return runList(nil, nil) },
		"test":  func() error { return runTest(&cobra.Command{}, []string{"cases.json"}) },
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, run(), errUtils.ErrPolicyInvalid)
		})
	}
}

// writeInvalidHookPolicyDir installs a loadable bundle whose governing hook fails offline validation.
func writeInvalidHookPolicyDir(t *testing.T) {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "policy.d")
	require.NoError(t, os.Mkdir(directory, 0o700))
	manifest := `apiVersion: atmos/v1
kind: Policy
metadata: {name: lifecycle}
spec:
  hooks:
    lint: {kind: command, command: echo, on_failure: explode}
  policies:
    run-lint:
      on: operation
      action: hooks
      rule: "true"
      hooks: [{ref: lint, event: after.terraform.plan}]
`
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(manifest), 0o600))
	t.Setenv("ATMOS_POLICY_DIR", directory)
}

func TestCommandsRejectInvalidGoverningHooks(t *testing.T) {
	stdout := captureData(t)
	writeInvalidHookPolicyDir(t)
	cases := filepath.Join(t.TempDir(), "cases.json")
	require.NoError(t, os.WriteFile(cases, []byte(`[{"name":"x","request":{"kind":"operation","input":{}},"status":"allow"}]`), 0o600))

	t.Run("check", func(t *testing.T) {
		require.ErrorIs(t, runCheck(newCheckCommand(), nil), errUtils.ErrInvalidHookOnFailure)
		assert.Empty(t, stdout.String(), "an invalid bundle is never reported as valid")
	})

	t.Run("test", func(t *testing.T) {
		require.ErrorIs(t, runTest(contextCommand(t), []string{cases}), errUtils.ErrInvalidHookOnFailure)
		assert.Empty(t, stdout.String(), "fixtures are not run against an invalid bundle")
	})
}

func TestCommandsFailWhenDefaultPolicyDirectoryIsAFile(t *testing.T) {
	captureData(t)
	t.Chdir(t.TempDir())
	t.Setenv("ATMOS_POLICY_DIR", "")
	require.NoError(t, os.WriteFile("policy.d", []byte("not a directory"), 0o600))

	err := runCheck(newCheckCommand(), nil)

	require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
	assert.Contains(t, err.Error(), "not a directory")
}

// failingWriter simulates a closed stdout.
type failingWriter struct{}

var errClosedStdout = errors.New("stdout closed")

func (failingWriter) Write([]byte) (int, error) { return 0, errClosedStdout }

func TestRunTestReportsResultWriteFailure(t *testing.T) {
	ioCtx, err := iolib.NewContext(iolib.WithStreams(&testStreams{stdout: failingWriter{}, stderr: &bytes.Buffer{}}))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)
	writePolicyDir(t)
	cases := filepath.Join(t.TempDir(), "cases.json")
	require.NoError(t, os.WriteFile(cases, []byte(`[{"name":"deny","request":{"kind":"command","input":{}},"status":"deny"}]`), 0o600))

	err = runTest(contextCommand(t), []string{cases})

	require.ErrorIs(t, err, errClosedStdout)
}

func TestIsInspection(t *testing.T) {
	root := &cobra.Command{Use: "atmos"}
	root.AddCommand(command, &cobra.Command{Use: "version"})
	t.Cleanup(func() { root.RemoveCommand(command) })

	assert.True(t, IsInspection(root, []string{"policy"}))
	assert.True(t, IsInspection(root, []string{"policy", "check"}))
	assert.True(t, IsInspection(root, []string{"policy", "schema", "function"}))
	assert.False(t, IsInspection(root, []string{"version"}))
	assert.False(t, IsInspection(root, nil))
	assert.False(t, IsInspection(root, []string{"no-such-command"}), "an unresolvable command is not an inspection")
}

func TestCommandProvider(t *testing.T) {
	provider := &CommandProvider{}
	assert.Same(t, command, provider.GetCommand())
	assert.Equal(t, "policy", provider.GetName())
	assert.NotEmpty(t, provider.GetGroup())
	assert.Nil(t, provider.GetFlagsBuilder())
	assert.Nil(t, provider.GetPositionalArgsBuilder())
	assert.Nil(t, provider.GetCompatibilityFlags())
	assert.Nil(t, provider.GetAliases())
	assert.False(t, provider.IsExperimental())
	require.NoError(t, command.PersistentPreRunE(command, nil))
	assert.Len(t, command.Commands(), 4)
}
