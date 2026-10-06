package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

// Compile-time sentinel: a rename of the field the tests rely on must fail the build.
var _ = schema.WorkflowStep{ScriptSource: ""}

const hookScript = `load("lib/util.star", "marker")
if marker != "included-dir":
    fail("load() resolved against " + marker)
`

// scriptSourceFixture builds a project whose hook script and its lib sit under scripts/hooks, and
// a decoy lib in the working directory that must lose to the included file's directory.
func scriptSourceFixture(t *testing.T) (*schema.AtmosConfiguration, string) {
	t.Helper()
	base := t.TempDir()
	write := func(path, content string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	write(filepath.Join(base, "scripts", "hooks", "check.star"), hookScript)
	write(filepath.Join(base, "scripts", "hooks", "lib", "util.star"), `marker = "included-dir"`+"\n")

	decoy := t.TempDir()
	write(filepath.Join(decoy, "lib", "util.star"), `marker = "decoy-cwd"`+"\n")
	t.Chdir(decoy)

	cfg := &schema.AtmosConfiguration{BasePath: base, BasePathAbsolute: base}
	return cfg, filepath.Join(base, "stacks", "dev.yaml")
}

// hookSection decodes a stack manifest snippet exactly like stack processing does (so !include
// records script_source) and returns the value of its `with` key.
func hookSection(t *testing.T, cfg *schema.AtmosConfiguration, manifest, text string) any {
	t.Helper()
	out, err := utils.UnmarshalYAMLFromFile[map[string]any](cfg, text, manifest)
	require.NoError(t, err)
	return out["with"]
}

func TestStepHookScriptSource_LoadResolvesNextToIncludedFile(t *testing.T) {
	cfg, manifest := scriptSourceFixture(t)
	with := hookSection(t, cfg, manifest, "with:\n  interpreter: starlark\n  output: none\n  script: !include scripts/hooks/check.star\n")
	require.Contains(t, with, utils.ScriptSourceKey)

	ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "script", OnFailure: OnFailureFail, With: with})
	ctx.AtmosConfig = cfg

	_, err := stepEngine{}.Run(ctx)
	require.NoError(t, err)
}

func TestStepHookScriptSource_WithoutProvenanceLoadsFromWorkingDirectory(t *testing.T) {
	cfg, manifest := scriptSourceFixture(t)
	with, ok := hookSection(t, cfg, manifest, "with:\n  interpreter: starlark\n  output: none\n  script: !include scripts/hooks/check.star\n").(map[string]any)
	require.True(t, ok)
	// Negative path: with the recorded source removed the script resolves load() against the
	// working directory and finds the decoy.
	delete(with, utils.ScriptSourceKey)

	ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "script", OnFailure: OnFailureFail, With: with})
	ctx.AtmosConfig = cfg

	_, err := stepEngine{}.Run(ctx)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.ErrorContains(t, err, "decoy-cwd")
}

