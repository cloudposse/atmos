package tflint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/scanners"
	"github.com/cloudposse/atmos/pkg/schema"
)

// findingSARIF is a minimal SARIF document with one result, standing in for what a real
// tflint invocation emits when it finds an issue and exits 1.
const findingSARIF = `{"runs":[{"tool":{"driver":{"name":"tflint","rules":[{"id":"test_rule"}]}},"results":[{"ruleId":"test_rule","level":"error","message":{"text":"test finding"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"main.tf"},"region":{"startLine":1,"startColumn":1}}}]}]}]}`

func TestMain(m *testing.M) {
	if os.Getenv("_ATMOS_TFLINT_TEST_SARIF") == "1" {
		fmt.Fprint(os.Stdout, `{"runs":[{"tool":{"driver":{"name":"tflint","rules":[]}},"results":[]}]}`)
		os.Exit(0)
	}
	if os.Getenv("_ATMOS_TFLINT_TEST_SARIF_FINDING") == "1" {
		fmt.Fprint(os.Stdout, findingSARIF)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestRunUsesToolchainBinaryAndParsesSARIF(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	toolchain := t.TempDir()
	toolName := Command
	if runtime.GOOS == "windows" {
		toolName += ".exe"
	}
	tool := filepath.Join(toolchain, toolName)
	require.NoError(t, os.Symlink(exe, tool))

	out, scan, err := Run(context.Background(), &Options{
		ToolchainPATH: toolchain,
		Env:           map[string]string{"_ATMOS_TFLINT_TEST_SARIF": "1"},
		Info:          &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "prod"},
	})
	require.NoError(t, err)
	require.NotNil(t, scan)
	assert.Equal(t, Name, scan.Name)
	assert.Equal(t, toolchain, scan.ToolchainPATH)
	require.NotNil(t, out.Artifact)
	require.NotNil(t, out.Summary)
	assert.Equal(t, Name, out.Summary.Kind)
	assert.Contains(t, string(out.Artifact.Body), `"tflint"`)
}

// TestRunOnFailureFailSurfacesFindingsAsError verifies that OnFailure: scanners.OnFailureFail
// (what executeTarget now passes for the standalone `atmos terraform lint` command) turns a
// non-zero tflint exit — including one caused by genuine findings, not just a crash — into a
// returned error, so the overall command's exit code reflects real findings.
func TestRunOnFailureFailSurfacesFindingsAsError(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	toolchain := t.TempDir()
	toolName := Command
	if runtime.GOOS == "windows" {
		toolName += ".exe"
	}
	tool := filepath.Join(toolchain, toolName)
	require.NoError(t, os.Symlink(exe, tool))
	base := t.TempDir()
	componentPath := filepath.Join(base, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentPath, "main.tf"), []byte("resource \"null_resource\" \"example\" {}\n"), 0o600))

	out, _, err := Run(context.Background(), &Options{
		ToolchainPATH: toolchain,
		Env:           map[string]string{"_ATMOS_TFLINT_TEST_SARIF_FINDING": "1"},
		AtmosConfig:   &schema.AtmosConfiguration{TerraformDirAbsolutePath: filepath.Join(base, "components", "terraform")},
		Info:          &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", FinalComponent: "vpc", Stack: "prod"},
		OnFailure:     scanners.OnFailureFail,
		OutputFormat:  OutputFormatRich,
	})
	require.Error(t, err)
	require.NotNil(t, out.Summary)
	assert.Contains(t, out.Summary.TerminalBody, "main.tf:1:1")
	assert.Contains(t, out.Summary.Body, "| Severity | Rule | Message | Location |")
}

// TestRunOnFailureWarnSuppressesFindingsError verifies the shared scanner default
// (OnFailureWarn, used by `hooks:`-embedded tflint runs during plan/apply) still swallows a
// findings-driven non-zero exit, so a lint hook never blocks infrastructure changes.
func TestRunOnFailureWarnSuppressesFindingsError(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	toolchain := t.TempDir()
	toolName := Command
	if runtime.GOOS == "windows" {
		toolName += ".exe"
	}
	tool := filepath.Join(toolchain, toolName)
	require.NoError(t, os.Symlink(exe, tool))

	_, _, err = Run(context.Background(), &Options{
		ToolchainPATH: toolchain,
		Env:           map[string]string{"_ATMOS_TFLINT_TEST_SARIF_FINDING": "1"},
		Info:          &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "prod"},
	})
	require.NoError(t, err)
}

func TestConfigPathPrefersComponentConfig(t *testing.T) {
	base := t.TempDir()
	componentPath := filepath.Join(base, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentPath, ".tflint.hcl"), []byte("plugin \"terraform\" {}"), 0o600))

	config := &schema.AtmosConfiguration{BasePathAbsolute: base}
	config.TerraformDirAbsolutePath = filepath.Join(base, "components", "terraform")
	config.Components.Terraform.Lint.Config = "config/.tflint.hcl"
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"}

	require.Equal(t, filepath.Join(componentPath, ".tflint.hcl"), ConfigPath(config, info))
}

