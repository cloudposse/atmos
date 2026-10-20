package stack

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
)

// fakeCIReporter records the summary and outputs the lifecycle publishes.
type fakeCIReporter struct {
	ci.Reporter
	summaries []string
	outputs   [][2]string
}

func (f *fakeCIReporter) Summary(markdown string) (ci.Receipt, error) {
	f.summaries = append(f.summaries, markdown)
	return ci.Receipt{}, nil
}

func (f *fakeCIReporter) Output(key, value string) (ci.Receipt, error) {
	f.outputs = append(f.outputs, [2]string{key, value})
	return ci.Receipt{}, nil
}

func (f *fakeCIReporter) outputMap() map[string]string {
	m := map[string]string{}
	for _, pair := range f.outputs {
		m[pair[0]] = pair[1]
	}
	return m
}

func clearCIEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"ATMOS_CI", "CI", "GITHUB_ACTIONS"} {
		t.Setenv(name, "")
	}
}

func ciTestPlan(t *testing.T, nodes ...*dependency.Node) *lifecycle.Plan {
	t.Helper()
	graph := dependency.NewGraph()
	for _, n := range nodes {
		require.NoError(t, graph.AddNode(n))
	}
	return &lifecycle.Plan{Graph: graph}
}

func ciTestNodes() (vpc, app *dependency.Node) {
	return &dependency.Node{ID: "vpc-dev", Type: "terraform", Stack: "dev", Component: "vpc"},
		&dependency.Node{ID: "app-dev", Type: "helm", Stack: "dev", Component: "app"}
}

func TestLifecycleCommandsExposeCIFlagAndSkipInvocationGroup(t *testing.T) {
	root := &cobra.Command{Use: "atmos"}
	group := &cobra.Command{Use: "stack"}
	other := &cobra.Command{Use: "version", Run: func(*cobra.Command, []string) {}}
	for _, verb := range []string{"deploy", "destroy"} {
		cmd := newLifecycleCommand(verb)
		flag := cmd.Flag("ci")
		require.NotNil(t, flag, verb)
		assert.Equal(t, "false", flag.DefValue)
		assert.Equal(t, "Enable CI mode for automated pipelines (writes job summary, outputs)", flag.Usage)
		assert.Equal(t, "true", cmd.Annotations[annotationSkipInvocationGroup])
		group.AddCommand(cmd)
	}
	root.AddCommand(group, other)

	assert.True(t, SkipsInvocationGroup(root, []string{"stack", "deploy", "--all", "--stack=dev"}))
	assert.True(t, SkipsInvocationGroup(root, []string{"stack", "destroy", "--all"}))
	assert.False(t, SkipsInvocationGroup(root, []string{"version"}))
	assert.False(t, SkipsInvocationGroup(root, []string{"stack"}))
	assert.False(t, SkipsInvocationGroup(root, []string{"unknown", "deploy"}))
	assert.False(t, SkipsInvocationGroup(root, nil))
	assert.False(t, SkipsInvocationGroup(nil, []string{"stack", "deploy"}))
}

func TestLifecycleCIModePrecedence(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		args []string
		want bool
	}{
		{"default off", nil, nil, false},
		{"flag on", nil, []string{"--ci"}, true},
		{"ATMOS_CI on", map[string]string{"ATMOS_CI": "true"}, nil, true},
		{"CI on", map[string]string{"CI": "true"}, nil, true},
		{"explicit false beats env", map[string]string{"ATMOS_CI": "true", "CI": "true"}, []string{"--ci=false"}, false},
		{"explicit true beats env false", map[string]string{"ATMOS_CI": "false"}, []string{"--ci=true"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearCIEnvironment(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			opts, err := lifecycleTestCommand(t, "deploy", append([]string{"--all"}, tc.args...)...)
			require.NoError(t, err)
			assert.Equal(t, tc.want, opts.ci)
		})
	}
}

func TestWithCIMode(t *testing.T) {
	base := []string{"KEEP=1", "ATMOS_CI=false"}
	got := withCIMode(append([]string{}, base...), true)
	assert.Equal(t, []string{"KEEP=1", "ATMOS_CI=true", ci.WorkerModeEnv + "=true"}, got)
	// Off overrides automatic detection without dropping provider metadata.
	assert.Equal(t, []string{"KEEP=1", "ATMOS_CI=false", ci.WorkerModeEnv + "=false"}, withCIMode(base, false))
	assert.Equal(t, []string{"KEEP=1", "ATMOS_CI=false"}, base)
}

