package docs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tfdocsTf "github.com/terraform-docs/terraform-docs/terraform"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/hooks/events"
	"github.com/cloudposse/atmos/pkg/schema"
)

var errTestHook = errors.New("hook failed")

// fakeHookRunner records fired events and the generation result passed with each.
type fakeHookRunner struct {
	fired    []events.HookEvent
	genErrs  []error
	failOn   events.HookEvent
	failWith error
	// outputExistsAtBefore records whether the output file existed when the before event fired.
	outputPath         string
	outputExistsAtFire map[events.HookEvent]bool
}

func (f *fakeHookRunner) Run(event events.HookEvent, genErr error) error {
	f.fired = append(f.fired, event)
	f.genErrs = append(f.genErrs, genErr)
	if f.outputExistsAtFire == nil {
		f.outputExistsAtFire = map[events.HookEvent]bool{}
	}
	_, statErr := os.Stat(f.outputPath)
	f.outputExistsAtFire[event] = statErr == nil
	if event == f.failOn {
		return f.failWith
	}
	return nil
}

// failingRenderer always fails to render, simulating a broken template.
type failingRenderer struct{}

func (failingRenderer) Render(string, string, map[string]any, *tfdocsTf.Module) (string, error) {
	return "", errTestHook
}

func lifecycleFixture(t *testing.T) (string, *schema.DocsGenerate) {
	t.Helper()
	dir := t.TempDir()
	return dir, &schema.DocsGenerate{BaseDir: dir, Output: filepath.Join(dir, "OUT.md")}
}

func TestRunLifecycle(t *testing.T) {
	tests := []struct {
		name          string
		failOn        events.HookEvent
		failWith      error
		breakTemplate bool
		wantFired     []events.HookEvent
		wantErr       error
		wantFile      bool
	}{
		{
			name:      "success runs before then after",
			wantFired: []events.HookEvent{events.BeforeDocsGenerate, events.AfterDocsGenerate},
			wantFile:  true,
		},
		{
			name:      "before failure aborts before generation",
			failOn:    events.BeforeDocsGenerate,
			failWith:  errTestHook,
			wantFired: []events.HookEvent{events.BeforeDocsGenerate},
			wantErr:   errTestHook,
		},
		{
			name:          "generation failure still runs after hooks",
			breakTemplate: true,
			wantFired:     []events.HookEvent{events.BeforeDocsGenerate, events.AfterDocsGenerate},
			wantErr:       errUtils.ErrRenderTemplate,
		},
		{
			name:      "after failure is returned after a successful generation",
			failOn:    events.AfterDocsGenerate,
			failWith:  errTestHook,
			wantFired: []events.HookEvent{events.BeforeDocsGenerate, events.AfterDocsGenerate},
			wantErr:   errTestHook,
			wantFile:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, dg := lifecycleFixture(t)
			var renderer templateRenderer = mockRenderer{}
			if tt.breakTemplate {
				renderer = failingRenderer{}
			}
			runner := &fakeHookRunner{failOn: tt.failOn, failWith: tt.failWith, outputPath: dg.Output}

			err := runLifecycle(&schema.AtmosConfiguration{}, dir, dg, renderer, runner)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantFired, runner.fired)
			if tt.wantFile {
				assert.FileExists(t, dg.Output)
			} else {
				assert.NoFileExists(t, dg.Output)
			}
			assert.False(t, runner.outputExistsAtFire[events.BeforeDocsGenerate], "before hooks run before the file is written")
			if len(runner.genErrs) == 2 {
				assert.NoError(t, runner.genErrs[0], "before events carry no generation result")
				assert.Equal(t, tt.breakTemplate, runner.genErrs[1] != nil, "after events carry the generation result")
			}
		})
	}
}

func TestRunLifecycleWithoutRunnerGenerates(t *testing.T) {
	dir, dg := lifecycleFixture(t)
	require.NoError(t, runLifecycle(&schema.AtmosConfiguration{}, dir, dg, mockRenderer{}, nil))
	assert.FileExists(t, dg.Output)
}

func TestGenerateWithHooksUsesRegisteredFactory(t *testing.T) {
	t.Cleanup(func() { RegisterHookRunnerFactory(nil) })

	dir, dg := lifecycleFixture(t)
	dg.Hooks = map[string]any{"example": map[string]any{}}
	var got *HookRunnerRequest
	runner := &fakeHookRunner{outputPath: dg.Output}
	RegisterHookRunnerFactory(func(req *HookRunnerRequest) (HookRunner, error) {
		got = req
		return runner, nil
	})
	cmd := &cobra.Command{}

	require.NoError(t, GenerateWithHooks(&schema.AtmosConfiguration{}, cmd, "readme", dg))

	require.NotNil(t, got)
	assert.Equal(t, "readme", got.DocType)
	assert.Equal(t, dir, got.BaseDir)
	assert.Equal(t, dg.Output, got.OutputPath)
	assert.Same(t, cmd, got.Cmd)
	assert.Equal(t, dg.Hooks, got.Hooks)
	assert.Equal(t, []events.HookEvent{events.BeforeDocsGenerate, events.AfterDocsGenerate}, runner.fired)
}

func TestGenerateWithHooksFactoryError(t *testing.T) {
	t.Cleanup(func() { RegisterHookRunnerFactory(nil) })

	_, dg := lifecycleFixture(t)
	RegisterHookRunnerFactory(func(*HookRunnerRequest) (HookRunner, error) { return nil, errTestHook })

	require.ErrorIs(t, GenerateWithHooks(&schema.AtmosConfiguration{}, nil, "readme", dg), errTestHook)
	assert.NoFileExists(t, dg.Output)
}

func TestGenerateWithHooksWithoutFactory(t *testing.T) {
	RegisterHookRunnerFactory(nil)
	_, dg := lifecycleFixture(t)
	require.NoError(t, GenerateWithHooks(&schema.AtmosConfiguration{}, nil, "readme", dg))
	assert.FileExists(t, dg.Output)
}

func TestGenerateWithHooksConfiguredHooksWithoutFactoryFails(t *testing.T) {
	RegisterHookRunnerFactory(nil)
	_, dg := lifecycleFixture(t)
	dg.Hooks = map[string]any{"publish": map[string]any{"kind": "step"}}

	err := GenerateWithHooks(&schema.AtmosConfiguration{}, nil, "readme", dg)

	require.ErrorIs(t, err, errUtils.ErrDocsHooksUnavailable)
	assert.NoFileExists(t, dg.Output, "nothing may be generated when configured hooks cannot run")
}
