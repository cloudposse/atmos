package starlark

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
)

// runGeneric executes source against the real reporter with no CI provider detected, so every call
// renders through the generic provider. The tests are not parallel: they swap the provider registry.
func runGeneric(t *testing.T, cfg *schema.AtmosConfiguration, source string) (stdout, stderr string, err error) {
	t.Helper()
	useGenericOnly(t)
	return executeCI(t, &script.Spec{CI: ci.NewReporter(cfg), Source: source})
}

func TestCIGenericAnnotateRendersOneLine(t *testing.T) {
	stdout, stderr, err := runGeneric(t, nil, `ci.annotate("warning", "deprecated module", file = "main.tf", line = 3, end_line = 5, title = "Lint")
ci.annotate("error", "no location")`)
	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "main.tf:3: warning: deprecated module (Lint)")
	assert.Contains(t, stderr, "error: no location")
}

func TestCIGenericSARIFNamesTheFile(t *testing.T) {
	dir := t.TempDir()
	report := filepath.Join(dir, "r.sarif")
	require.NoError(t, os.WriteFile(report, []byte(`{"runs":[]}`), 0o600))
	useGenericOnly(t)

	_, stderr, err := executeCI(t, &script.Spec{CI: ci.NewReporter(nil), Source: `ci.sarif(` + strconv.Quote(report) + `, category = "tf")`})
	require.NoError(t, err)
	assert.Contains(t, stderr, "SARIF")
	assert.Contains(t, stderr, report)
	assert.Contains(t, stderr, "not uploaded")
}

func TestCIGenericCheckCreatesAndUpdates(t *testing.T) {
	stdout, stderr, err := runGeneric(t, nil, `h = ci.check("ci/build", state = "in_progress", description = "building")
first = h.id
h.update("success", description = "built")
print(h.name, h.state, first > 0)`)
	require.NoError(t, err)
	assert.Equal(t, "ci/build success True\n", stdout)
	assert.Contains(t, stderr, "Check run created: ci/build")
	assert.Contains(t, stderr, "Check run completed: ci/build")
}

func TestCIGenericGroupRunsFunctionAndReturnsItsValue(t *testing.T) {
	stdout, stderr, err := runGeneric(t, nil, `def work():
    print("inside")
    return 5
print("returned", ci.group("Install", work))
print("empty title", ci.group("", lambda: 1))`)
	require.NoError(t, err)
	assert.Equal(t, "inside\nreturned 5\nempty title 1\n", stdout)
	assert.Contains(t, stderr, "Install")
}

func TestCIGenericEnvAndPathRenderExportLines(t *testing.T) {
	stdout, stderr, err := runGeneric(t, nil, `ci.env("FT_REGION", "us-east-1")
ci.env("FT_SPACED", "a b")
ci.path("/opt/ft/bin")`)
	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "export FT_REGION=us-east-1")
	assert.Contains(t, stderr, "export FT_SPACED='a b'")
	assert.Contains(t, stderr, "export PATH=/opt/ft/bin")
}

func TestCIGenericEnvAndPathAppendToFiles(t *testing.T) {
	dir := t.TempDir()
	envFile, pathFile := filepath.Join(dir, "env"), filepath.Join(dir, "path")
	useGenericOnly(t)
	t.Setenv("ATMOS_CI_ENV", envFile)
	t.Setenv("ATMOS_CI_PATH", pathFile)

	stdout, stderr, err := executeCI(t, &script.Spec{CI: ci.NewReporter(nil), Source: `ci.env("FT_REGION", "us-east-1")
ci.path("/opt/ft/bin")`})
	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.NotContains(t, stderr, "export")
	envContent, readErr := os.ReadFile(envFile)
	require.NoError(t, readErr)
	assert.Equal(t, "FT_REGION=us-east-1\n", string(envContent))
	pathContent, readErr := os.ReadFile(pathFile)
	require.NoError(t, readErr)
	assert.Equal(t, "/opt/ft/bin\n", string(pathContent))
}

