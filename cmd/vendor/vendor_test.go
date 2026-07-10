package vendor

import (
	"bytes"
	stdio "io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	listpkg "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	"github.com/cloudposse/atmos/pkg/vendoring"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// initVendorTestWriter wires a fresh data writer for RunE calls that write to stdout
// (get/list commands), cleaning up afterward.
func initVendorTestWriter(t *testing.T) {
	t.Helper()
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)
}

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
	path, err := vendoring.ComponentVersionPath(file, "vpc")
	require.NoError(t, err)
	got, err := atmosyaml.GetFile(file, path)
	require.NoError(t, err)
	assert.Equal(t, "v0.2.0", got)

	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	require.NoError(t, vendorGetCmd.RunE(vendorGetCmd, []string{"vpc"}))
}

// captureVendorStdout wires a data writer backed by a buffer so RunE calls
// that write to stdout (get) can be asserted against.
func captureVendorStdout(t *testing.T) *bytes.Buffer {
	t.Helper()

	stdout := &bytes.Buffer{}
	streams := &testStreams{stdin: &bytes.Buffer{}, stdout: stdout, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)
	return stdout
}

// TestVendorGetSetAliasesVendorConfig proves "vendor get/set" are true thin
// aliases of "vendor config get/set": for the same component in the same
// manifest, both command pairs resolve/write the identical value.
func TestVendorGetSetAliasesVendorConfig(t *testing.T) {
	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: eks
      source: github.com/cloudposse/terraform-aws-eks?ref={{.Version}}
      version: v1.0.0
      targets: ["components/terraform/eks"]
    - component: vpc
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: ["components/terraform/vpc"]
`)
	const configPath = "spec.sources[1].version"

	resetCommandFlags(t, vendorGetCmd)
	resetCommandFlags(t, vendorConfigGetCmd)
	resetCommandFlags(t, vendorSetCmd)
	resetCommandFlags(t, vendorConfigSetCmd)
	require.NoError(t, vendorGetCmd.Flags().Set("file", file))
	require.NoError(t, vendorConfigGetCmd.Flags().Set("file", file))
	require.NoError(t, vendorSetCmd.Flags().Set("file", file))
	require.NoError(t, vendorConfigSetCmd.Flags().Set("file", file))

	// get: "vendor get vpc" and "vendor config get spec.sources[1].version"
	// must read the identical value.
	flatOut := captureVendorStdout(t)
	require.NoError(t, vendorGetCmd.RunE(vendorGetCmd, []string{"vpc"}))
	configOut := captureVendorStdout(t)
	require.NoError(t, vendorConfigGetCmd.RunE(vendorConfigGetCmd, []string{configPath}))
	assert.Equal(t, "v0.1.0", strings.TrimSpace(plainOutput(configOut.String())))
	assert.Equal(t, strings.TrimSpace(plainOutput(configOut.String())), strings.TrimSpace(plainOutput(flatOut.String())))

	// set: "vendor set vpc <version>" must write the exact same node that
	// "vendor config set spec.sources[1].version <version>" would.
	setupVendorUICapture(t)
	require.NoError(t, vendorSetCmd.RunE(vendorSetCmd, []string{"vpc", "v9.9.9"}))
	got, err := atmosyaml.GetFile(file, configPath)
	require.NoError(t, err)
	assert.Equal(t, "v9.9.9", got)

	setupVendorUICapture(t)
	require.NoError(t, vendorConfigSetCmd.RunE(vendorConfigSetCmd, []string{configPath, "v10.0.0"}))
	got, err = atmosyaml.GetFile(file, configPath)
	require.NoError(t, err)
	assert.Equal(t, "v10.0.0", got)

	// eks (index 0) must be untouched by either alias.
	eksVersion, err := atmosyaml.GetFile(file, "spec.sources[0].version")
	require.NoError(t, err)
	assert.Equal(t, "v1.0.0", eksVersion)
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

func TestBuildVendorConfigPathRows(t *testing.T) {
	dir := t.TempDir()
	rootFile := filepath.Join(dir, DefaultVendorManifest)
	importFile := filepath.Join(dir, "imports", "common.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(importFile), 0o755))
	require.NoError(t, os.WriteFile(rootFile, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  imports:
    - imports/common.yaml
  sources:
    - component: vpc
      version: v0.1.0
`), 0o644))
	require.NoError(t, os.WriteFile(importFile, []byte(`spec:
  sources:
    - component: eks
      version: v0.2.0
`), 0o644))

	rows, err := buildVendorConfigPathRows(rootFile)
	require.NoError(t, err)
	require.Contains(t, rows, listpkg.PathRow{File: "vendor.yaml", Path: "spec.sources[0].version", Type: "string", Value: "v0.1.0"})
	require.Contains(t, rows, listpkg.PathRow{File: "imports/common.yaml", Path: "spec.sources[0].component", Type: "string", Value: "eks"})

	output, err := listpkg.RenderPathRows(rows, "paths", "")
	require.NoError(t, err)
	require.Contains(t, output, "imports/common.yaml\n  spec\n  spec.sources\n")
	require.Contains(t, output, "vendor.yaml\n  apiVersion\n")
}

