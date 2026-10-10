package starlark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestErrorBuilderDiagnostics(t *testing.T) {
	t.Parallel()
	_, err := runSource(t, `
(errors.build("Deployment blocked")
    .with_title("Release error")
    .with_explanation("The component has no owner.")
    .with_hint("Set vars.owner before deploying.")
    .with_hint("Run with --stack <name>.")
    .with_example("vars:\n  owner: platform")
    .with_context("component", "api")
    .with_context("attempt", 2)
    .with_exit_code(3)
    .fail())
`)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.Equal(t, prefixText+": Deployment blocked", err.Error())
	assert.Equal(t, 3, errUtils.GetExitCode(err))
	assert.ElementsMatch(t, []string{
		"TITLE:Release error", "Set vars.owner before deploying.",
		"Run with --stack &lt;name&gt;.", "EXAMPLE:vars:\n  owner: platform",
	}, cockroach.GetAllHints(err))
	assert.True(t, errUtils.HasContext(err, "component", "api"))
	assert.True(t, errUtils.HasContext(err, "attempt", "2"))
	details := strings.Join(cockroach.GetAllDetails(err), "\n")
	assert.Contains(t, details, "The component has no owner.")
	assert.Contains(t, details, "test.star")
	assert.Contains(t, details, "<toplevel>")
}

func TestErrorBuilderTemplateIsImmutable(t *testing.T) {
	t.Parallel()
	_, err := runSource(t, `
base = errors.build("blocked").with_hint("base hint")
unused = base.with_hint("only on the copy").with_exit_code(9)
base.fail()
`)
	require.Error(t, err)
	assert.Equal(t, []string{"base hint"}, cockroach.GetAllHints(err))
	assert.Equal(t, 1, errUtils.GetExitCode(err))
}

func TestErrorBuilderCausePreservesDiagnostics(t *testing.T) {
	t.Parallel()
	_, err := runSource(t, `
cause = errors.build("Registry rejected the image").with_hint("Check registry permissions").with_exit_code(2)
(errors.build("Release failed")
    .with_exit_code(3)
    .with_cause(cause)
    .with_cause("image api:v1")
    .with_hint("Retry the release after fixing access")
    .fail())
`)
	require.Error(t, err)
	assert.Equal(t, prefixText+": Release failed: Registry rejected the image: image api:v1", err.Error())
	assert.ElementsMatch(t, []string{"Check registry permissions", "Retry the release after fixing access"}, cockroach.GetAllHints(err))
	assert.Equal(t, 3, errUtils.GetExitCode(err), "outer error settings override the cause regardless of builder call order")
}

func TestErrorBuilderLoadedAndParallel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "errors.star"), []byte(`
base = errors.build("blocked")
def reject(name):
    base.with_hint("fix " + name).with_explanation("reason " + name).fail()
`), 0o600))
	_, err := New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: `
load("errors.star", "reject")
steps.parallel(tasks = [steps.task(name=n, function=reject, args=[n]) for n in ["api", "web"]])
`})
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.Contains(t, err.Error(), "2 tasks failed")
	assert.ElementsMatch(t, []string{"fix api", "fix web"}, cockroach.GetAllHints(err))
	details := strings.Join(cockroach.GetAllDetails(err), "\n")
	for _, text := range []string{"reason api", "reason web", `Traceback for task "api"`, `Traceback for task "web"`} {
		assert.Contains(t, details, text)
	}
}

func TestErrorBuilderRetryKeepsFinalDiagnostics(t *testing.T) {
	t.Parallel()
	_, err := runSource(t, `
def reject():
    errors.build("blocked").with_hint("fix it").with_exit_code(7).fail()
steps.parallel(tasks=[steps.task(name="release", function=reject, retry={"max_attempts": 2, "initial_delay": "1ms"})])
`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "after 2 attempts")
	assert.Equal(t, []string{"fix it"}, cockroach.GetAllHints(err))
	assert.Equal(t, 7, errUtils.GetExitCode(err))
}

func TestErrorBuilderValidation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ source, want string }{
		{`errors.build(1)`, "want string"},
		{`errors.build("")`, "must not be empty"},
		{`errors.build("x").with_hint(1)`, "want string"},
		{`errors.build("x").with_context("a b", "x")`, "key must be an identifier"},
		{`errors.build("x").with_context("bad", [])`, "value must be a string"},
		{`errors.build("x").with_exit_code(0)`, "between 1 and 255"},
		{`errors.build("x").with_exit_code(256)`, "between 1 and 255"},
		{`errors.build("x").with_exit_code("1")`, "want int"},
		{`errors.build("x").with_cause(1)`, "cause must be a string or error builder"},
		{`errors.build("x").with_cause("")`, "cause must not be empty"},
		{`errors.build("x").fail("extra")`, "arguments"},
		{`errors.build("x").missing()`, "no .missing"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			t.Parallel()
			_, err := runSource(t, tc.source)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestErrorBuilderCreationAndDryRunDoNotFail(t *testing.T) {
	t.Parallel()
	result, err := runSource(t, `errors.build("not raised").with_hint("unused")
output = "finished"`)
	require.NoError(t, err)
	assert.Equal(t, "finished", result.Value)
	_, err = New().Execute(t.Context(), script.Spec{DryRun: true, Source: `errors.build("not raised").fail()`})
	require.NoError(t, err)
}