func TestConfigPathUsesConfiguredGlobalFallback(t *testing.T) {
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePathAbsolute: base}
	config.TerraformDirAbsolutePath = filepath.Join(base, "components", "terraform")
	config.Components.Terraform.Lint.Config = ".tflint.hcl"
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"}

	require.Equal(t, filepath.Join(base, ".tflint.hcl"), ConfigPath(config, info))
	require.Equal(
		t,
		[]string{"--format=sarif", "$ATMOS_COMPONENT_PATH", "--config=" + filepath.Join(base, ".tflint.hcl")},
		ResolveArgs(DefaultArgs(), config, info),
	)
}

func TestConfigPathUsesMostSpecificStandardLocation(t *testing.T) {
	base := t.TempDir()
	componentsPath := filepath.Join(base, "components", "terraform")
	componentPath := filepath.Join(componentsPath, "vpc")
	require.NoError(t, os.MkdirAll(componentPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, ".tflint.hcl"), []byte("root"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(componentsPath, ".tflint.hcl"), []byte("base"), 0o600))

	config := &schema.AtmosConfiguration{BasePathAbsolute: base, TerraformDirAbsolutePath: componentsPath}
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"}
	require.Equal(t, filepath.Join(componentsPath, ".tflint.hcl"), ConfigPath(config, info))

	require.NoError(t, os.Remove(filepath.Join(componentsPath, ".tflint.hcl")))
	require.Equal(t, filepath.Join(base, ".tflint.hcl"), ConfigPath(config, info))

	require.NoError(t, os.WriteFile(filepath.Join(componentPath, ".tflint.hcl"), []byte("component"), 0o600))
	require.Equal(t, filepath.Join(componentPath, ".tflint.hcl"), ConfigPath(config, info))
}

func TestResolveArgsPreservesExplicitConfig(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.Lint.Config = "global.tflint.hcl"
	args := []string{"--config=custom.tflint.hcl", "--format=sarif"}

	require.Equal(t, args, ResolveArgs(args, config, &schema.ConfigAndStacksInfo{}))
}

func TestResolveArgsLeavesArgsUnchangedWhenNoConfigFound(t *testing.T) {
	// No component/base/repo-root .tflint.hcl and no global fallback
	// configured: ResolveArgs must return the original args untouched
	// (ConfigPath resolves to "").
	base := t.TempDir()
	config := &schema.AtmosConfiguration{BasePathAbsolute: base}
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc"}

	args := []string{"--format=sarif"}
	require.Equal(t, args, ResolveArgs(args, config, info))
}

func TestConfigPathNilInputsReturnEmpty(t *testing.T) {
	assert.Equal(t, "", ConfigPath(nil, &schema.ConfigAndStacksInfo{}))
	assert.Equal(t, "", ConfigPath(&schema.AtmosConfiguration{}, nil))
}

func TestConfigDirectoriesSkipsEmptyAndDuplicateEntries(t *testing.T) {
	base := t.TempDir()

	// TerraformDirAbsolutePath and BasePathAbsolute are both empty (so
	// repositoryRoot("") == "" too), and ComponentPath falls back to the
	// current working directory when AtmosConfig.TerraformDirAbsolutePath
	// is empty. Point BasePath/TerraformDirAbsolutePath at the same
	// directory so ComponentPath's fallback and repositoryRoot's fallback
	// produce the SAME directory, exercising the dedup-skip branch.
	config := &schema.AtmosConfiguration{
		BasePathAbsolute:         base,
		TerraformDirAbsolutePath: base,
	}
	info := &schema.ConfigAndStacksInfo{}

	dirs := configDirectories(config, info)
	require.Len(t, dirs, 1, "ComponentPath and repositoryRoot fallback both resolve to base, so the duplicate must be skipped")
	assert.Equal(t, filepath.Clean(base), dirs[0])
}

func TestConfigDirectoriesSkipsEmptyEntries(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)

	// TerraformDirAbsolutePath and BasePathAbsolute are both empty, so
	// repositoryRoot("") == "" is skipped and ComponentPath falls back to
	// the working directory (the only non-empty candidate).
	config := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{}

	dirs := configDirectories(config, info)
	require.Len(t, dirs, 1)
	assert.Equal(t, filepath.Clean(wd), dirs[0])
}

func TestRepositoryRootEmptyBasePath(t *testing.T) {
	assert.Equal(t, "", repositoryRoot(""))
}

func TestRepositoryRootResolvesGitWorktreeRoot(t *testing.T) {
	root := t.TempDir()
	_, err := gitlib.PlainInit(root, false)
	require.NoError(t, err)

	nested := filepath.Join(root, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	// Resolve symlinks (e.g. macOS /var -> /private/var) on both sides so the
	// comparison isn't sensitive to how the OS temp dir happens to resolve.
	want, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	got, err := filepath.EvalSymlinks(repositoryRoot(nested))
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestRepositoryRootFallsBackWhenNotAGitWorktree(t *testing.T) {
	notARepo := t.TempDir()
	assert.Equal(t, notARepo, repositoryRoot(notARepo))
}