func TestFormatVendorConfigFiles_FormatsRootAndImports(t *testing.T) {
	dir := t.TempDir()
	rootFile := filepath.Join(dir, DefaultVendorManifest)
	importFile := filepath.Join(dir, "imports", "common.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(importFile), 0o755))
	require.NoError(t, os.WriteFile(rootFile, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  imports: [imports/common.yaml]
  sources:
    - component: vpc
      version: v0.1.0
`), 0o644))
	require.NoError(t, os.WriteFile(importFile, []byte("spec: {sources: [{component: eks, version: v0.2.0}]}\n"), 0o644))

	files, err := formatVendorConfigFiles(rootFile)
	require.NoError(t, err)
	assert.Equal(t, []string{rootFile, importFile}, files)

	gotRoot, err := os.ReadFile(rootFile)
	require.NoError(t, err)
	assert.NotEmpty(t, gotRoot)
	gotVersion, err := atmosyaml.GetFile(rootFile, "spec.sources[0].version")
	require.NoError(t, err)
	assert.Equal(t, "v0.1.0", gotVersion)
	gotImport, err := os.ReadFile(importFile)
	require.NoError(t, err)
	assert.NotEmpty(t, gotImport)
	gotComponent, err := atmosyaml.GetFile(importFile, "spec.sources[0].component")
	require.NoError(t, err)
	assert.Equal(t, "eks", gotComponent)
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

// TestVendorUpdateCommand_HonorsVendorBasePath reproduces the reported bug: "atmos --chdir=<dir>
// vendor update --check" failed with "No vendor.yaml found in the current directory" whenever the
// target directory's atmos.yaml configured vendor.base_path to something other than a literal
// ./vendor.yaml (e.g. the common infra-live layout), because repo-wide vendor.yaml discovery only
// ever checked the process cwd and ignored atmos.yaml entirely.
func TestVendorUpdateCommand_HonorsVendorBasePath(t *testing.T) {
	resetCommandFlags(t, vendorUpdateCmd)
	chdirTest(t, t.TempDir()) // simulates having --chdir'd into a repo with no ./vendor.yaml.

	vendorDir := filepath.Join(t.TempDir(), "vendor-configs")
	require.NoError(t, os.MkdirAll(vendorDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(vendorDir, DefaultVendorManifest), []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: mock
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: ["components/terraform/mock"]
`), 0o644))
	t.Setenv("ATMOS_VENDOR_BASE_PATH", filepath.Join(vendorDir, DefaultVendorManifest))

	require.NoError(t, vendorUpdateCmd.Flags().Set("check", "true"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("component", ""))
	require.NoError(t, vendorUpdateCmd.Flags().Set("tags", ""))
	require.NoError(t, vendorUpdateCmd.Flags().Set("pull", "false"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("outdated", "false"))

	require.NoError(t, vendorUpdateCmd.RunE(vendorUpdateCmd, nil),
		"vendor update --check must honor atmos.yaml's vendor.base_path instead of only checking cwd")
}

// writeComponentManifestFixture writes a "vpc" component.yaml pinned at v0.1.0 under
// <base>/vpc/component.yaml and points ATMOS_COMPONENTS_TERRAFORM_BASE_PATH at base, so
// DefaultComponentDirResolver resolves it without needing a real atmos.yaml.
func writeComponentManifestFixture(t *testing.T, source string) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH", base)
	componentDir := filepath.Join(base, "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	file := filepath.Join(componentDir, "component.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`apiVersion: atmos/v1
kind: ComponentVendorConfig
spec:
  source:
    uri: "`+source+`"
    version: "v0.1.0"
`), 0o644))
}

// chdirTest switches the working directory for the duration of the test and restores it after.
func chdirTest(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(wd))
	})
}

func TestVendorDiffCommand_FallsBackToComponentManifest(t *testing.T) {
	resetCommandFlags(t, vendorDiffCmd)
	chdirTest(t, t.TempDir()) // no ./vendor.yaml here.
	writeComponentManifestFixture(t, "oci://ghcr.io/cloudposse/mock-component-yaml:{{.Version}}")

	require.NoError(t, vendorDiffCmd.Flags().Set("component", "vpc"))

	err := vendorDiffCmd.RunE(vendorDiffCmd, nil)
	require.ErrorIs(t, err, errUtils.ErrVendorSourceNotGit)
	assert.Contains(t, err.Error(), "mock-component-yaml", "the component.yaml fallback source must be the one diffed")
}

