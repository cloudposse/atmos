package stack

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"al.essio.dev/pkg/shellescape"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
)

// lifecyclePackageDir is the package directory, captured before any test changes the working directory.
var lifecyclePackageDir, _ = os.Getwd()

// useLifecycleFixture switches to the stack-lifecycle-field-test fixture and points Atmos at it.
func useLifecycleFixture(t *testing.T) string {
	t.Helper()
	fixture := filepath.Join(lifecyclePackageDir, "..", "..", "tests", "fixtures", "scenarios", "stack-lifecycle-field-test")
	require.DirExists(t, fixture)
	t.Chdir(fixture)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", fixture)
	t.Setenv("ATMOS_BASE_PATH", fixture)
	e.ClearFindStacksMapCache()
	t.Cleanup(e.ClearFindStacksMapCache)
	return fixture
}

// registerFixtureCommands mirrors what the root command does at startup: it registers the built-in
// commands and attaches the fixture's custom script commands.
func registerFixtureCommands(t *testing.T) (root, script *cobra.Command) {
	t.Helper()
	root = &cobra.Command{Use: "atmos"}
	require.NoError(t, internal.RegisterAll(root))
	script = &cobra.Command{Use: "script"}
	for _, name := range []string{"deploy", "destroy"} {
		script.AddCommand(&cobra.Command{Use: name, Annotations: map[string]string{"customCommand": "true"}})
	}
	root.AddCommand(script)
	return root, script
}

// lifecycleFixturePlan builds a plan for the stack-lifecycle-field-test fixture with
// the real built-in handlers and the fixture's custom script commands.
func lifecycleFixturePlan(t *testing.T, verb string, selection lifecycle.Selection) (*lifecycle.Plan, error) {
	t.Helper()
	useLifecycleFixture(t)

	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Custom commands are attached by the root command at startup; mirror that here.
	_, script := registerFixtureCommands(t)

	stacks, handlers, err := loadLifecycleInventory(script, &config, verb, nil)
	require.NoError(t, err)
	return lifecycle.BuildPlan(stacks, handlers, selection, "{{")
}

func planLabels(t *testing.T, plan *lifecycle.Plan) []string {
	t.Helper()
	order, err := plan.Graph.TopologicalSort()
	require.NoError(t, err)
	labels := make([]string, 0, len(order))
	for i := range order {
		labels = append(labels, lifecycle.Label(&order[i]))
	}
	return labels
}

func assertBefore(t *testing.T, labels []string, first, second string) {
	t.Helper()
	i, j := slices.Index(labels, first), slices.Index(labels, second)
	require.NotEqual(t, -1, i, "%s missing from %v", first, labels)
	require.NotEqual(t, -1, j, "%s missing from %v", second, labels)
	assert.Less(t, i, j, "%s must run before %s in %v", first, second, labels)
}

func TestLifecycleFixtureDeployPlan(t *testing.T) {
	plan, err := lifecycleFixturePlan(t, "deploy", lifecycle.Selection{})
	require.NoError(t, err)
	labels := planLabels(t, plan)

	// Mixed kinds, including Helmfile and the custom script kind, are ordered by dependencies.
	assertBefore(t, labels, "terraform/dev/network", "terraform/dev/database")
	assertBefore(t, labels, "terraform/dev/network", "terraform/dev/cache")
	assertBefore(t, labels, "terraform/dev/database", "terraform/dev/app")
	assertBefore(t, labels, "terraform/dev/app", "script/dev/smoke")
	assertBefore(t, labels, "terraform/dev/network", "terraform/prod/app")
	assertBefore(t, labels, "terraform/platform/registry", "helmfile/platform/ingress")
	assert.Equal(t, []string{"helmfile", "apply", "--stack=platform", "ingress"}, plan.Commands[lifecycle.NodeID("helmfile", "platform", "ingress")])

	// Abstract and disabled components never run; Packer has no handler and is reported.
	assert.NotContains(t, labels, "terraform/dev/marker-base")
	assert.NotContains(t, labels, "terraform/dev/retired")
	assert.NotContains(t, labels, "packer/platform/image")
	notices := make([]string, 0, len(plan.Notices))
	for _, notice := range plan.Notices {
		notices = append(notices, notice.Kind+"/"+notice.Stack+"/"+notice.Component)
	}
	assert.Contains(t, notices, "packer/platform/image")
}

