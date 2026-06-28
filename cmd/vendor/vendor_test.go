package vendor

import (
	"bytes"
	stdio "io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// TestVendorPullCmd_ExecutorError tests that vendor pull executor handles unexpected args.
func TestVendorPullCmd_ExecutorError(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/terraform-apply-affected"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	err := vendorPullCmd.RunE(vendorPullCmd, []string{"unexpected-arg"})
	assert.Error(t, err, "vendor pull command should return an error with unexpected arguments")
}

// TestVendorCommandProvider tests the VendorCommandProvider interface methods.
func TestVendorCommandProvider(t *testing.T) {
	provider := &VendorCommandProvider{}

	t.Run("GetCommand returns vendorCmd", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.Equal(t, "vendor", cmd.Use)
	})

	t.Run("GetName returns vendor", func(t *testing.T) {
		assert.Equal(t, "vendor", provider.GetName())
	})

	t.Run("GetGroup returns Component Lifecycle", func(t *testing.T) {
		assert.Equal(t, "Component Lifecycle", provider.GetGroup())
	})

	t.Run("GetFlagsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetFlagsBuilder())
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetPositionalArgsBuilder())
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetCompatibilityFlags())
	})

	t.Run("GetAliases returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetAliases())
	})

	t.Run("IsExperimental returns false", func(t *testing.T) {
		assert.False(t, provider.IsExperimental())
	})
}

func writeCommandVendorManifest(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, DefaultVendorManifest)
	require.NoError(t, os.WriteFile(file, []byte(content), 0o644))
	return file
}

type testStreams struct {
	stdin  stdio.Reader
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func (ts *testStreams) Input() stdio.Reader     { return ts.stdin }
func (ts *testStreams) Output() stdio.Writer    { return ts.stdout }
func (ts *testStreams) Error() stdio.Writer     { return ts.stderr }
func (ts *testStreams) RawOutput() stdio.Writer { return ts.stdout }
func (ts *testStreams) RawError() stdio.Writer  { return ts.stderr }

func setupVendorUICapture(t *testing.T) *bytes.Buffer {
	t.Helper()

	stderr := &bytes.Buffer{}
	streams := &testStreams{stdin: &bytes.Buffer{}, stdout: &bytes.Buffer{}, stderr: stderr}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
	t.Cleanup(func() {
		data.Reset()
		ui.Reset()
	})
	return stderr
}

func resetCommandFlags(t *testing.T, cmd *cobra.Command) {
	t.Helper()

	reset := func(flags *pflag.FlagSet) {
		flags.VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	}
	reset(cmd.Flags())
	reset(cmd.PersistentFlags())
	t.Cleanup(func() {
		reset(cmd.Flags())
		reset(cmd.PersistentFlags())
	})
}

func plainOutput(s string) string {
	return ansiEscapeRE.ReplaceAllString(s, "")
}

func TestVendorGetSetCommands_UseFileOverride(t *testing.T) {
	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: ["components/terraform/vpc"]
`)

	oldFileFlag := vendorFileFlag
	vendorFileFlag = file
	t.Cleanup(func() {
		vendorFileFlag = oldFileFlag
		data.Reset()
	})

	require.NoError(t, vendorSetCmd.RunE(vendorSetCmd, []string{"vpc", "v0.2.0"}))
	got, err := vendoring.GetComponentVersion(file, "vpc")
	require.NoError(t, err)
	assert.Equal(t, "v0.2.0", got)

	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	require.NoError(t, vendorGetCmd.RunE(vendorGetCmd, []string{"vpc"}))
}

func TestResolveVendorFileWithOverrideAndDefault(t *testing.T) {
	override := filepath.Join(t.TempDir(), "custom.yaml")
	got, err := resolveVendorFileWithOverride(override)
	require.NoError(t, err)
	assert.Equal(t, override, got)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, DefaultVendorManifest), []byte("spec: {}\n"), 0o644))
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
	require.NoError(t, os.Chdir(dir))
	got, err = resolveVendorFileWithOverride("")
	require.NoError(t, err)
	assert.Equal(t, DefaultVendorManifest, got)
}

func TestResolveVendorFileWithOverride_MissingDefault(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
	require.NoError(t, os.Chdir(dir))

	_, err = resolveVendorFileWithOverride("")
	require.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
}

func TestVendorDiffCommandValidationAndManifestErrors(t *testing.T) {
	resetCommandFlags(t, vendorDiffCmd)

	require.NoError(t, vendorDiffCmd.Flags().Set("component", ""))
	err := vendorDiffCmd.RunE(vendorDiffCmd, nil)
	require.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)

	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: ["components/terraform/vpc"]
`)
	require.NoError(t, vendorDiffCmd.Flags().Set("component", "vpc"))
	require.NoError(t, vendorDiffCmd.Flags().Set("file", file))
	require.NoError(t, vendorDiffCmd.Flags().Set("from", "v0.1.0"))
	require.NoError(t, vendorDiffCmd.Flags().Set("to", "v0.2.0"))

	err = vendorDiffCmd.RunE(vendorDiffCmd, nil)
	require.ErrorIs(t, err, errUtils.ErrVendorSourceNotGit)
}

func TestVendorUpdateCommandSkipsNonGitSources(t *testing.T) {
	resetCommandFlags(t, vendorUpdateCmd)

	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: mock
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: ["components/terraform/mock"]
`)
	require.NoError(t, vendorUpdateCmd.Flags().Set("file", file))
	require.NoError(t, vendorUpdateCmd.Flags().Set("check", "true"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("component", ""))
	require.NoError(t, vendorUpdateCmd.Flags().Set("tags", ""))
	require.NoError(t, vendorUpdateCmd.Flags().Set("pull", "false"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("outdated", "false"))

	require.NoError(t, vendorUpdateCmd.RunE(vendorUpdateCmd, nil))
}

func TestRenderUpdateReport_AllStatuses(t *testing.T) {
	stderr := setupVendorUICapture(t)
	report := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{
		{Component: "updated", Status: vendoring.StatusUpdated, CurrentVersion: "1.0.0", LatestVersion: "1.1.0"},
		{Component: "current", Status: vendoring.StatusUpToDate, CurrentVersion: "1.0.0"},
		{Component: "skipped", Status: vendoring.StatusSkipped, Reason: "not git"},
		{Component: "failed", Status: vendoring.StatusFailed, Reason: "remote failed"},
	}}

	renderUpdateReport(report, false, false)
	got := plainOutput(stderr.String())
	assert.Contains(t, got, "updated")
	assert.Contains(t, got, "1.0.0")
	assert.Contains(t, got, "1.1.0")
	assert.Contains(t, got, "current")
	assert.Contains(t, got, "up to date")
	assert.Contains(t, got, "skipped")
	assert.Contains(t, got, "not git")
	assert.Contains(t, got, "failed")
	assert.Contains(t, got, "remote failed")
	assert.Contains(t, got, "Updated 1 component(s).")

	stderr.Reset()
	renderUpdateReport(report, true, false)
	assert.Contains(t, plainOutput(stderr.String()), "Found 1 update(s) available.")

	stderr.Reset()
	renderUpdateReport(report, false, true)
	got = plainOutput(stderr.String())
	assert.Contains(t, got, "updated")
	assert.NotContains(t, got, "current")
	assert.NotContains(t, got, "skipped")
	assert.NotContains(t, got, "failed")
}