func TestLifecycleEnvironmentSetsCIModeForChildren(t *testing.T) {
	clearCIEnvironment(t)
	t.Setenv("ATMOS_CI", "false")
	cmd := &cobra.Command{Use: "deploy"}
	cmd.Flags().String(flagSkipHooks, "", "")
	assert.Equal(t, "true", envValue(lifecycleEnvironment(cmd, true), "ATMOS_CI"))
	assert.Equal(t, "false", envValue(lifecycleEnvironment(cmd, false), "ATMOS_CI"))
	// A single entry must remain: children never see duplicated keys.
	count := 0
	for _, entry := range lifecycleEnvironment(cmd, true) {
		if strings.HasPrefix(entry, "ATMOS_CI=") {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestNewLifecycleCIDoesNothingWhenOff(t *testing.T) {
	vpc, _ := ciTestNodes()
	plan := ciTestPlan(t, vpc)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	for name, tc := range map[string]struct {
		config *schema.AtmosConfiguration
		opts   *lifecycleOptions
	}{
		"ci mode off":        {&schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}, &lifecycleOptions{verb: "deploy"}},
		"ci.enabled false":   {&schema.AtmosConfiguration{}, &lifecycleOptions{verb: "deploy", ci: true}},
		"neither is enabled": {&schema.AtmosConfiguration{}, &lifecycleOptions{verb: "deploy"}},
	} {
		t.Run(name, func(t *testing.T) {
			reporter := &fakeCIReporter{}
			state, err := newLifecycleCI(tc.config, tc.opts, plan, os.Environ(), reporter)
			require.NoError(t, err)
			assert.Nil(t, state)
			// A nil state is inert, so the caller needs no branches.
			assert.NoError(t, state.publish([]scheduler.Result{{NodeID: vpc.ID, Node: *vpc, Status: scheduler.StatusSucceeded}}, 0))
			state.cleanup()
			assert.Empty(t, reporter.summaries)
			assert.Empty(t, reporter.outputs)
		})
	}
	entries, err := os.ReadDir(tmp)
	require.NoError(t, err)
	assert.Empty(t, entries, "no temporary files are created")
}

func TestLifecycleCINodeEnvironment(t *testing.T) {
	vpc, app := ciTestNodes()
	plan := ciTestPlan(t, vpc, app)
	config := &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}
	opts := &lifecycleOptions{verb: "deploy", ci: true}

	t.Run("generic provider", func(t *testing.T) {
		state, err := newLifecycleCI(config, opts, plan, []string{"PATH=/bin"}, &fakeCIReporter{})
		require.NoError(t, err)
		require.NotNil(t, state)
		t.Cleanup(state.cleanup)
		vpcEnv, appEnv := state.nodeEnvironment(vpc), state.nodeEnvironment(app)
		require.Len(t, vpcEnv, 2)
		require.Len(t, appEnv, 2)
		assert.Equal(t, state.files[vpc.ID].output, envValue(vpcEnv, "ATMOS_CI_OUTPUT"))
		assert.Equal(t, state.files[vpc.ID].summary, envValue(vpcEnv, "ATMOS_CI_SUMMARY"))
		assert.Empty(t, envValue(vpcEnv, "GITHUB_OUTPUT"))
		assert.Empty(t, envValue(vpcEnv, "GITHUB_STEP_SUMMARY"))
		// Every child gets files of its own, in its own directory.
		assert.NotEqual(t, envValue(vpcEnv, "ATMOS_CI_OUTPUT"), envValue(appEnv, "ATMOS_CI_OUTPUT"))
		assert.NotEqual(t, envValue(vpcEnv, "ATMOS_CI_SUMMARY"), envValue(appEnv, "ATMOS_CI_SUMMARY"))
		assert.Nil(t, state.nodeEnvironment(&dependency.Node{ID: "unknown"}))
	})

	t.Run("github variables are redirected only when the parent has them", func(t *testing.T) {
		parent := []string{"GITHUB_OUTPUT=/runner/output", "GITHUB_STEP_SUMMARY=/runner/summary"}
		state, err := newLifecycleCI(config, opts, plan, parent, &fakeCIReporter{})
		require.NoError(t, err)
		t.Cleanup(state.cleanup)
		env := state.nodeEnvironment(vpc)
		assert.Equal(t, state.files[vpc.ID].output, envValue(env, "GITHUB_OUTPUT"))
		assert.Equal(t, state.files[vpc.ID].summary, envValue(env, "GITHUB_STEP_SUMMARY"))
		assert.NotContains(t, env, "GITHUB_OUTPUT=/runner/output")
	})

	t.Run("cleanup removes the capture directory", func(t *testing.T) {
		state, err := newLifecycleCI(config, opts, plan, nil, &fakeCIReporter{})
		require.NoError(t, err)
		require.DirExists(t, state.dir)
		state.cleanup()
		assert.NoDirExists(t, state.dir)
	})
}

func TestParseCIOutputs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    map[string]string
	}{
		{"empty", "", map[string]string{}},
		{"key value", "stack=dev\ncomponent=vpc\nexit_code=0\n", map[string]string{"stack": "dev", "component": "vpc", "exit_code": "0"}},
		{"value containing equals and heredoc marker", "url=https://x/?a=b\nexpr=a<<b\n", map[string]string{"url": "https://x/?a=b", "expr": "a<<b"}},
		{"empty value", "empty=\n", map[string]string{"empty": ""}},
		{"heredoc", "output_cidrs<<EOF\n10.0.0.0/16\n10.1.0.0/16\nEOF\nafter=1\n", map[string]string{"output_cidrs": "10.0.0.0/16\n10.1.0.0/16", "after": "1"}},
		{"extended delimiter", "body<<EOF_\nline EOF inside\nEOF\nEOF_\nnext=2\n", map[string]string{"body": "line EOF inside\nEOF", "next": "2"}},
		{"github delimiter and CRLF", "value<<ghadelimiter_1\r\nfirst\r\nsecond\r\nghadelimiter_1\r\nok=yes\r\n", map[string]string{"value": "first\nsecond", "ok": "yes"}},
		{"last write wins", "a=1\na=2\n", map[string]string{"a": "2"}},
		{"blank lines ignored", "\n\na=1\n\n", map[string]string{"a": "1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseCIOutputs(tc.content))
		})
	}
}