func TestLifecycleFixtureDestroyProtectsSurvivingConsumers(t *testing.T) {
	_, err := lifecycleFixturePlan(t, "destroy", lifecycle.Selection{Stack: "dev", Reverse: true})
	require.ErrorIs(t, err, errUtils.ErrSurvivingConsumers)
	assert.Contains(t, err.Error(), "terraform/prod/app -> terraform/dev/network")
}

func TestLifecycleFixtureDestroyWithDependentsReversesOrder(t *testing.T) {
	plan, err := lifecycleFixturePlan(t, "destroy", lifecycle.Selection{Stack: "dev", Dependents: -1, Reverse: true})
	require.NoError(t, err)
	labels := planLabels(t, plan)
	assertBefore(t, labels, "script/dev/smoke", "terraform/dev/app")
	assertBefore(t, labels, "terraform/dev/app", "terraform/dev/database")
	assertBefore(t, labels, "terraform/prod/app", "terraform/dev/network")
	assertBefore(t, labels, "terraform/dev/database", "terraform/dev/network")
	assert.NotContains(t, labels, "terraform/platform/registry")
}

// lifecycleFixtureCommand builds the real deploy or destroy command beneath a root that mirrors the
// CLI's global flags and the fixture's custom script commands, then parses args.
func lifecycleFixtureCommand(t *testing.T, verb string, args ...string) (cmd *cobra.Command, fixture string) {
	t.Helper()
	fixture = useLifecycleFixture(t)
	root, _ := registerFixtureCommands(t)
	root.PersistentFlags().String("base-path", "", "")
	for _, name := range []string{"config", "config-path", "profile"} {
		root.PersistentFlags().StringSlice(name, nil, "")
	}
	cmd = newLifecycleCommand(verb)
	root.AddCommand(cmd)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags(args))
	return cmd, fixture
}

func TestLifecycleDeployDryRunPreviewsTheWholePlan(t *testing.T) {
	cmd, fixture := lifecycleFixtureCommand(t, "deploy", "--all", "--dry-run")
	output := captureLifecycle(t)
	require.NoError(t, cmd.RunE(cmd, nil))
	text := output.String()

	// Every component is previewed, in dependency order, with the exact child command line.
	labels := []string{"terraform/dev/network", "terraform/dev/database", "terraform/dev/app", "script/dev/smoke"}
	last := -1
	for _, label := range labels {
		index := strings.Index(text, "Would deploy "+label+"\n")
		require.GreaterOrEqual(t, index, 0, "missing %s in:\n%s", label, text)
		assert.Greater(t, index, last, "%s is out of dependency order", label)
		last = index
	}
	command := func(args ...string) string {
		return "command: atmos " + shellescape.QuoteCommand(append([]string{"--base-path=" + fixture}, args...))
	}
	assert.Contains(t, text, command("terraform", "deploy", "--stack=dev", "network"))
	assert.Contains(t, text, command("helmfile", "apply", "--stack=platform", "ingress"))
	assert.Contains(t, text, command("script", "deploy", "--stack=dev", "smoke"))
	assert.Contains(t, text, "waits for: terraform/dev/cache, terraform/dev/database")
	assert.Contains(t, text, "hook mark-before-deploy: event=before.terraform.deploy kind=step")
	assert.Contains(t, text, "packer/platform/image: component kind does not define this operation")
	assert.NotContains(t, text, "terraform/dev/retired", "disabled components are not planned")
	assert.NotContains(t, text, "marker-base", "abstract components are not planned")
}

