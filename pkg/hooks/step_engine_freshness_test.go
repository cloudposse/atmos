package hooks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/runner/freshness"
	runnerstep "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// uncachedGlobber resolves patterns directly so a file removed between two in-process runs is
// seen as missing; the production globber is LRU-cached per process, which real CLI runs never
// observe because every invocation starts a fresh process.
type uncachedGlobber struct{}

func (uncachedGlobber) Glob(baseDir, pattern string) ([]string, error) {
	return filepath.Glob(filepath.Join(baseDir, pattern))
}

// stepCountHandler records how many times each step name executed.
type stepCountHandler struct {
	runnerstep.BaseHandler
	runs map[string]int
}

func (h *stepCountHandler) Validate(*schema.WorkflowStep) error { return nil }

func (h *stepCountHandler) Execute(_ context.Context, step *schema.WorkflowStep, _ *runnerstep.Variables) (*runnerstep.StepResult, error) {
	h.runs[step.Name]++
	return runnerstep.NewStepResult("ok"), nil
}

func newStepCountContext(t *testing.T, handler *stepCountHandler) (*ExecContext, string) {
	t.Helper()
	runnerstep.Register(handler)
	original := newFreshnessChecker
	newFreshnessChecker = func() *freshness.Checker { return freshness.NewChecker(freshness.WithGlobber(uncachedGlobber{})) }
	t.Cleanup(func() { newFreshnessChecker = original })
	basePath := t.TempDir()
	componentDir := filepath.Join(basePath, "components", "terraform", "demo")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	ctx := &ExecContext{
		AtmosConfig: &schema.AtmosConfiguration{BasePath: basePath, TerraformDirAbsolutePath: filepath.Join(basePath, "components", "terraform")},
		Info:        &schema.ConfigAndStacksInfo{ComponentFromArg: "demo", FinalComponent: "demo", Stack: "dev"},
		Event:       HookEvent("before.terraform.plan"),
		HookName:    "synth",
		Outcome:     Outcome{Status: RunSuccess},
	}
	return ctx, componentDir
}

// A per-step `when:` inside a `kind: steps` hook must be honored, with the same
// lifecycle predicates the hook envelope understands.
func TestStepsHookHonorsStepWhen(t *testing.T) {
	handler := &stepCountHandler{BaseHandler: runnerstep.NewBaseHandler("count-when-test", runnerstep.CategoryCommand, false), runs: map[string]int{}}
	ctx, _ := newStepCountContext(t, handler)
	ctx.Hook = &Hook{Kind: stepsKindName, With: []any{
		map[string]any{"name": "always", "type": "count-when-test"},
		map[string]any{"name": "never", "type": "count-when-test", "when": "never"},
		map[string]any{"name": "on-failure", "type": "count-when-test", "when": "failure"},
		map[string]any{"name": "cel-false", "type": "count-when-test", "when": "component == 'other'"},
		map[string]any{"name": "cel-true", "type": "count-when-test", "when": "component == 'demo' && stack == 'dev'"},
	}}
	_, err := stepsEngine{}.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"always": 1, "cel-true": 1}, handler.runs)
}

// A step declaring inputs/artifacts runs on first use, is skipped while its sources
// are unchanged and its artifact exists, and reruns when the artifact disappears or a
// source changes. This is the same freshness contract custom commands and workflows honor.
func TestStepsHookHonorsFreshness(t *testing.T) {
	handler := &stepCountHandler{BaseHandler: runnerstep.NewBaseHandler("count-fresh-test", runnerstep.CategoryCommand, false), runs: map[string]int{}}
	ctx, componentDir := newStepCountContext(t, handler)
	lock := filepath.Join(componentDir, "package-lock.json")
	artifact := filepath.Join(componentDir, "node_modules", ".bin", "cli")
	require.NoError(t, os.WriteFile(lock, []byte("v1\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Dir(artifact), 0o755))
	require.NoError(t, os.WriteFile(artifact, []byte("bin\n"), 0o700))
	ctx.Hook = &Hook{Kind: stepsKindName, With: []any{
		map[string]any{
			"name": "install", "type": "count-fresh-test",
			"inputs":    map[string]any{"sources": []any{"package-lock.json"}},
			"artifacts": map[string]any{"paths": []any{"node_modules/.bin/cli"}},
		},
		map[string]any{"name": "synth", "type": "count-fresh-test"},
	}}

	run := func() {
		_, err := stepsEngine{}.Run(ctx)
		require.NoError(t, err)
	}
	run()
	assert.Equal(t, 1, handler.runs["install"], "first run installs")
	run()
	assert.Equal(t, 1, handler.runs["install"], "unchanged sources with an existing artifact skip the install")
	assert.Equal(t, 2, handler.runs["synth"], "steps without freshness declarations always run")

	require.NoError(t, os.RemoveAll(filepath.Join(componentDir, "node_modules")))
	run()
	assert.Equal(t, 2, handler.runs["install"], "a missing artifact reruns the step")

	require.NoError(t, os.MkdirAll(filepath.Dir(artifact), 0o755))
	require.NoError(t, os.WriteFile(artifact, []byte("bin\n"), 0o700))
	// A different length and a later mtime make the change visible even to a
	// metadata-keyed hash cache.
	require.NoError(t, os.WriteFile(lock, []byte("v2-changed\n"), 0o600))
	later := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(lock, later, later))
	run()
	assert.Equal(t, 3, handler.runs["install"], "a changed source reruns the step")
	run()
	assert.Equal(t, 3, handler.runs["install"], "state is recorded after success")
}

// A single `kind: step` hook honors the same freshness declarations.
func TestStepHookHonorsFreshness(t *testing.T) {
	handler := &stepCountHandler{BaseHandler: runnerstep.NewBaseHandler("count-fresh-single-test", runnerstep.CategoryCommand, false), runs: map[string]int{}}
	ctx, componentDir := newStepCountContext(t, handler)
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, "in.txt"), []byte("v1\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, "out.txt"), []byte("v1\n"), 0o600))
	ctx.Hook = &Hook{Kind: stepKindName, Type: "count-fresh-single-test", With: map[string]any{
		"name":      "build",
		"inputs":    map[string]any{"sources": []any{"in.txt"}},
		"artifacts": map[string]any{"paths": []any{"out.txt"}},
	}}
	for i := 0; i < 2; i++ {
		_, err := stepEngine{}.Run(ctx)
		require.NoError(t, err)
	}
	assert.Equal(t, 1, handler.runs["build"])
}