func writeChildFiles(t *testing.T, state *lifecycleCI, node *dependency.Node, output, summary string) {
	t.Helper()
	files := state.files[node.ID]
	if output != "" {
		require.NoError(t, os.WriteFile(files.output, []byte(output), ciFilePermissions))
	}
	if summary != "" {
		require.NoError(t, os.WriteFile(files.summary, []byte(summary), ciFilePermissions))
	}
}

func TestLifecycleCIPublishCombinesSummaryAndOutputs(t *testing.T) {
	vpc, app := ciTestNodes()
	skipped := &dependency.Node{ID: "db-dev", Type: "terraform", Stack: "dev", Component: "db"}
	plan := ciTestPlan(t, vpc, app, skipped)
	reporter := &fakeCIReporter{}
	state, err := newLifecycleCI(&schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}, &lifecycleOptions{verb: "deploy", ci: true}, plan, nil, reporter)
	require.NoError(t, err)
	t.Cleanup(state.cleanup)
	writeChildFiles(t, state, vpc, "stack=dev\nsummary<<EOF\n## repeated\nEOF\nexit_code=0\noutput_cidrs<<EOF\n10.0.0.0/16\n10.1.0.0/16\nEOF\n", "## vpc plan\n\n3 to add\n")
	writeChildFiles(t, state, app, "exit_code=1\n", "")

	results := []scheduler.Result{
		{NodeID: vpc.ID, Node: *vpc, Status: scheduler.StatusSucceeded},
		{NodeID: app.ID, Node: *app, Status: scheduler.StatusFailed},
		{NodeID: skipped.ID, Node: *skipped, Status: scheduler.StatusSkipped},
	}
	require.NoError(t, state.publish(results, 2))

	require.Len(t, reporter.summaries, 1, "one combined summary for the whole run")
	summary := reporter.summaries[0]
	assert.True(t, strings.HasPrefix(summary, "### atmos stack deploy\n"))
	assert.Contains(t, summary, "| Component | Kind | Stack | Result |\n| --- | --- | --- | --- |\n")
	assert.Contains(t, summary, "| vpc | terraform | dev | succeeded |\n")
	assert.Contains(t, summary, "| app | helm | dev | failed |\n")
	assert.Contains(t, summary, "| db | terraform | dev | skipped |\n")
	assert.Contains(t, summary, "1 succeeded, 1 failed, 1 skipped, 2 excluded")
	assert.Contains(t, summary, "<details><summary>terraform/dev/vpc</summary>\n\n## vpc plan\n\n3 to add\n\n</details>")
	assert.Equal(t, 1, strings.Count(summary, "<details>"), "children without a summary add no block")
	assert.Less(t, strings.Index(summary, "| vpc |"), strings.Index(summary, "<details>"))

	outputs := reporter.outputMap()
	assert.Len(t, reporter.outputs, 8, "outputs are written exactly once")
	assert.Equal(t, "3", outputs["components_total"])
	assert.Equal(t, "1", outputs["components_succeeded"])
	assert.Equal(t, "1", outputs["components_failed"])
	assert.Equal(t, "1", outputs["components_skipped"])
	assert.Equal(t, "2", outputs["components_excluded"])
	assert.Equal(t, "0", outputs["components_deleted"])
	assert.Equal(t, "false", outputs["success"])

	var decoded []ciResult
	require.NoError(t, json.Unmarshal([]byte(outputs["results"]), &decoded))
	require.Len(t, decoded, 3)
	assert.Equal(t, ciResult{Kind: "terraform", Stack: "dev", Component: "vpc", Status: "succeeded", Outputs: map[string]string{"stack": "dev", "exit_code": "0", "output_cidrs": "10.0.0.0/16\n10.1.0.0/16"}}, decoded[0])
	assert.Equal(t, ciResult{Kind: "helm", Stack: "dev", Component: "app", Status: "failed", Outputs: map[string]string{"exit_code": "1"}}, decoded[1])
	assert.Equal(t, ciResult{Kind: "terraform", Stack: "dev", Component: "db", Status: "skipped", Outputs: map[string]string{}}, decoded[2])
	assert.NotContains(t, outputs["results"], "\n", "results is compact JSON")
	assert.NotContains(t, outputs["results"], "repeated", "the per-child summary output lives in the details block only")
	assert.Contains(t, outputs["results"], `"outputs":{}`)
}

