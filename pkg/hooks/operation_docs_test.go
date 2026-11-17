package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/docs"
	"github.com/cloudposse/atmos/pkg/schema"
)

// docsGenerateFixture writes a template and returns a docs.generate entry that renders it into dir.
func docsGenerateFixture(t *testing.T, hooks map[string]any) *schema.DocsGenerate {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md.gotmpl"), []byte("hello docs"), 0o600))
	return &schema.DocsGenerate{
		BaseDir:  dir,
		Template: "README.md.gotmpl",
		Output:   filepath.Join(dir, "README.md"),
		Hooks:    hooks,
	}
}

func TestGenerateWithHooksEndToEnd(t *testing.T) {
	recorder, url := newRecorderServer(t)
	dg := docsGenerateFixture(t, map[string]any{
		"before": httpStepHook(url+"/before/{{ .docs.type }}", map[string]any{"events": []any{"before.docs.generate"}}),
		"after":  httpStepHook(url+"/after", map[string]any{"events": []any{"after.docs.generate"}}),
	})

	require.NoError(t, docs.GenerateWithHooks(&schema.AtmosConfiguration{}, &cobra.Command{}, "readme", dg))

	assert.Equal(t, []string{"/before/readme", "/after"}, recorder.seen())
	content, err := os.ReadFile(dg.Output)
	require.NoError(t, err)
	assert.Equal(t, "hello docs", string(content))
}

func TestGenerateWithHooksBeforeFailureWritesNothing(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	dg := docsGenerateFixture(t, map[string]any{
		"gate": map[string]any{
			"kind": "command", "command": exe, "events": []any{"before.docs.generate"},
			"env": map[string]any{"_ATMOS_TEST_EXIT_ONE": "1"}, "on_failure": OnFailureFail,
		},
	})

	require.Error(t, docs.GenerateWithHooks(&schema.AtmosConfiguration{}, &cobra.Command{}, "readme", dg))
	assert.NoFileExists(t, dg.Output)
}

func TestGenerateWithHooksAfterRunsOnFailure(t *testing.T) {
	recorder, url := newRecorderServer(t)
	dg := docsGenerateFixture(t, map[string]any{
		"on-fail":    httpStepHook(url+"/failure", map[string]any{"events": []any{"after.docs.generate"}, "when": "failure"}),
		"on-success": httpStepHook(url+"/success", map[string]any{"events": []any{"after.docs.generate"}}),
		"always":     httpStepHook(url+"/always", map[string]any{"events": []any{"after.docs.generate"}, "when": "always"}),
	})
	dg.Template = "missing.gotmpl"

	err := docs.GenerateWithHooks(&schema.AtmosConfiguration{}, &cobra.Command{}, "readme", dg)
	require.Error(t, err)
	assert.NoFileExists(t, dg.Output)
	assert.ElementsMatch(t, []string{"/failure", "/always"}, recorder.seen())
}

func TestGenerateWithHooksPolicyDenialBlocksGeneration(t *testing.T) {
	recorder, url := newRecorderServer(t)
	t.Cleanup(func() { docs.RegisterHookRunnerFactory(newDocsHookRunner) })
	docs.RegisterHookRunnerFactory(func(req *docs.HookRunnerRequest) (docs.HookRunner, error) {
		runner, err := newDocsHookRunner(req)
		if err != nil {
			return nil, err
		}
		runner.(*docsHookRunner).hooks = docsPolicyHooks(t, url, true)
		return runner, nil
	})
	dg := docsGenerateFixture(t, nil)

	err := docs.GenerateWithHooks(&schema.AtmosConfiguration{}, &cobra.Command{}, "readme", dg)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	assert.NoFileExists(t, dg.Output)
	assert.Empty(t, recorder.seen())
}

func TestGenerateWithHooksInvalidEventRejected(t *testing.T) {
	dg := docsGenerateFixture(t, map[string]any{
		"bad": httpStepHook("http://127.0.0.1:1", map[string]any{"events": []any{"after.docs.publish"}}),
	})
	err := docs.GenerateWithHooks(&schema.AtmosConfiguration{}, &cobra.Command{}, "readme", dg)
	require.ErrorIs(t, err, errUtils.ErrUnsupportedHookEvent)
	assert.NoFileExists(t, dg.Output, "invalid hook configuration must fail before generation")
}

func TestDocsOperationContext(t *testing.T) {
	op := docsOperationContext(&docs.HookRunnerRequest{DocType: "readme", BaseDir: "/base", OutputPath: "/base/README.md"})
	assert.Equal(t, "docs", op.Type)
	assert.Equal(t, "readme", op.Name)
	assert.Equal(t, map[string]string{
		"ATMOS_DOCS_TYPE": "readme", "ATMOS_DOCS_OUTPUT_PATH": "/base/README.md", "ATMOS_DOCS_BASE_DIR": "/base",
	}, op.Env)
	assert.Equal(t, "/base/README.md", op.Data["output_path"])
}