func TestVendorDiffCommand_VendorYamlPrecedence(t *testing.T) {
	resetCommandFlags(t, vendorDiffCmd)
	dir := t.TempDir()
	chdirTest(t, dir)
	writeComponentManifestFixture(t, "oci://ghcr.io/cloudposse/mock-component-yaml:{{.Version}}")
	require.NoError(t, os.WriteFile(filepath.Join(dir, DefaultVendorManifest), []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: oci://ghcr.io/cloudposse/mock-vendor-yaml:{{.Version}}
      version: v0.2.0
      targets: ["components/terraform/vpc"]
`), 0o644))

	require.NoError(t, vendorDiffCmd.Flags().Set("component", "vpc"))

	err := vendorDiffCmd.RunE(vendorDiffCmd, nil)
	require.ErrorIs(t, err, errUtils.ErrVendorSourceNotGit)
	assert.Contains(t, err.Error(), "mock-vendor-yaml", "vendor.yaml must win when it declares the component")
	assert.NotContains(t, err.Error(), "mock-component-yaml")
}

func TestVendorDiffCommand_TypeFlagDefaultsToTerraform(t *testing.T) {
	f := vendorDiffCmd.Flags().Lookup("type")
	require.NotNil(t, f)
	assert.Equal(t, "terraform", f.DefValue)
}

func TestVendorDiffCommand_NeitherManifestExists(t *testing.T) {
	resetCommandFlags(t, vendorDiffCmd)
	chdirTest(t, t.TempDir())
	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH", t.TempDir())

	require.NoError(t, vendorDiffCmd.Flags().Set("component", "vpc"))

	err := vendorDiffCmd.RunE(vendorDiffCmd, nil)
	require.ErrorIs(t, err, errUtils.ErrVendorSourceNotFound)
}

func TestVendorUpdateCommand_ComponentManifestFallback(t *testing.T) {
	resetCommandFlags(t, vendorUpdateCmd)
	chdirTest(t, t.TempDir())
	writeComponentManifestFixture(t, "oci://ghcr.io/cloudposse/mock:{{.Version}}")

	require.NoError(t, vendorUpdateCmd.Flags().Set("component", "vpc"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("check", "true"))

	require.NoError(t, vendorUpdateCmd.RunE(vendorUpdateCmd, nil))
}

func TestVendorUpdateCommand_NeitherManifestExists_ReturnsError(t *testing.T) {
	resetCommandFlags(t, vendorUpdateCmd)
	chdirTest(t, t.TempDir())
	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH", t.TempDir())

	require.NoError(t, vendorUpdateCmd.Flags().Set("component", "vpc"))

	err := vendorUpdateCmd.RunE(vendorUpdateCmd, nil)
	require.ErrorIs(t, err, errUtils.ErrVendorSourceNotFound)
}

// TestVendorUpdateCommand_ErrorsWhenNothingToUpdate is a regression test: a --component-less
// "vendor update" in a repo with neither a vendor.yaml nor any component.yaml manifests anywhere
// must still return a helpful error, rather than silently doing nothing.
func TestVendorUpdateCommand_ErrorsWhenNothingToUpdate(t *testing.T) {
	resetCommandFlags(t, vendorUpdateCmd)
	chdirTest(t, t.TempDir())
	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH", t.TempDir())
	t.Setenv("ATMOS_COMPONENTS_HELMFILE_BASE_PATH", t.TempDir())
	t.Setenv("ATMOS_COMPONENTS_PACKER_BASE_PATH", t.TempDir())

	err := vendorUpdateCmd.RunE(vendorUpdateCmd, nil)
	require.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
}

// TestVendorUpdateCommand_AutoSweepsComponentManifestsWithoutVendorYaml proves a component.yaml-only
// repo (no vendor.yaml at all) is a valid, successful repo-wide sweep with no extra flag required —
// this is the exact scenario reported: `atmos --chdir=<repo> vendor update --check` in a repo that
// vendors exclusively via per-component component.yaml manifests.
func TestVendorUpdateCommand_AutoSweepsComponentManifestsWithoutVendorYaml(t *testing.T) {
	resetCommandFlags(t, vendorUpdateCmd)
	chdirTest(t, t.TempDir())
	writeComponentManifestFixture(t, "oci://ghcr.io/cloudposse/mock:{{.Version}}")

	require.NoError(t, vendorUpdateCmd.Flags().Set("check", "true"))

	require.NoError(t, vendorUpdateCmd.RunE(vendorUpdateCmd, nil))
}

// TestVendorUpdateCommand_ComponentManifestsFlag_SweepsWithoutVendorYaml proves --component-manifests
// still opts a component.yaml-only repo (no vendor.yaml at all) into a valid, successful repo-wide
// sweep, matching the automatic-fallback behavior when the flag is omitted.
func TestVendorUpdateCommand_ComponentManifestsFlag_SweepsWithoutVendorYaml(t *testing.T) {
	resetCommandFlags(t, vendorUpdateCmd)
	chdirTest(t, t.TempDir())
	writeComponentManifestFixture(t, "oci://ghcr.io/cloudposse/mock:{{.Version}}")

	require.NoError(t, vendorUpdateCmd.Flags().Set("component-manifests", "true"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("check", "true"))

	require.NoError(t, vendorUpdateCmd.RunE(vendorUpdateCmd, nil))
}

// TestVendorUpdateCommand_ComponentManifestsFlag_CombinesWithVendorYaml proves --component-manifests
// additionally sweeps component.yaml manifests even when a vendor.yaml IS present, for repos that
// mix both manifest styles.
func TestVendorUpdateCommand_ComponentManifestsFlag_CombinesWithVendorYaml(t *testing.T) {
	resetCommandFlags(t, vendorUpdateCmd)
	chdirTest(t, t.TempDir())
	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: eks
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v1.0.0
      targets: ["components/terraform/eks"]
`)
	writeComponentManifestFixture(t, "oci://ghcr.io/cloudposse/mock:{{.Version}}")

	require.NoError(t, vendorUpdateCmd.Flags().Set("file", file))
	require.NoError(t, vendorUpdateCmd.Flags().Set("component-manifests", "true"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("check", "true"))

	require.NoError(t, vendorUpdateCmd.RunE(vendorUpdateCmd, nil))
}

// writeLocalComponentManifestFixture writes a component.yaml for componentName under base (the
// components/terraform directory), pointing its source.uri at a local sourceDir (an absolute path,
// so pkg/utils.JoinPath returns it unchanged regardless of the component's own directory). Because
// uri is a local path (not oci://../git), ExecuteComponentVendorInternal resolves it as pkgTypeLocal
// and copies it directly off disk - no network access, matching this repo's "prefer unit tests with
// mocks/fakes over integration tests" convention (CLAUDE.md) instead of hitting a real Git remote.
// Returns the component's own directory (where component.yaml lives and where the source gets
// copied).
func writeLocalComponentManifestFixture(t *testing.T, base, componentName, sourceDir string) string {
	t.Helper()
	componentDir := filepath.Join(base, componentName)
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	file := filepath.Join(componentDir, "component.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`apiVersion: atmos/v1
kind: ComponentVendorConfig
spec:
  source:
    uri: "`+filepath.ToSlash(sourceDir)+`"
    version: "v0.1.0"
`), 0o644))
	return componentDir
}

// newVendorPullTestCmd builds a throwaway *cobra.Command carrying exactly the flags the
// runVendorPull call chain needs: vendorUpdateCmd's own component/type/everything/stack/tags/dry-run
// flags (which pullUpdatedComponent reads and resets), plus the global flags
// internal/exec.ProcessCommandLineArgs reads directly off cmd.Flags() (base-path, config,
// config-path, profile). The real vendorUpdateCmd can't be used for this in
// `go test ./cmd/vendor/...`: those global flags only exist as RootCmd's persistent flags, which
// live in package `cmd` (the root command package) - a package this test binary never compiles or
// links, since cmd/vendor is a dependency of cmd, not the reverse. This mirrors the same
// throwaway-minimal-cmd pattern used by cmd/emulator/emulator_test.go for the identical reason.
func newVendorPullTestCmd() *cobra.Command {
	c := &cobra.Command{Use: "update"}
	c.Flags().StringP("component", "c", "", "")
	c.Flags().StringP("type", "t", "terraform", "")
	c.Flags().Bool("everything", false, "")
	c.Flags().StringP("stack", "s", "", "")
	c.Flags().String("tags", "", "")
	c.Flags().Bool("dry-run", false, "")
	c.Flags().String("base-path", "", "")
	c.Flags().StringSlice("config", nil, "")
	c.Flags().StringSlice("config-path", nil, "")
	c.Flags().StringSlice("profile", nil, "")
	return c
}

// TestRunVendorPull_ComponentManifestOnlyRepo_PullsOnlyUpdatedComponents is a regression test for
// the reported bug: a component.yaml-only repo (no vendor.yaml anywhere) running
// "vendor update --pull" updated every component successfully, then the automatic follow-up pull
// hard-failed with "the '--everything' flag is set, but vendor config file does not exist" -
// because the old runVendorPull always set --everything=true for a component-less "--pull", and
// --everything only knows how to enumerate a vendor.yaml's sources (internal/exec/vendor.go's
// handleVendorConfig), which doesn't exist in this repo shape at all.
//
// The fixed runVendorPull now drives one "--component X" pull per StatusUpdated result instead -
// the same code path "vendor pull --component X" already uses successfully against component.yaml
// sources.
// This proves that end to end: given a synthetic report with one updated and one untouched
// component, only the updated component's source is actually copied to disk, and no
// ErrVendorConfigNotExist (or any other error) is returned.
func TestRunVendorPull_ComponentManifestOnlyRepo_PullsOnlyUpdatedComponents(t *testing.T) {
	repoRoot := t.TempDir()
	chdirTest(t, repoRoot) // no vendor.yaml anywhere in this repo.

	// Components live under the default "components/terraform" (relative to cwd), rather than an
	// ATMOS_COMPONENTS_TERRAFORM_BASE_PATH override: ExecuteVendorPullCmd (unlike vendor update's own
	// component resolution) runs a full cfg.InitCliConfig, whose base-path join drops an absolute
	// override's leading separator when combined with the default "." base path, so an absolute
	// override here would silently break path resolution.
	base := filepath.Join(repoRoot, "components", "terraform")
	require.NoError(t, os.MkdirAll(base, 0o755))

	updatedSource := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(updatedSource, "main.tf"), []byte("# updated\n"), 0o644))
	updatedDir := writeLocalComponentManifestFixture(t, base, "updated-component", updatedSource)

	untouchedSource := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(untouchedSource, "main.tf"), []byte("# should never be pulled\n"), 0o644))
	untouchedDir := writeLocalComponentManifestFixture(t, base, "untouched-component", untouchedSource)

	report := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{
		{Component: "updated-component", Status: vendoring.StatusUpdated},
		{Component: "untouched-component", Status: vendoring.StatusUpToDate},
	}}

	err := runVendorPull(newVendorPullTestCmd(), nil, report, vendorPullParams{componentType: "terraform"})
	require.NoError(t, err, "a component.yaml-only repo-wide --pull must succeed, not fail with ErrVendorConfigNotExist")

	assert.FileExists(t, filepath.Join(updatedDir, "main.tf"), "the updated component's source must have been pulled to disk")
	assert.NoFileExists(t, filepath.Join(untouchedDir, "main.tf"), "the untouched component must not have been pulled")
}

// TestRunVendorPull_ClearsStackAndTagsBetweenIterations proves the per-component pull loop clears
// "stack" and "tags" (left over from vendor update's own --stack/--tags flags, shared with the pull
// path on the same cmd) before each "vendor pull --component X" call. Without this,
// "vendor update --tags foo --pull" or "--stack bar --pull" would fail validateVendorFlags'
// mutually-exclusive checks (component+stack, component+tags), even though the top-level update
// already resolved exactly which components to pull. It also proves "stack" is cleared via
// resetUnchangedFlag (marking Changed=false), not cmd.Flags().Set (which always marks Changed=true):
// ExecuteVendorPullCommand reads flags.Changed("stack") - not its value - to decide whether to
// process stacks at all, so a spuriously "changed" empty stack flag would force stack processing in
// a repo with no stack configuration and fail for an unrelated reason.
func TestRunVendorPull_ClearsStackAndTagsBetweenIterations(t *testing.T) {
	repoRoot := t.TempDir()
	chdirTest(t, repoRoot)

	// See TestRunVendorPull_ComponentManifestOnlyRepo_PullsOnlyUpdatedComponents for why this uses
	// the default "components/terraform" (relative to cwd) instead of an absolute
	// ATMOS_COMPONENTS_TERRAFORM_BASE_PATH override.
	base := filepath.Join(repoRoot, "components", "terraform")
	require.NoError(t, os.MkdirAll(base, 0o755))

	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# updated\n"), 0o644))
	componentDir := writeLocalComponentManifestFixture(t, base, "vpc", sourceDir)

	// Simulate "vendor update --tags foo --stack bar --pull": both flags are Changed on the shared
	// cmd before the per-component pull loop runs.
	cmd := newVendorPullTestCmd()
	require.NoError(t, cmd.Flags().Set("tags", "foo"))
	require.NoError(t, cmd.Flags().Set("stack", "bar"))
	require.True(t, cmd.Flags().Changed("stack"))

	report := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{
		{Component: "vpc", Status: vendoring.StatusUpdated},
	}}

	err := runVendorPull(cmd, nil, report, vendorPullParams{componentType: "terraform"})
	require.NoError(t, err, "stale --tags/--stack from vendor update must not fail the per-component pull")

	assert.FileExists(t, filepath.Join(componentDir, "main.tf"))
	assert.False(t, cmd.Flags().Changed("stack"),
		"'stack' must end up unchanged so a later flags.Changed(\"stack\") check doesn't force stack processing")
	assert.Equal(t, "", cmd.Flags().Lookup("tags").Value.String())
}

// TestRunVendorPull_BatchesMultipleComponentManifestUpdates is a regression test for the reported
// UX bug: "atmos vendor update --pull" against a component.yaml-only repo rendered one separate
// "0/1" progress-bar-and-completion block per updated component instead of a single unified
// "0/N" -> "N/N" run. Real SourceUpdateResult.File values (unlike the synthetic
// TestRunVendorPull_ComponentManifestOnlyRepo_PullsOnlyUpdatedComponents/
// TestRunVendorPull_ClearsStackAndTagsBetweenIterations reports above, which leave File empty and
// so exercise only the pre-existing per-component fallback loop) point at the real
// "component.yaml" manifest that declared the source, and that File basename is what
// partitionUpdatedResults uses to route every component.yaml-declared update into a single
// ExecuteComponentVendorPullBatch call. This test proves that: given three updated components,
// all three land on disk from one batched call, without ever going through the noisy
// per-component pullUpdatedComponent/e.ExecuteVendorPullCmd path.
func TestRunVendorPull_BatchesMultipleComponentManifestUpdates(t *testing.T) {
	repoRoot := t.TempDir()
	chdirTest(t, repoRoot) // no vendor.yaml anywhere in this repo.

	// See TestRunVendorPull_ComponentManifestOnlyRepo_PullsOnlyUpdatedComponents for why this uses
	// the default "components/terraform" (relative to cwd) instead of an absolute
	// ATMOS_COMPONENTS_TERRAFORM_BASE_PATH override.
	base := filepath.Join(repoRoot, "components", "terraform")
	require.NoError(t, os.MkdirAll(base, 0o755))

	names := []string{"account", "account-map", "account-settings"}
	dirs := make(map[string]string, len(names))
	results := make([]vendoring.SourceUpdateResult, 0, len(names))
	for _, name := range names {
		sourceDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# "+name+"\n"), 0o644))
		componentDir := writeLocalComponentManifestFixture(t, base, name, sourceDir)
		dirs[name] = componentDir
		results = append(results, vendoring.SourceUpdateResult{
			Component: name,
			Status:    vendoring.StatusUpdated,
			// The real basename a component.yaml-declared source's File carries (see
			// pkg/vendoring.FindComponentManifestFile), which is what routes this result into the
			// batched ExecuteComponentVendorPullBatch call instead of the per-component fallback.
			File: filepath.Join(componentDir, "component.yaml"),
		})
	}

	report := &vendoring.UpdateReport{Results: results}

	err := runVendorPull(newVendorPullTestCmd(), nil, report, vendorPullParams{componentType: "terraform"})
	require.NoError(t, err, "a batched component.yaml pull must succeed")

	for _, name := range names {
		assert.FileExists(t, filepath.Join(dirs[name], "main.tf"), "component %q must have been pulled by the batched call", name)
	}
}

// TestRunVendorPull_MixedManifestAndVendorYamlUpdates proves that a repo mixing component.yaml- and
// vendor.yaml-declared updates in the same run routes each through the right path: the
// component.yaml-declared update goes through the batched ExecuteComponentVendorPullBatch call,
// while a result whose File does not end in component.yaml/component.yml (simulating a
// vendor.yaml-declared source) keeps going through the pre-existing per-component
// pullUpdatedComponent fallback. Both must still land their files on disk.
func TestRunVendorPull_MixedManifestAndVendorYamlUpdates(t *testing.T) {
	repoRoot := t.TempDir()
	chdirTest(t, repoRoot)

	base := filepath.Join(repoRoot, "components", "terraform")
	require.NoError(t, os.MkdirAll(base, 0o755))

	batchedSource := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(batchedSource, "main.tf"), []byte("# batched\n"), 0o644))
	batchedDir := writeLocalComponentManifestFixture(t, base, "batched-component", batchedSource)

	fallbackSource := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(fallbackSource, "main.tf"), []byte("# fallback\n"), 0o644))
	fallbackDir := writeLocalComponentManifestFixture(t, base, "fallback-component", fallbackSource)

	report := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{
		{
			Component: "batched-component",
			Status:    vendoring.StatusUpdated,
			File:      filepath.Join(batchedDir, "component.yaml"),
		},
		{
			// File points at a vendor.yaml-style manifest, not a component.yaml, so this result must
			// take the fallback per-component path even though the component itself also happens to
			// be vendored via a component.yaml on disk (pullUpdatedComponent resolves it the same
			// way "vendor pull --component X" already does).
			Component: "fallback-component",
			Status:    vendoring.StatusUpdated,
			File:      filepath.Join(repoRoot, "vendor.yaml"),
		},
	}}

	err := runVendorPull(newVendorPullTestCmd(), nil, report, vendorPullParams{componentType: "terraform"})
	require.NoError(t, err, "a mixed batch/fallback pull must succeed")

	assert.FileExists(t, filepath.Join(batchedDir, "main.tf"), "the batched component must have been pulled")
	assert.FileExists(t, filepath.Join(fallbackDir, "main.tf"), "the fallback component must have been pulled")
}

// TestVendorUpdateCmd_ArchivedFlagRegistered proves the new --archived bool flag (added alongside
// --outdated so users can filter to "what's updated or archived" in one view) is registered on
// vendorUpdateCmd with a false default, following the same flags.WithBoolFlag pattern as
// --outdated.
func TestVendorUpdateCmd_ArchivedFlagRegistered(t *testing.T) {
	resetCommandFlags(t, vendorUpdateCmd)

	f := vendorUpdateCmd.Flags().Lookup("archived")
	require.NotNil(t, f, "--archived flag must be registered")
	assert.Equal(t, "false", f.DefValue, "--archived must default to false")
	assert.Equal(t, "bool", f.Value.Type())
}

func TestRenderUpdateReport_AllStatuses(t *testing.T) {
	stderr := setupVendorUICapture(t)
	report := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{
		{Component: "updated", Status: vendoring.StatusUpdated, CurrentVersion: "1.0.0", LatestVersion: "1.1.0"},
		{Component: "current", Status: vendoring.StatusUpToDate, CurrentVersion: "1.0.0"},
		{Component: "skipped", Status: vendoring.StatusSkipped, Reason: "not git"},
		{Component: "failed", Status: vendoring.StatusFailed, Reason: "remote failed"},
	}}

	renderUpdateReport(report, false, false, false)
	got := plainOutput(stderr.String())
	assert.Contains(t, got, "updated")
	assert.Contains(t, got, "1.0.0")
	assert.Contains(t, got, "1.1.0")
	assert.Contains(t, got, "current")
	assert.Contains(t, got, "Up to date")
	assert.Contains(t, got, "skipped")
	assert.Contains(t, got, "Skipped")
	assert.Contains(t, got, "not git")
	assert.Contains(t, got, "failed")
	assert.Contains(t, got, "Failed")
	assert.Contains(t, got, "remote failed")
	assert.Contains(t, got, "Updated 1 component(s).")

	stderr.Reset()
	renderUpdateReport(report, true, false, false)
	assert.Contains(t, plainOutput(stderr.String()), "Found 1 update(s) available.")

	stderr.Reset()
	renderUpdateReport(report, false, true, false)
	got = plainOutput(stderr.String())
	assert.Contains(t, got, "updated")
	assert.NotContains(t, got, "current")
	assert.NotContains(t, got, "skipped")
	assert.NotContains(t, got, "failed")
}

// TestRenderUpdateReport_TruncatesLongSHAs proves the CURRENT -> LATEST column truncates full
// 40-char git commit SHAs to a short, readable form (matching `git rev-parse --short`'s default
// of 7 chars) so a single SHA-pinned row can't blow out the table's column width and misalign
// every other row. Tag-like versions must pass through unchanged, and a SHA already at/under the
// truncation length must not be shortened further.
func TestRenderUpdateReport_TruncatesLongSHAs(t *testing.T) {
	stderr := setupVendorUICapture(t)
	const fullSHA = "b51ca8bc1750c21940eaa0f1eecc0ea514d724f1"
	const shortSHA = "b51ca8b"
	report := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{
		{Component: "aws-sso", Status: vendoring.StatusUpdated, CurrentVersion: fullSHA, LatestVersion: "v4.0.0"},
		{Component: "vpc", Status: vendoring.StatusUpdated, CurrentVersion: "v0.1.0", LatestVersion: "v0.2.0"},
		{Component: "already-short", Status: vendoring.StatusUpToDate, CurrentVersion: shortSHA},
	}}

	renderUpdateReport(report, false, false, false)
	got := plainOutput(stderr.String())

	assert.NotContains(t, got, fullSHA, "the full 40-char SHA must not appear in the rendered table")
	assert.Contains(t, got, shortSHA, "the SHA must be truncated to its short 7-char form")
	assert.Contains(t, got, "v4.0.0")
	assert.Contains(t, got, "v0.1.0")
	assert.Contains(t, got, "v0.2.0")
}

func TestFormatVersionForDisplay(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"full 40-char SHA is truncated", "b51ca8bc1750c21940eaa0f1eecc0ea514d724f1", "b51ca8b"},
		{"tag-like version passes through unchanged", "v0.1.0", "v0.1.0"},
		{"short SHA at truncation length is left as-is", "b51ca8b", "b51ca8b"},
		{"SHA shorter than truncation length is left as-is", "abc1234", "abc1234"},
		{"empty string passes through unchanged", "", ""},
		{"non-hex string longer than short length passes through unchanged", "not-a-sha-value", "not-a-sha-value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatVersionForDisplay(tt.in))
		})
	}
}

// TestBuildUpdateReportRow_StatusDotColors proves each status renders a single "●"
// (theme.IconActive) dot colored per the sensible mapping (green=Updated, blue/info=Up to date,
// yellow=Skipped, red=Failed), and that an Archived result overrides the dot to gray/Muted
// regardless of its underlying Status, appending "(archived)" to the status word. The dot and the
// status word are now separate row fields (dot goes in its own leading table column, matching the
// convention used by `atmos auth list`/`atmos version list`), so this asserts each independently.
func TestBuildUpdateReportRow_StatusDotColors(t *testing.T) {
	styles := theme.GetCurrentStyles()

	tests := []struct {
		name     string
		result   vendoring.SourceUpdateResult
		wantDot  string
		wantWord string
	}{
		{
			name:     "updated is green (Success)",
			result:   vendoring.SourceUpdateResult{Component: "vpc", Status: vendoring.StatusUpdated},
			wantDot:  styles.Success.Render(theme.IconActive),
			wantWord: "Updated",
		},
		{
			name:     "up to date is blue/info, distinct from Updated",
			result:   vendoring.SourceUpdateResult{Component: "vpc", Status: vendoring.StatusUpToDate},
			wantDot:  styles.Info.Render(theme.IconActive),
			wantWord: "Up to date",
		},
		{
			name:     "skipped is yellow/warning",
			result:   vendoring.SourceUpdateResult{Component: "vpc", Status: vendoring.StatusSkipped},
			wantDot:  styles.Warning.Render(theme.IconActive),
			wantWord: "Skipped",
		},
		{
			name:     "failed is red/error",
			result:   vendoring.SourceUpdateResult{Component: "vpc", Status: vendoring.StatusFailed},
			wantDot:  styles.Error.Render(theme.IconActive),
			wantWord: "Failed",
		},
		{
			name:     "archived overrides to gray/muted regardless of status, appends (archived)",
			result:   vendoring.SourceUpdateResult{Component: "vpc", Status: vendoring.StatusUpToDate, Archived: true},
			wantDot:  styles.Muted.Render(theme.IconActive),
			wantWord: "Up to date (archived)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, ok := buildUpdateReportRow(&tt.result, false, false, styles)
			require.True(t, ok)
			assert.Equal(t, tt.wantDot, row.dot, "dot must be its own leading-column value")
			assert.Equal(t, tt.wantWord, row.status, "status cell must be plain text with no dot prefix")
			assert.NotContains(t, row.status, theme.IconActive, "the dot must not be prefixed onto the status word anymore")
			assert.NotContains(t, row.status, theme.IconCheckmark, "old checkmark icon must not appear")
			assert.NotContains(t, row.status, theme.IconXMark, "old x-mark icon must not appear")
			assert.NotContains(t, row.status, theme.IconWarning, "old warning icon must not appear")
		})
	}
}

// TestBuildUpdateReportRows_SplitsCurrentAndLatestColumns proves CURRENT and LATEST are separate
// columns (no "CURRENT → LATEST" combined cell, no dangling arrow artifact) and that an
// up-to-date row shows its version in both columns rather than leaving LATEST blank.
func TestBuildUpdateReportRows_SplitsCurrentAndLatestColumns(t *testing.T) {
	results := []vendoring.SourceUpdateResult{
		{Component: "vpc", Status: vendoring.StatusUpdated, CurrentVersion: "1.0.0", LatestVersion: "2.0.0"},
		{Component: "eks", Status: vendoring.StatusUpToDate, CurrentVersion: "1.5.0"},
	}

	headers, rows := buildUpdateReportRows(results, false, false)
	require.Equal(t, []string{"", "COMPONENT", "STATUS", "CURRENT", "LATEST"}, headers)
	require.Len(t, rows, 2)

	assert.Equal(t, []string{rows[0][0], "vpc", rows[0][2], "1.0.0", "2.0.0"}, rows[0])
	assert.Equal(t, []string{rows[1][0], "eks", rows[1][2], "1.5.0", "1.5.0"}, rows[1])
	for _, row := range rows {
		for _, cell := range row {
			assert.NotContains(t, cell, "→", "no dangling arrow artifact should remain in any cell")
		}
	}
}

// TestBuildUpdateReportRows_ReasonColumnOmittedWhenEmpty proves the REASON column (and cell) is
// dropped entirely from both headers and rows when no row in the (post-outdated-filter) result
// set carries a reason, since an always-empty column wastes width the CURRENT/LATEST split needs.
func TestBuildUpdateReportRows_ReasonColumnOmittedWhenEmpty(t *testing.T) {
	results := []vendoring.SourceUpdateResult{
		{Component: "vpc", Status: vendoring.StatusUpdated, CurrentVersion: "1.0.0", LatestVersion: "2.0.0"},
		{Component: "eks", Status: vendoring.StatusUpToDate, CurrentVersion: "1.0.0"},
	}

	headers, rows := buildUpdateReportRows(results, false, false)
	assert.NotContains(t, headers, "REASON")
	assert.Len(t, headers, 5)
	for _, row := range rows {
		assert.Len(t, row, 5, "REASON cell must be dropped, not left blank")
	}
}

// TestBuildUpdateReportRows_ReasonColumnKeptWhenPresent proves the REASON column is kept (and
// populated only for the rows that have one) when at least one row carries a reason.
func TestBuildUpdateReportRows_ReasonColumnKeptWhenPresent(t *testing.T) {
	results := []vendoring.SourceUpdateResult{
		{Component: "vpc", Status: vendoring.StatusUpdated, CurrentVersion: "1.0.0", LatestVersion: "2.0.0"},
		{Component: "mock", Status: vendoring.StatusSkipped, CurrentVersion: "v1", Reason: "not git"},
	}

	headers, rows := buildUpdateReportRows(results, false, false)
	require.Contains(t, headers, "REASON")
	require.Len(t, rows, 2)
	assert.Equal(t, "", rows[0][5], "vpc has no reason, but the column must still be present")
	assert.Equal(t, "not git", rows[1][5])
}

// TestRenderUpdateReport_ReasonHeaderPresenceMatchesData is an end-to-end check (through the
// actual rendered table, ANSI stripped) that the REASON header text appears only when the report
// contains at least one reason, matching the user's exact complaint that an always-empty REASON
// column is dead weight.
func TestRenderUpdateReport_ReasonHeaderPresenceMatchesData(t *testing.T) {
	t.Run("omitted when no row has a reason", func(t *testing.T) {
		stderr := setupVendorUICapture(t)
		report := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{
			{Component: "vpc", Status: vendoring.StatusUpdated, CurrentVersion: "1.0.0", LatestVersion: "2.0.0"},
			{Component: "eks", Status: vendoring.StatusUpToDate, CurrentVersion: "1.0.0"},
		}}

		renderUpdateReport(report, false, false, false)
		got := plainOutput(stderr.String())
		headerLine := findHeaderLine(t, got)
		assert.NotContains(t, headerLine, "REASON")
		assert.Contains(t, headerLine, "CURRENT")
		assert.Contains(t, headerLine, "LATEST")
	})

	t.Run("kept when a row has a reason", func(t *testing.T) {
		stderr := setupVendorUICapture(t)
		report := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{
			{Component: "vpc", Status: vendoring.StatusUpdated, CurrentVersion: "1.0.0", LatestVersion: "2.0.0"},
			{Component: "mock", Status: vendoring.StatusSkipped, Reason: "not git"},
		}}

		renderUpdateReport(report, false, false, false)
		got := plainOutput(stderr.String())
		headerLine := findHeaderLine(t, got)
		assert.Contains(t, headerLine, "REASON")
	})
}

// TestRenderUpdateReport_ArchivedShowsLabel proves an archived result's status word is annotated
// with "(archived)" in the rendered output, end-to-end.
func TestRenderUpdateReport_ArchivedShowsLabel(t *testing.T) {
	stderr := setupVendorUICapture(t)
	report := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{
		{Component: "old-repo", Status: vendoring.StatusUpToDate, CurrentVersion: "1.0.0", Archived: true},
	}}

	renderUpdateReport(report, false, false, false)
	got := plainOutput(stderr.String())
	assert.Contains(t, got, "Up to date (archived)")
}

// TestBuildUpdateReportRows_DotIsLeadingColumn proves the status dot now lives in its own,
// blank-titled leading column (matching `atmos auth list`'s identity table and
// `atmos version list`'s createVersionTable convention), rather than being prefixed onto the
// STATUS cell: the first column of every data row is just the dot glyph, and the STATUS-position
// cell is plain text with no dot prefix.
func TestBuildUpdateReportRows_DotIsLeadingColumn(t *testing.T) {
	results := []vendoring.SourceUpdateResult{
		{Component: "vpc", Status: vendoring.StatusUpdated, CurrentVersion: "1.0.0", LatestVersion: "2.0.0"},
	}

	headers, rows := buildUpdateReportRows(results, false, false)
	require.Equal(t, "", headers[0], "the leading column header must be blank")
	require.Equal(t, "COMPONENT", headers[1])
	require.Equal(t, "STATUS", headers[2])
	require.Len(t, rows, 1)

	dotCell := rows[0][0]
	statusCellValue := rows[0][2]
	assert.Contains(t, dotCell, theme.IconActive, "the first column must contain the dot glyph")
	assert.Equal(t, "vpc", rows[0][1], "the second column must be the component name")
	assert.Equal(t, "Updated", statusCellValue, "the STATUS cell must be plain text")
	assert.NotContains(t, statusCellValue, theme.IconActive, "the STATUS cell must not carry the dot prefix anymore")
}

// TestBuildUpdateReportRows_ArchivedFilter proves --archived alone keeps rows whose upstream is
// archived regardless of Status (an archived-but-up-to-date or archived-but-skipped component
// still shows, since archived is orthogonal to Status), and hides every non-archived row.
func TestBuildUpdateReportRows_ArchivedFilter(t *testing.T) {
	results := []vendoring.SourceUpdateResult{
		{Component: "updated-comp", Status: vendoring.StatusUpdated, CurrentVersion: "1.0.0", LatestVersion: "2.0.0"},
		{Component: "current-comp", Status: vendoring.StatusUpToDate, CurrentVersion: "1.0.0"},
		{Component: "archived-current", Status: vendoring.StatusUpToDate, CurrentVersion: "1.0.0", Archived: true},
		{Component: "archived-skipped", Status: vendoring.StatusSkipped, Reason: "not git", Archived: true},
		{Component: "archived-updated", Status: vendoring.StatusUpdated, CurrentVersion: "1.0.0", LatestVersion: "2.0.0", Archived: true},
	}

	_, rows := buildUpdateReportRows(results, false, true)
	components := rowComponents(rows)
	assert.ElementsMatch(t, []string{"archived-current", "archived-skipped", "archived-updated"}, components)
	assert.NotContains(t, components, "updated-comp")
	assert.NotContains(t, components, "current-comp")
}

// TestBuildUpdateReportRows_OutdatedAndArchivedTogetherIsUnion proves setting --outdated and
// --archived together shows the union (StatusUpdated rows OR archived rows), matching the user's
// literal ask "what's updated or archived" in one view, not their intersection.
func TestBuildUpdateReportRows_OutdatedAndArchivedTogetherIsUnion(t *testing.T) {
	results := []vendoring.SourceUpdateResult{
		{Component: "updated-comp", Status: vendoring.StatusUpdated, CurrentVersion: "1.0.0", LatestVersion: "2.0.0"},
		{Component: "current-comp", Status: vendoring.StatusUpToDate, CurrentVersion: "1.0.0"},
		{Component: "archived-current", Status: vendoring.StatusUpToDate, CurrentVersion: "1.0.0", Archived: true},
		{Component: "archived-skipped", Status: vendoring.StatusSkipped, Reason: "not git", Archived: true},
		{Component: "archived-updated", Status: vendoring.StatusUpdated, CurrentVersion: "1.0.0", LatestVersion: "2.0.0", Archived: true},
	}

	_, rows := buildUpdateReportRows(results, true, true)
	components := rowComponents(rows)
	// archived-updated matches both conditions; it must appear exactly once (deduplicated), not twice.
	assert.ElementsMatch(t, []string{"updated-comp", "archived-current", "archived-skipped", "archived-updated"}, components)
	assert.NotContains(t, components, "current-comp", "current-comp is neither updated nor archived")
}

// TestBuildUpdateReportRows_NeitherFlagShowsEverything is a regression check that the unchanged
// default (no --outdated, no --archived) still shows every row.
func TestBuildUpdateReportRows_NeitherFlagShowsEverything(t *testing.T) {
	results := []vendoring.SourceUpdateResult{
		{Component: "updated-comp", Status: vendoring.StatusUpdated, CurrentVersion: "1.0.0", LatestVersion: "2.0.0"},
		{Component: "current-comp", Status: vendoring.StatusUpToDate, CurrentVersion: "1.0.0"},
		{Component: "archived-current", Status: vendoring.StatusUpToDate, CurrentVersion: "1.0.0", Archived: true},
	}

	_, rows := buildUpdateReportRows(results, false, false)
	components := rowComponents(rows)
	assert.ElementsMatch(t, []string{"updated-comp", "current-comp", "archived-current"}, components)
}

// rowComponents extracts the COMPONENT column (index 1, since index 0 is the leading dot column)
// from a set of built rows, for filter-assertion convenience.
func rowComponents(rows [][]string) []string {
	components := make([]string, 0, len(rows))
	for _, row := range rows {
		components = append(components, row[1])
	}
	return components
}

// findHeaderLine returns the rendered table's header line (the one containing "COMPONENT").
func findHeaderLine(t *testing.T, rendered string) string {
	t.Helper()
	for _, line := range strings.Split(rendered, "\n") {
		if strings.Contains(line, "COMPONENT") {
			return line
		}
	}
	t.Fatal("no header line containing COMPONENT found in rendered output")
	return ""
}

// --- edit.go error paths -----------------------------------------------------

func TestVendorGetCmd_MissingFile(t *testing.T) {
	resetCommandFlags(t, vendorGetCmd)

	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	require.NoError(t, vendorGetCmd.Flags().Set("file", missing))

	err := vendorGetCmd.RunE(vendorGetCmd, []string{"vpc"})
	require.Error(t, err)
	assert.ErrorIs(t, err, atmosyaml.ErrReadFile)
}

func TestVendorGetCmd_MissingComponent(t *testing.T) {
	resetCommandFlags(t, vendorGetCmd)

	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: ["components/terraform/vpc"]
`)
	require.NoError(t, vendorGetCmd.Flags().Set("file", file))

	err := vendorGetCmd.RunE(vendorGetCmd, []string{"missing-component"})
	require.Error(t, err)
	assert.ErrorIs(t, err, atmosyaml.ErrYAMLPathNotFound)
}

func TestVendorSetCmd_MissingFile(t *testing.T) {
	resetCommandFlags(t, vendorSetCmd)

	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	require.NoError(t, vendorSetCmd.Flags().Set("file", missing))

	err := vendorSetCmd.RunE(vendorSetCmd, []string{"vpc", "v0.2.0"})
	require.Error(t, err)
	assert.ErrorIs(t, err, atmosyaml.ErrReadFile)
}

func TestVendorSetCmd_ComponentNotFound(t *testing.T) {
	resetCommandFlags(t, vendorSetCmd)

	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: ["components/terraform/vpc"]
`)
	require.NoError(t, vendorSetCmd.Flags().Set("file", file))

	// SetComponentVersion fails because "missing-component" is not declared in
	// the manifest, surfacing ErrYAMLPathNotFound from the pre-check in
	// vendoring.SetComponentVersion.
	err := vendorSetCmd.RunE(vendorSetCmd, []string{"missing-component", "v0.2.0"})
	require.Error(t, err)
	assert.ErrorIs(t, err, atmosyaml.ErrYAMLPathNotFound)
}

func TestSplitTags(t *testing.T) {
	tests := []struct {
		name string
		csv  string
		want []string
	}{
		{"empty string", "", nil},
		{"whitespace only", "   ", nil},
		{"single tag", "prod", []string{"prod"}},
		{"leading and trailing commas", ",prod,staging,", []string{"prod", "staging"}},
		{"whitespace around tags", " prod , staging ", []string{"prod", "staging"}},
		{"empty segments between commas", "prod,,staging", []string{"prod", "staging"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, splitTags(tt.csv))
		})
	}
}