func TestLifecycleCIPublishSucceedsWhenNothingFailed(t *testing.T) {
	vpc, _ := ciTestNodes()
	reporter := &fakeCIReporter{}
	state, err := newLifecycleCI(&schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}, &lifecycleOptions{verb: "destroy", ci: true}, ciTestPlan(t, vpc), nil, reporter)
	require.NoError(t, err)
	t.Cleanup(state.cleanup)
	require.NoError(t, state.publish([]scheduler.Result{{NodeID: vpc.ID, Node: *vpc, Status: scheduler.StatusSucceeded}}, 0))
	outputs := reporter.outputMap()
	assert.Equal(t, "true", outputs["success"])
	assert.Equal(t, "0", outputs["components_failed"])
	assert.True(t, strings.HasPrefix(reporter.summaries[0], "### atmos stack destroy\n"))

	// Publishing with no results still yields an empty array, never null.
	reporter.outputs = nil
	require.NoError(t, state.publish(nil, 0))
	assert.Equal(t, "[]", reporter.outputMap()["results"])
	assert.Equal(t, "0", reporter.outputMap()["components_total"])
}

func TestNewLifecycleCIReportsUnusableTemporaryDirectory(t *testing.T) {
	vpc, _ := ciTestNodes()
	missing := filepath.Join(t.TempDir(), "missing")
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, missing)
	}
	state, err := newLifecycleCI(&schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}, &lifecycleOptions{verb: "deploy", ci: true}, ciTestPlan(t, vpc), nil, &fakeCIReporter{})
	require.Error(t, err)
	assert.Nil(t, state)
}

func TestReadCIFileToleratesMissingFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output")
	require.NoError(t, os.WriteFile(path, []byte("a=1\n"), ciFilePermissions))
	assert.Equal(t, "a=1\n", readCIFile(path))
	assert.Empty(t, readCIFile(""), "a child without capture files has no content")
	assert.Empty(t, readCIFile(filepath.Join(dir, "never-written")))
}