func TestLifecycleDestroyDryRunReversesTheOrder(t *testing.T) {
	cmd, _ := lifecycleFixtureCommand(t, "destroy", "--all", "--dry-run")
	output := captureLifecycle(t)
	require.NoError(t, cmd.RunE(cmd, nil))
	text := output.String()

	app := strings.Index(text, "Would destroy terraform/dev/app\n")
	network := strings.Index(text, "Would destroy terraform/dev/network\n")
	smoke := strings.Index(text, "Would destroy script/dev/smoke\n")
	require.GreaterOrEqual(t, smoke, 0)
	require.GreaterOrEqual(t, app, 0)
	require.GreaterOrEqual(t, network, 0)
	assert.Less(t, smoke, app, "consumers are destroyed before their prerequisites")
	assert.Less(t, app, network)
	assert.NotContains(t, text, "event=before.terraform.deploy", "only hooks for the destroy events are previewed")
	assert.NotContains(t, text, "event=after.terraform.deploy")
}

func TestLifecycleDryRunHonorsStackSelectionAndSkipHooks(t *testing.T) {
	cmd, _ := lifecycleFixtureCommand(t, "deploy", "--all", "--stack=platform", "--dry-run", "--skip-hooks=mark-before-deploy")
	output := captureLifecycle(t)
	require.NoError(t, cmd.RunE(cmd, nil))
	text := output.String()
	assert.Contains(t, text, "Would deploy terraform/platform/registry")
	assert.Contains(t, text, "Would deploy helmfile/platform/ingress")
	assert.NotContains(t, text, "terraform/dev/", "other stacks are not selected")
}

func TestLifecycleDryRunSelectsByTagsAndLabels(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		env      map[string]string
		want     []string
		excluded []string
	}{
		{name: "tags alone", args: []string{"--tags=data"}, want: []string{"terraform/dev/database", "terraform/dev/cache"}, excluded: []string{"terraform/dev/network", "terraform/dev/app", "terraform/prod/app"}},
		{name: "labels must all match", args: []string{"--labels=team=platform,tier=1"}, want: []string{"terraform/dev/database"}, excluded: []string{"terraform/dev/cache"}},
		{name: "tags narrow --all", args: []string{"--all", "--tags=stateful"}, want: []string{"terraform/dev/database"}, excluded: []string{"terraform/dev/cache"}},
		{name: "expansion adds untagged prerequisites", args: []string{"--tags=stateful", "--include-dependencies"}, want: []string{"terraform/dev/network", "terraform/dev/database"}, excluded: []string{"terraform/dev/cache"}},
		{name: "ATMOS_TAGS selects too", env: map[string]string{"ATMOS_TAGS": "data"}, want: []string{"terraform/dev/database", "terraform/dev/cache"}, excluded: []string{"terraform/dev/app"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			cmd, _ := lifecycleFixtureCommand(t, "deploy", append(tc.args, "--dry-run")...)
			output := captureLifecycle(t)
			require.NoError(t, cmd.RunE(cmd, nil))
			text := output.String()
			for _, label := range tc.want {
				assert.Contains(t, text, "Would deploy "+label)
			}
			for _, label := range tc.excluded {
				assert.NotContains(t, text, "Would deploy "+label)
			}
		})
	}
}

func TestLifecycleSelectorValidation(t *testing.T) {
	_, err := lifecycleTestCommand(t, "deploy", "--all", "--affected")
	require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	assert.Contains(t, err.Error(), "exactly one of --all or --affected")

	_, err = lifecycleTestCommand(t, "deploy")
	require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	assert.Contains(t, err.Error(), "or select components with --tags or --labels")

	_, err = lifecycleTestCommand(t, "deploy", "--labels=missing-separator")
	require.ErrorIs(t, err, errUtils.ErrInvalidFlag)

	opts, err := lifecycleTestCommand(t, "destroy", "--tags=a,b", "--labels=team=platform", "--labels=tier:1")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, opts.tags)
	assert.Equal(t, map[string]string{"team": "platform", "tier": "1"}, opts.labels)
	assert.False(t, opts.all)
}

func TestLifecycleCommandSurfacesSelectionErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		verb   string
		args   []string
		wantIs error
		want   string
		hint   string
	}{
		{name: "no selector", verb: "deploy", args: []string{"--dry-run"}, wantIs: errUtils.ErrInvalidFlag, want: "exactly one of --all or --affected"},
		{name: "unknown stack", verb: "deploy", args: []string{"--all", "--stack=nope", "--dry-run"}, wantIs: errUtils.ErrStackNotFound, want: `"nope"`, hint: "atmos list stacks"},
		{name: "surviving consumers", verb: "destroy", args: []string{"--all", "--stack=dev", "--dry-run"}, wantIs: errUtils.ErrSurvivingConsumers, want: "terraform/prod/app -> terraform/dev/network", hint: "--include-dependents"},
		{name: "repo path with ref", verb: "deploy", args: []string{"--affected", "--repo-path=" + os.TempDir(), "--ref=main", "--dry-run"}, wantIs: errUtils.ErrInvalidFlag, want: "--repo-path cannot be combined"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd, _ := lifecycleFixtureCommand(t, tc.verb, tc.args...)
			output := captureLifecycle(t)
			err := cmd.RunE(cmd, nil)
			require.ErrorIs(t, err, tc.wantIs)
			assert.Contains(t, err.Error(), tc.want)
			assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), tc.hint)
			assert.NotContains(t, output.String(), "Would ", "nothing is previewed when planning fails")
		})
	}
}

// lifecycleChildRun prepares a plan over the platform stack, whose two components run in order, and a
// run that launches this test binary as each child. The returned log records every child command line.
func lifecycleChildRun(t *testing.T, verb string, exitCode int, mutate func(*lifecycleOptions)) (*lifecycleRun, string) {
	t.Helper()
	plan, err := lifecycleFixturePlan(t, verb, lifecycle.Selection{Stack: "platform", Reverse: verb == "destroy"})
	require.NoError(t, err)
	require.Equal(t, 2, plan.Graph.Size())
	log := filepath.Join(t.TempDir(), "children.log")
	t.Setenv(childExitEnv, strconv.Itoa(exitCode))
	t.Setenv(childLogEnv, log)
	opts := &lifecycleOptions{verb: verb, concurrency: 1, failureMode: "fail-fast", logOrder: "stream"}
	if mutate != nil {
		mutate(opts)
	}
	return &lifecycleRun{config: &schema.AtmosConfiguration{}, plan: plan, globalArgs: []string{"--base-path=fixture"}, options: opts}, log
}

func childCommandLines(t *testing.T, log string) []string {
	t.Helper()
	content, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(content)), "\n")
}

func lifecycleTestCommandWithContext() *cobra.Command {
	cmd := &cobra.Command{Use: "deploy"}
	cmd.Flags().String(flagSkipHooks, "", "")
	cmd.SetContext(context.Background())
	return cmd
}

func TestRunLifecyclePlanLaunchesEachChildInOrder(t *testing.T) {
	run, log := lifecycleChildRun(t, "deploy", 0, nil)
	output := captureLifecycle(t)

	require.NoError(t, runLifecyclePlan(lifecycleTestCommandWithContext(), run))
	assert.Equal(t, []string{
		"--base-path=fixture terraform deploy --stack=platform registry",
		"--base-path=fixture helmfile apply --stack=platform ingress",
	}, childCommandLines(t, log), "prerequisites run first and each child gets the global arguments")
	text := output.String()
	assert.Contains(t, text, "packer/platform/image: component kind does not define this operation")
	assert.Contains(t, text, "deploy terraform/platform/registry: succeeded")
	assert.Contains(t, text, "deploy helmfile/platform/ingress: succeeded")
	assert.Contains(t, text, "2 succeeded, 0 failed, 0 skipped, 1 excluded")
}

