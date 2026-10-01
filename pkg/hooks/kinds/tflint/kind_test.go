package tflint

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks"
	tflintscanner "github.com/cloudposse/atmos/pkg/scanners/tflint"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestKindIsRegistered(t *testing.T) {
	kind, ok := hooks.GetKind(kindName)
	require.True(t, ok)

	assert.Equal(t, kindName, kind.Name)
	assert.Equal(t, "tflint", kind.Command)
	assert.Equal(t, []string{
		"--format=sarif",
		"--chdir=$ATMOS_COMPONENT_PATH",
	}, kind.DefaultArgs)
	assert.Equal(t, hooks.OnFailureWarn, kind.OnFailure)
	assert.True(t, kind.CaptureStdout, "tflint emits SARIF to stdout with no file-output flag")
	assert.NotNil(t, kind.ResultHandler)
	_, ok = kind.Engine.(tflintEngine)
	assert.True(t, ok)
}

func TestTflintEngineRunNilGuards(t *testing.T) {
	var engine tflintEngine

	out, err := engine.Run(nil)
	require.Nil(t, out)
	require.ErrorIs(t, err, errUtils.ErrNilParam)

	out, err = engine.Run(&hooks.ExecContext{})
	require.Nil(t, out)
	require.ErrorIs(t, err, errUtils.ErrNilParam)
}

// fakeToolchain symlinks the running test binary as `tflint` (or `tflint.exe`
// on Windows) inside a fresh toolchain dir, so tflintEngine resolves it via
// ctx.ToolchainPATH exactly like a real dependencies.tools-installed binary.
func fakeToolchain(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	toolchain := t.TempDir()
	toolName := tflintscanner.Command
	if runtime.GOOS == "windows" {
		toolName += ".exe"
	}
	require.NoError(t, os.Symlink(exe, filepath.Join(toolchain, toolName)))
	return toolchain
}

func TestTflintEngineRunResolvesArgsAndCapturesStdout(t *testing.T) {
	base := t.TempDir()
	componentPath := filepath.Join(base, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentPath, ".tflint.hcl"), []byte("plugin \"terraform\" {}"), 0o600))

	argsFile := filepath.Join(t.TempDir(), "args.txt")
	const sarifBody = `{"runs":[{"tool":{"driver":{"name":"tflint","rules":[]}},"results":[]}]}`

	atmosConfig := &schema.AtmosConfiguration{
		BasePathAbsolute:         base,
		TerraformDirAbsolutePath: filepath.Join(base, "components", "terraform"),
	}
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "prod"}

	kind, ok := hooks.GetKind(kindName)
	require.True(t, ok)

	hook := kind.ResolveDefaults(&hooks.Hook{Kind: kindName})
	ctx := &hooks.ExecContext{
		Hook:          hook,
		Kind:          kind,
		AtmosConfig:   atmosConfig,
		Info:          info,
		ToolchainPATH: fakeToolchain(t),
	}
	// hooks.ExecContext has no BaseEnv/Ctx knobs to inject env; use the hook's
	// own Env map (expanded by CommandEngine same as any other hook field).
	ctx.Hook.Env = map[string]string{
		"_ATMOS_TEST_TFLINT_FAKE": "1",
		"_ATMOS_TEST_ARGS_FILE":   argsFile,
		"_ATMOS_TEST_STDOUT_BODY": sarifBody,
	}

	out, err := kind.Engine.Run(ctx)
	require.NoError(t, err)
	require.NotNil(t, out)

	// ResolveArgs must have injected the component's .tflint.hcl before the
	// subprocess ran.
	argsBody, readErr := os.ReadFile(argsFile)
	require.NoError(t, readErr)
	assert.Contains(t, string(argsBody), "--config="+filepath.Join(componentPath, ".tflint.hcl"))

	// CaptureStdout must have redirected the fake tool's SARIF into the
	// artifact/summary, exercising the exit-code-aware ResultHandler.
	require.NotNil(t, out.Artifact)
	assert.Contains(t, string(out.Artifact.Body), `"tflint"`)
	require.NotNil(t, out.Summary)
	assert.Equal(t, kindName, out.Summary.Kind)
	assert.Equal(t, hooks.StatusSuccess, out.Summary.Status)
	assert.NotContains(t, strings.ToLower(out.Summary.Title), "fail")
}