func TestLifecycleCIPublishIgnoresResultsWithoutCaptureFiles(t *testing.T) {
	vpc, app := ciTestNodes()
	reporter := &fakeCIReporter{}
	state, err := newLifecycleCI(&schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}, &lifecycleOptions{verb: "deploy", ci: true}, ciTestPlan(t, vpc), nil, reporter)
	require.NoError(t, err)
	t.Cleanup(state.cleanup)

	// The second result was never part of the plan, so it has no capture files.
	require.NoError(t, state.publish([]scheduler.Result{
		{NodeID: vpc.ID, Node: *vpc, Status: scheduler.StatusSucceeded},
		{NodeID: app.ID, Node: *app, Status: scheduler.StatusFailed},
	}, 0))
	var decoded []ciResult
	require.NoError(t, json.Unmarshal([]byte(reporter.outputMap()["results"]), &decoded))
	require.Len(t, decoded, 2)
	assert.Equal(t, map[string]string{}, decoded[1].Outputs)
	assert.NotContains(t, reporter.summaries[0], "<details>")
	assert.Equal(t, "false", reporter.outputMap()["success"])
}

func TestLifecycleReporterFallsBackToTheGenericProvider(t *testing.T) {
	clearCIEnvironment(t)
	dir := t.TempDir()
	output, summary := filepath.Join(dir, "output"), filepath.Join(dir, "summary")
	t.Setenv("ATMOS_CI_OUTPUT", output)
	t.Setenv("ATMOS_CI_SUMMARY", summary)
	reporter := newLifecycleReporter(&schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}})
	require.NotNil(t, reporter)

	// Without a detected provider, forced CI mode still writes to the files the generic provider names.
	_, err := reporter.Output("components_total", "2")
	require.NoError(t, err)
	_, err = reporter.Summary("### atmos stack deploy\n")
	require.NoError(t, err)
	written, err := os.ReadFile(output)
	require.NoError(t, err)
	assert.Equal(t, "components_total=2\n", string(written))
	rendered, err := os.ReadFile(summary)
	require.NoError(t, err)
	assert.Equal(t, "### atmos stack deploy\n", string(rendered))
}

// ciRun returns a run over the platform stack's two components with CI mode on and the parent's
// output and summary files redirected into a temporary directory.
func ciRun(t *testing.T) (run *lifecycleRun, outputFile, summaryFile string) {
	t.Helper()
	clearCIEnvironment(t)
	run, _ = lifecycleChildRun(t, "deploy", 0, func(o *lifecycleOptions) { o.ci = true })
	run.config.CI = schema.CIConfig{Enabled: true}
	dir := t.TempDir()
	outputFile, summaryFile = filepath.Join(dir, "parent-output"), filepath.Join(dir, "parent-summary")
	t.Setenv("ATMOS_CI_OUTPUT", outputFile)
	t.Setenv("ATMOS_CI_SUMMARY", summaryFile)
	t.Setenv("ATMOS_CI", "true")
	return run, outputFile, summaryFile
}

func TestExecuteLifecycleChildrenPublishesCombinedCIResults(t *testing.T) {
	run, outputFile, summaryFile := ciRun(t)
	t.Setenv(childCaptureEnv, "1")
	captureLifecycle(t)

	require.NoError(t, executeLifecycleChildren(lifecycleTestCommandWithContext(), run))

	outputs, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	parsed := parseCIOutputs(string(outputs))
	assert.Equal(t, "2", parsed["components_total"])
	assert.Equal(t, "2", parsed["components_succeeded"])
	assert.Equal(t, "0", parsed["components_failed"])
	assert.Equal(t, "1", parsed["components_excluded"], "the packer component without a handler is counted")
	assert.Equal(t, "true", parsed["success"])
	// Each child wrote to its own private files, and the parent collected them per component.
	var results []ciResult
	require.NoError(t, json.Unmarshal([]byte(parsed["results"]), &results))
	require.Len(t, results, 2)
	for _, result := range results {
		assert.Equal(t, "succeeded", result.Status)
		assert.Equal(t, map[string]string{"child_key": "child_value"}, result.Outputs, "the duplicated summary output is dropped")
	}

	summary, err := os.ReadFile(summaryFile)
	require.NoError(t, err)
	assert.Contains(t, string(summary), "### atmos stack deploy")
	assert.Contains(t, string(summary), "| registry | terraform | platform | succeeded |")
	assert.Contains(t, string(summary), "<details><summary>helmfile/platform/ingress</summary>\n\n## child summary\n\n</details>")
	assert.Contains(t, string(summary), "2 succeeded, 0 failed, 0 skipped, 1 excluded")
}