func TestCIGenericOutputAndSummaryReadFilesAtCallTime(t *testing.T) {
	dir := t.TempDir()
	outputFile, summaryFile := filepath.Join(dir, "out"), filepath.Join(dir, "summary")
	useGenericOnly(t)
	reporter := ci.NewReporter(nil)
	// Set after the reporter is built: the files must be read when the call runs.
	t.Setenv("ATMOS_CI_OUTPUT", outputFile)
	t.Setenv("ATMOS_CI_SUMMARY", summaryFile)

	_, stderr, err := executeCI(t, &script.Spec{CI: reporter, Source: `ci.output("answer", "42")
ci.summary("## Done")`})
	require.NoError(t, err)
	assert.Empty(t, stderr)
	outputContent, readErr := os.ReadFile(outputFile)
	require.NoError(t, readErr)
	assert.Equal(t, "answer=42\n", string(outputContent))
	summaryContent, readErr := os.ReadFile(summaryFile)
	require.NoError(t, readErr)
	assert.Contains(t, string(summaryContent), "## Done")
}

func TestCIGenericMaskRedactsLaterOutput(t *testing.T) {
	const secret = "hunter2-masked-value"
	// The surrounding words avoid credential keywords on purpose: the process-wide masker also
	// applies pattern rules, and a phrase such as "token is" can match one of them wholesale.
	_, stderr, err := runGeneric(t, nil, `ci.mask("`+secret+`")
ci.summary("the value is `+secret+`")`)
	require.NoError(t, err)
	assert.NotContains(t, stderr, secret)
	assert.Contains(t, stderr, "the value is")
}

func TestCIGenericCommentReturnsASyntheticIDStartingAtOne(t *testing.T) {
	stdout, _, err := runGeneric(t, nil, `first = ci.comment("one", key = "a")
second = ci.comment("two", key = "b")
print(first.id, second.id, first.created)`)
	require.NoError(t, err)
	assert.Equal(t, "1 2 True\n", stdout)
}

// TestCIGenericTemplateRendersThroughRealFile drives Starlark through RenderReport against a real
// template file on disk, then through the generic provider into the summary file.
func TestCIGenericTemplateRendersThroughRealFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plan.md"),
		[]byte("## {{ .Stack }}\n{{ range .Items }}- {{ . }}\n{{ end }}count={{ .Counts.Add }}\n"), 0o600))
	summaryFile := filepath.Join(dir, "summary")
	cfg := &schema.AtmosConfiguration{}
	cfg.CI.Templates.BasePath = dir
	useGenericOnly(t)
	t.Setenv("ATMOS_CI_SUMMARY", summaryFile)

	_, stderr, err := executeCI(t, &script.Spec{CI: ci.NewReporter(cfg), Source: `ci.summary(template = "plan", data = {"Stack": "dev", "Items": ["a", "b"], "Counts": {"Add": 3}})`})
	require.NoError(t, err)
	assert.Empty(t, stderr)
	content, readErr := os.ReadFile(summaryFile)
	require.NoError(t, readErr)
	assert.Equal(t, "## dev\n- a\n- b\ncount=3\n", string(content))
}

func TestCIGenericTemplateErrorsReachTheScript(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "typo.md"), []byte("{{ .Stak }}"), 0o600))
	cfg := &schema.AtmosConfiguration{}
	cfg.CI.Templates.BasePath = dir

	t.Run("typo in a key", func(t *testing.T) {
		_, _, err := runGeneric(t, cfg, `ci.summary(template = "typo", data = {"Stack": "dev"})`)
		require.ErrorIs(t, err, errUtils.ErrTemplateEvaluation)
		require.ErrorIs(t, err, errUtils.ErrStarlark)
	})
	t.Run("missing template", func(t *testing.T) {
		_, _, err := runGeneric(t, cfg, `ci.summary(template = "absent")`)
		require.ErrorIs(t, err, errUtils.ErrCITemplateNotFound)
		require.ErrorIs(t, err, errUtils.ErrStarlark)
	})
	t.Run("no config default", func(t *testing.T) {
		withDefault := &schema.AtmosConfiguration{}
		withDefault.CI.Templates.BasePath = dir
		withDefault.CI.Summary.Template = "typo"
		_, _, err := runGeneric(t, withDefault, `ci.summary(data = {"Stack": "dev"})`)
		require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
		assert.ErrorContains(t, err, "data requires template=")
	})
}