func TestStepsHookScriptSource_EachStepKeepsItsOwnFile(t *testing.T) {
	cfg, manifest := scriptSourceFixture(t)
	other := filepath.Join(cfg.BasePath, "scripts", "other")
	require.NoError(t, os.MkdirAll(filepath.Join(other, "lib"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(other, "second.star"), []byte(`load("lib/util.star", "marker")
if marker != "other-dir":
    fail("second step resolved load() against " + marker)
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(other, "lib", "util.star"), []byte(`marker = "other-dir"`+"\n"), 0o600))

	with := hookSection(t, cfg, manifest, "with:\n"+
		"  - type: script\n    interpreter: starlark\n    output: none\n    script: !include scripts/hooks/check.star\n"+
		"  - type: script\n    interpreter: starlark\n    output: none\n    script: !include scripts/other/second.star\n"+
		"  - type: script\n    interpreter: starlark\n    output: none\n    script: print('inline')\n")

	ctx := stepsExecContext(&Hook{Kind: stepsKindName, OnFailure: OnFailureFail, With: with})
	ctx.AtmosConfig = cfg

	_, err := stepsEngine{}.Run(ctx)
	require.NoError(t, err)
}

func TestWorkflowStepFromHookPayload_ScriptSource(t *testing.T) {
	cfg, manifest := scriptSourceFixture(t)
	absolute := filepath.Join(cfg.BasePath, "scripts", "hooks", "check.star")

	t.Run("recorded relative path becomes absolute and is not a step parameter", func(t *testing.T) {
		with := hookSection(t, cfg, manifest, "with:\n  interpreter: starlark\n  script: !include scripts/hooks/check.star\n")
		ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "script", With: with})
		ctx.AtmosConfig = cfg

		ws, err := stepFromHookWithVariables(ctx, stepVariables(ctx))
		require.NoError(t, err)
		assert.Equal(t, absolute, ws.ScriptSource)
		assert.Equal(t, hookScript, ws.Script)
		assert.NotContains(t, ws.With, utils.ScriptSourceKey)
	})

	t.Run("inline script has no source", func(t *testing.T) {
		with := hookSection(t, cfg, manifest, "with:\n  interpreter: starlark\n  script: print('inline')\n")
		ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "script", With: with})
		ctx.AtmosConfig = cfg

		ws, err := stepFromHookWithVariables(ctx, stepVariables(ctx))
		require.NoError(t, err)
		assert.Empty(t, ws.ScriptSource)
	})

	t.Run("absolute recorded path is kept", func(t *testing.T) {
		ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "script", With: map[string]any{
			"interpreter": "starlark", "script": "print('x')", utils.ScriptSourceKey: absolute,
			utils.ScriptSourceSHA256Key: utils.ScriptSourceHash("print('x')"),
		}})
		ctx.AtmosConfig = cfg

		ws, err := stepFromHookWithVariables(ctx, stepVariables(ctx))
		require.NoError(t, err)
		assert.Equal(t, absolute, ws.ScriptSource)
	})

	t.Run("provenance without a fingerprint is ignored", func(t *testing.T) {
		ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "script", With: map[string]any{
			"interpreter": "starlark", "script": "print('x')", utils.ScriptSourceKey: absolute,
		}})
		ctx.AtmosConfig = cfg

		ws, err := stepFromHookWithVariables(ctx, stepVariables(ctx))
		require.NoError(t, err)
		assert.Empty(t, ws.ScriptSource)
		assert.NotContains(t, ws.With, utils.ScriptSourceKey)
		assert.NotContains(t, ws.With, utils.ScriptSourceSHA256Key)
	})

	t.Run("child steps of a group keep their own source", func(t *testing.T) {
		with := hookSection(t, cfg, manifest, "with:\n  steps:\n"+
			"    - type: script\n      interpreter: starlark\n      script: !include scripts/hooks/check.star\n"+
			"    - type: script\n      interpreter: starlark\n      script: print('inline')\n")
		ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "parallel", With: with})
		ctx.AtmosConfig = cfg

		ws, err := stepFromHookWithVariables(ctx, stepVariables(ctx))
		require.NoError(t, err)
		require.Len(t, ws.Steps, 2)
		assert.Equal(t, absolute, ws.Steps[0].ScriptSource)
		assert.Empty(t, ws.Steps[1].ScriptSource)
	})
}

// staleProvenanceWith builds the merged `with` of a hook whose base stack included a script and
// whose child stack overrode only `script` with childScript. The merge is the real stack deep
// merge, so the base's script_source and script_source_sha256 survive into the result exactly as
// they do during stack inheritance.
func staleProvenanceWith(t *testing.T, cfg *schema.AtmosConfiguration, manifest, childScript string) map[string]any {
	t.Helper()
	base := hookSection(t, cfg, manifest, "with:\n  interpreter: starlark\n  output: none\n  script: !include scripts/hooks/check.star\n")
	baseWith, ok := base.(map[string]any)
	require.True(t, ok)
	require.Contains(t, baseWith, utils.ScriptSourceKey)
	require.Contains(t, baseWith, utils.ScriptSourceSHA256Key)

	merged, err := merge.Merge(cfg, []map[string]any{baseWith, {"script": childScript}})
	require.NoError(t, err)
	// The inherited keys are still there: describe output shows them.
	require.Equal(t, baseWith[utils.ScriptSourceKey], merged[utils.ScriptSourceKey])
	require.Equal(t, baseWith[utils.ScriptSourceSHA256Key], merged[utils.ScriptSourceSHA256Key])
	return merged
}

func TestStepHookScriptSource_InlineOverrideIgnoresInheritedProvenance(t *testing.T) {
	cfg, manifest := scriptSourceFixture(t)
	// The working directory holds the decoy lib; scripts/hooks/lib holds the included file's lib.
	// A stale source would resolve load() to "included-dir"; the working directory yields the decoy.
	merged := staleProvenanceWith(t, cfg, manifest, `load("lib/util.star", "marker")
if marker != "decoy-cwd":
    fail("load() resolved against " + marker)
`)

	ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "script", OnFailure: OnFailureFail, With: merged})
	ctx.AtmosConfig = cfg

	ws, err := stepFromHookWithVariables(ctx, stepVariables(ctx))
	require.NoError(t, err)
	assert.Empty(t, ws.ScriptSource)
	assert.NotContains(t, ws.With, utils.ScriptSourceKey)
	assert.NotContains(t, ws.With, utils.ScriptSourceSHA256Key)

	_, err = stepEngine{}.Run(ctx)
	require.NoError(t, err)
}

func TestStepsHookScriptSource_InlineOverrideIgnoresInheritedProvenance(t *testing.T) {
	cfg, manifest := scriptSourceFixture(t)
	merged := staleProvenanceWith(t, cfg, manifest, `load("lib/util.star", "marker")
if marker != "decoy-cwd":
    fail("load() resolved against " + marker)
`)
	merged["type"] = "script"

	ctx := stepsExecContext(&Hook{Kind: stepsKindName, OnFailure: OnFailureFail, With: []any{merged}})
	ctx.AtmosConfig = cfg

	_, err := stepsEngine{}.Run(ctx)
	require.NoError(t, err)
}

// Positive control for the stale-provenance tests: when the child does not override the script,
// the inherited provenance is valid and load() resolves next to the included file.
func TestStepHookScriptSource_UnchangedScriptKeepsInheritedProvenance(t *testing.T) {
	cfg, manifest := scriptSourceFixture(t)
	merged := staleProvenanceWith(t, cfg, manifest, hookScript)

	ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "script", OnFailure: OnFailureFail, With: merged})
	ctx.AtmosConfig = cfg

	_, err := stepEngine{}.Run(ctx)
	require.NoError(t, err)
}

// A script that uses {{ }} is rendered at hook time; the fingerprint is checked against the value
// as it was before rendering, so the included file's provenance still applies.
func TestStepHookScriptSource_TemplatedScriptStillMatches(t *testing.T) {
	cfg, manifest := scriptSourceFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(cfg.BasePath, "scripts", "hooks", "templated.star"), []byte(`load("lib/util.star", "marker")
if marker != "included-dir":
    fail("load() resolved against " + marker)
if "{{ printf "%s" "rendered" }}" != "rendered":
    fail("template was not rendered")
`), 0o600))
	with := hookSection(t, cfg, manifest, "with:\n  interpreter: starlark\n  output: none\n  script: !include scripts/hooks/templated.star\n")

	ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "script", OnFailure: OnFailureFail, With: with})
	ctx.AtmosConfig = cfg

	ws, err := stepFromHookWithVariables(ctx, stepVariables(ctx))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cfg.BasePath, "scripts", "hooks", "templated.star"), ws.ScriptSource)
	assert.Contains(t, ws.Script, `if "rendered" != "rendered"`)

	_, err = stepEngine{}.Run(ctx)
	require.NoError(t, err)
}

func TestWithoutScriptSource_StripsBothKeys(t *testing.T) {
	in := map[string]any{"script": "x", utils.ScriptSourceKey: "a.star", utils.ScriptSourceSHA256Key: "abc"}
	out := withoutScriptSource(in)
	assert.Equal(t, map[string]any{"script": "x"}, out)
	// The input is not modified.
	assert.Contains(t, in, utils.ScriptSourceKey)
	assert.Contains(t, in, utils.ScriptSourceSHA256Key)

	plain := map[string]any{"script": "x"}
	assert.Equal(t, plain, withoutScriptSource(plain))
}

// resolveHookForExecution round-trips the hook through yaml.v2, so a step hook's `with:` reaches the
// engine as map[any]any. Provenance must survive that shape, for the step and for group children.
func TestStepHookScriptSource_AcceptsYAMLv2MapShape(t *testing.T) {
	cfg, manifest := scriptSourceFixture(t)
	absolute := filepath.Join(cfg.BasePath, "scripts", "hooks", "check.star")
	with, ok := hookSection(t, cfg, manifest, "with:\n  interpreter: starlark\n  script: !include scripts/hooks/check.star\n").(map[string]any)
	require.True(t, ok)

	v2With := map[any]any{}
	for key, value := range with {
		v2With[key] = value
	}
	ctx := stepExecContext(&Hook{Kind: stepKindName, Type: "script", With: v2With})
	ctx.AtmosConfig = cfg

	ws, err := stepFromHookWithVariables(ctx, stepVariables(ctx))
	require.NoError(t, err)
	assert.Equal(t, absolute, ws.ScriptSource)

	group := map[any]any{"steps": []any{v2With, map[any]any{"interpreter": "starlark", "script": "print('inline')"}}}
	ctx = stepExecContext(&Hook{Kind: stepKindName, Type: "parallel", With: group})
	ctx.AtmosConfig = cfg
	ws, err = stepFromHookWithVariables(ctx, stepVariables(ctx))
	require.NoError(t, err)
	require.Len(t, ws.Steps, 2)
	assert.Equal(t, absolute, ws.Steps[0].ScriptSource)
	assert.Empty(t, ws.Steps[1].ScriptSource)
}