func TestExecuteLifecycleChildrenPublishesFailuresToo(t *testing.T) {
	run, outputFile, _ := ciRun(t)
	t.Setenv(childExitEnv, "4")
	captureLifecycle(t)

	err := executeLifecycleChildren(lifecycleTestCommandWithContext(), run)
	require.ErrorIs(t, err, errUtils.ErrComponentExecutionFailed)
	outputs, readErr := os.ReadFile(outputFile)
	require.NoError(t, readErr)
	parsed := parseCIOutputs(string(outputs))
	assert.Equal(t, "false", parsed["success"])
	assert.Equal(t, "1", parsed["components_failed"])
	assert.Equal(t, "1", parsed["components_skipped"])
}

func TestExecuteLifecycleChildrenKeepsGoingWhenPublishingFails(t *testing.T) {
	run, _, _ := ciRun(t)
	// The parent's output file lives in a directory that does not exist.
	t.Setenv("ATMOS_CI_OUTPUT", filepath.Join(t.TempDir(), "missing", "output"))
	output := captureLifecycle(t)

	require.NoError(t, executeLifecycleChildren(lifecycleTestCommandWithContext(), run), "a reporting failure does not fail the operation")
	assert.Contains(t, output.String(), "Failed to publish the CI summary and outputs")
	assert.Contains(t, output.String(), "2 succeeded, 0 failed, 0 skipped, 1 excluded")
}

func TestExecuteLifecycleChildrenStopsWhenCICaptureCannotBePrepared(t *testing.T) {
	run, _, _ := ciRun(t)
	log := os.Getenv(childLogEnv)
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, filepath.Join(t.TempDir(), "missing"))
	}
	captureLifecycle(t)

	require.Error(t, executeLifecycleChildren(lifecycleTestCommandWithContext(), run))
	assert.NoFileExists(t, log, "no child starts without somewhere to capture its CI output")
}

func TestEmptyLifecyclePublishesCIResultsWithoutStartingChildren(t *testing.T) {
	for _, verb := range []string{"deploy", "destroy"} {
		for _, mode := range []string{"enabled", "disabled", "preview"} {
			t.Run(verb+"/"+mode, func(t *testing.T) {
				clearCIEnvironment(t)
				dir := t.TempDir()
				output, summary := filepath.Join(dir, "output"), filepath.Join(dir, "summary")
				t.Setenv("ATMOS_CI", "true")
				t.Setenv("ATMOS_CI_OUTPUT", output)
				t.Setenv("ATMOS_CI_SUMMARY", summary)
				run, childLog := lifecycleChildRun(t, verb, 0, nil)
				run.config = &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}
				run.plan = ciTestPlan(t)
				run.options.ci = mode != "disabled"
				run.options.dryRun = mode == "preview"
				captureLifecycle(t)
				require.NoError(t, runLifecyclePlan(lifecycleTestCommandWithContext(), run))
				assert.NoFileExists(t, childLog)
				if mode != "enabled" {
					assert.NoFileExists(t, output)
					assert.NoFileExists(t, summary)
					return
				}
				content, err := os.ReadFile(output)
				require.NoError(t, err)
				outputs := parseCIOutputs(string(content))
				assert.Equal(t, "true", outputs["success"])
				assert.Equal(t, "[]", outputs["results"])
				for _, key := range []string{"components_total", "components_succeeded", "components_failed", "components_skipped", "components_excluded"} {
					assert.Equal(t, "0", outputs[key], key)
				}
				body, err := os.ReadFile(summary)
				require.NoError(t, err)
				assert.Contains(t, string(body), "atmos stack "+verb)
				assert.Contains(t, string(body), "0 succeeded, 0 failed, 0 skipped, 0 excluded")
			})
		}
	}
}