func TestRunLifecyclePlanReportsFailureAndSkipsDependents(t *testing.T) {
	run, log := lifecycleChildRun(t, "deploy", 3, nil)
	output := captureLifecycle(t)

	err := runLifecyclePlan(lifecycleTestCommandWithContext(), run)
	require.ErrorIs(t, err, errUtils.ErrComponentExecutionFailed)
	assert.Contains(t, err.Error(), "(exit 3)")
	assert.Equal(t, []string{"--base-path=fixture terraform deploy --stack=platform registry"}, childCommandLines(t, log), "the dependent never starts after a failure")
	text := output.String()
	assert.Contains(t, text, "deploy terraform/platform/registry: failed")
	assert.Contains(t, text, "deploy helmfile/platform/ingress: skipped")
	assert.Contains(t, text, "0 succeeded, 1 failed, 1 skipped, 1 excluded")
}

func TestRunLifecyclePlanDoesNothingWithoutMatches(t *testing.T) {
	run, log := lifecycleChildRun(t, "deploy", 0, nil)
	empty, err := lifecycle.BuildPlan(map[string]any{}, run.plan.Handlers, lifecycle.Selection{}, "{{")
	require.NoError(t, err)
	require.Zero(t, empty.Graph.Size())
	run.plan = empty
	output := captureLifecycle(t)

	require.NoError(t, runLifecyclePlan(lifecycleTestCommandWithContext(), run))
	assert.Contains(t, output.String(), "No components matched")
	assert.NoFileExists(t, log, "no child is launched")

	// A dry run of the same empty plan reports the same thing.
	run.options.dryRun = true
	output.Reset()
	require.NoError(t, runLifecyclePlan(lifecycleTestCommandWithContext(), run))
	assert.Contains(t, output.String(), "No components matched")
}

func TestRunLifecyclePlanRejectsUnanswerableConcurrency(t *testing.T) {
	t.Run("interactive identity selection", func(t *testing.T) {
		run, log := lifecycleChildRun(t, "deploy", 0, func(o *lifecycleOptions) {
			o.concurrency, o.identity = 2, cfg.IdentityFlagSelectValue
		})
		captureLifecycle(t)
		err := runLifecyclePlan(lifecycleTestCommandWithContext(), run)
		require.ErrorIs(t, err, errUtils.ErrInvalidConfig)
		assert.Contains(t, err.Error(), "concurrent execution requires a non-interactive identity")
		assert.NoFileExists(t, log)
	})
	t.Run("approval prompts without auto-approve", func(t *testing.T) {
		run, log := lifecycleChildRun(t, "destroy", 0, func(o *lifecycleOptions) { o.concurrency = 2 })
		captureLifecycle(t)
		err := runLifecyclePlan(lifecycleTestCommandWithContext(), run)
		require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
		assert.Contains(t, err.Error(), "cannot answer approval prompts")
		assert.NoFileExists(t, log)
	})
	t.Run("approval is auto-approved", func(t *testing.T) {
		run, log := lifecycleChildRun(t, "destroy", 0, func(o *lifecycleOptions) { o.concurrency, o.autoApprove = 2, true })
		applyLifecycleApproval(run.plan, true)
		captureLifecycle(t)
		require.NoError(t, runLifecyclePlan(lifecycleTestCommandWithContext(), run))
		lines := childCommandLines(t, log)
		require.Len(t, lines, 2)
		assert.Contains(t, lines, "--base-path=fixture terraform destroy --stack=platform registry -auto-approve", "the registered approval arguments reach the child")
		assert.Contains(t, lines, "--base-path=fixture helmfile destroy --stack=platform ingress", "handlers without approval arguments are unchanged")
	})
}
