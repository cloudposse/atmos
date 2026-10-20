package stack

import (
	"context"
	"errors"
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
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
)

// lifecyclePackageDir is the package directory, captured before any test changes the working directory.
var lifecyclePackageDir, _ = os.Getwd()

func TestLifecycleAuthPreflight(t *testing.T) {
	configured := schema.AuthConfig{Identities: map[string]schema.Identity{
		"global": {Kind: "aws/permission-set", Default: true},
	}, IdentityCaseMap: map[string]string{"global": "global"}}
	marker := map[string]any{"auth": map[string]any{"identities": map[string]any{
		"missing": map[string]any{"default": true},
	}}}
	local := map[string]any{"auth": map[string]any{"identities": map[string]any{
		"local": map[string]any{"kind": "aws/permission-set", "default": true},
	}}}
	for _, tt := range []struct {
		name      string
		identity  string
		component map[string]any
		dryRun    bool
		want      error
	}{
		{"missing explicit", "missing", nil, false, errUtils.ErrIdentityNotFound},
		{"undefined component default", "", marker, false, errUtils.ErrInvalidIdentityConfig},
		{"explicit retains native merge validation", "global", marker, false, errUtils.ErrInvalidIdentityConfig},
		{"component identity", "local", local, false, nil},
		{"component default", "", local, false, nil},
		{"explicit overrides valid default", "global", local, false, nil},
		{"case insensitive explicit", "GLOBAL", nil, false, nil},
		{"interactive selection remains deferred", cfg.IdentityFlagSelectValue, nil, false, nil},
		{"disabled auth", cfg.IdentityFlagDisabledValue, marker, false, nil},
		{"dry run", "missing", marker, true, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			node := &dependency.Node{ID: "app", Type: "terraform", Stack: "dev", Component: "app", Metadata: tt.component}
			run := &lifecycleRun{
				config: &schema.AtmosConfiguration{Auth: configured},
				plan: &lifecycle.Plan{Graph: &dependency.Graph{Nodes: map[string]*dependency.Node{
					"app": node,
				}}},
				options: &lifecycleOptions{identity: tt.identity, dryRun: tt.dryRun},
			}
			err := checkLifecycleAuth(cmd, run)
			if tt.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.want)
				require.Contains(t, strings.Join(errUtils.AllHints(err), " "), "no component operations were started")
			}
			require.True(t, configured.Identities["global"].Default, "preflight must not mutate global auth")
		})
	}
}

func TestLifecycleAuthNoDefaultDoesNotSelectIdentity(t *testing.T) {
	config := &schema.AuthConfig{Identities: map[string]schema.Identity{"unused": {}}}
	require.NoError(t, checkLifecycleIdentity(config, ""))
	require.ErrorIs(t, checkLifecycleIdentity(config, "unused"), errUtils.ErrIdentityNotFound)
}

func TestLifecycleAuthProfileHints(t *testing.T) {
	t.Setenv("ATMOS_PROFILE", "")
	root := t.TempDir()
	profileDir := filepath.Join(root, "profiles", "developer")
	require.NoError(t, os.MkdirAll(profileDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "auth.yaml"), []byte("auth:\n  identities:\n    plat-dev:\n      kind: aws/permission-set\n"), 0o600))
	config := &schema.AtmosConfiguration{CliConfigPath: root, Profiles: schema.ProfilesConfig{BasePath: "profiles"}}
	err := lifecycleAuthConfigError(config, missingLifecycleIdentity("plat-dev"), "terraform/dev/app")
	require.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
	require.Contains(t, strings.Join(errUtils.AllHints(err), " "), "--profile=developer")
	otherProfile := filepath.Join(root, "profiles", "devops")
	require.NoError(t, os.MkdirAll(otherProfile, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(otherProfile, "auth.yaml"), []byte("auth:\n  identities:\n    plat-dev:\n      kind: aws/permission-set\n"), 0o600))
	err = lifecycleAuthConfigError(config, missingLifecycleIdentity("plat-dev"), "terraform/dev/app")
	hints := strings.Join(errUtils.AllHints(err), " ")
	require.Contains(t, hints, "developer, devops")
	require.Equal(t, 1, strings.Count(hints, "--profile="))
	t.Setenv("ATMOS_PROFILE", "production")
	err = lifecycleAuthConfigError(config, missingLifecycleIdentity("plat-dev"), "terraform/dev/app")
	require.NotContains(t, strings.Join(errUtils.AllHints(err), " "), "--profile=developer")
}

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
func lifecycleFixturePlan(t *testing.T, verb string, selection *lifecycle.Selection) (*lifecycle.Plan, error) {
	t.Helper()
	useLifecycleFixture(t)

	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Custom commands are attached by the root command at startup; mirror that here.
	_, script := registerFixtureCommands(t)

	stacks, handlers, err := loadLifecycleInventory(script, &config, verb, nil)
	require.NoError(t, err)
	selection.Operation = lifecycle.Event(verb)
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
	plan, err := lifecycleFixturePlan(t, "deploy", &lifecycle.Selection{})
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
	_, err := lifecycleFixturePlan(t, "destroy", &lifecycle.Selection{Stack: "dev", Reverse: true})
	require.ErrorIs(t, err, errUtils.ErrSurvivingConsumers)
	assert.Contains(t, err.Error(), "terraform/prod/app -> terraform/dev/network")
}

func TestLifecycleFixtureDestroyWithDependentsReversesOrder(t *testing.T) {
	plan, err := lifecycleFixturePlan(t, "destroy", &lifecycle.Selection{Stack: "dev", Dependents: -1, Reverse: true})
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
		{name: "tags alone", args: []string{"--stack=dev", "--tags=data"}, want: []string{"terraform/dev/database", "terraform/dev/cache"}, excluded: []string{"terraform/dev/network", "terraform/dev/app", "terraform/prod/app"}},
		{name: "labels must all match", args: []string{"--stack=dev", "--labels=team=platform,tier=1"}, want: []string{"terraform/dev/database"}, excluded: []string{"terraform/dev/cache"}},
		{name: "tags narrow --all", args: []string{"--all", "--tags=stateful"}, want: []string{"terraform/dev/database"}, excluded: []string{"terraform/dev/cache"}},
		{name: "expansion adds untagged prerequisites", args: []string{"--stack=dev", "--tags=stateful", "--include-dependencies"}, want: []string{"terraform/dev/network", "terraform/dev/database"}, excluded: []string{"terraform/dev/cache"}},
		{name: "ATMOS_TAGS selects too", args: []string{"--stack=dev"}, env: map[string]string{"ATMOS_TAGS": "data"}, want: []string{"terraform/dev/database", "terraform/dev/cache"}, excluded: []string{"terraform/dev/app"}},
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

	opts, err := lifecycleTestCommand(t, "deploy")
	require.NoError(t, err)
	assert.True(t, needsLifecycleStack(&opts))
	assert.False(t, needsLifecycleComponent(&opts))

	_, err = lifecycleTestCommand(t, "deploy", "--labels=missing-separator")
	require.ErrorIs(t, err, errUtils.ErrInvalidFlag)

	opts, err = lifecycleTestCommand(t, "destroy", "--tags=a,b", "--labels=team=platform", "--labels=tier:1")
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
		{name: "no selector", verb: "deploy", args: []string{"--dry-run"}, wantIs: errUtils.ErrInvalidFlag, want: "specify --stack, --all, or --affected"},
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
	plan, err := lifecycleFixturePlan(t, verb, &lifecycle.Selection{Stack: "platform", Reverse: verb == "destroy"})
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
	empty, err := lifecycle.BuildPlan(map[string]any{}, run.plan.Handlers, &lifecycle.Selection{}, "{{")
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

func TestRunLifecyclePlanRejectsMissingIdentityBeforeAnyChild(t *testing.T) {
	run, log := lifecycleChildRun(t, "deploy", 0, nil)
	// The dependency runs first; an invalid identity on the later child must still
	// stop the entire operation before that first dependency is launched.
	for _, node := range run.plan.Graph.Nodes {
		if node.Component == "ingress" {
			node.Metadata["auth"] = map[string]any{"identities": map[string]any{
				"missing": map[string]any{"default": true},
			}}
		}
	}
	output := captureLifecycle(t)
	err := runLifecyclePlan(lifecycleTestCommandWithContext(), run)
	require.ErrorIs(t, err, errUtils.ErrInvalidIdentityConfig)
	require.NoFileExists(t, log)
	require.NotContains(t, output.String(), ": failed")
	require.NotContains(t, output.String(), ": skipped")
	require.NotContains(t, output.String(), "succeeded")
	require.Contains(t, strings.Join(errUtils.AllHints(err), " "), "no component operations were started")
}

func TestReportLifecycleResultsExplainsSchedulerSkips(t *testing.T) {
	for _, tc := range []struct {
		name        string
		failFast    bool
		dependent   string
		independent string
	}{
		{"fail fast", true, "skipped: fail-fast after terraform/dev/a failed", "skipped: fail-fast after terraform/dev/a failed"},
		{"keep going", false, "skipped: dependency terraform/dev/a failed", "succeeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := captureLifecycle(t)
			builder := dependency.NewBuilder()
			for _, name := range []string{"a", "b", "z"} {
				require.NoError(t, builder.AddNode(&dependency.Node{
					ID: lifecycle.NodeID("terraform", "dev", name), Type: "terraform", Stack: "dev", Component: name,
				}))
			}
			require.NoError(t, builder.AddDependency(lifecycle.NodeID("terraform", "dev", "b"), lifecycle.NodeID("terraform", "dev", "a")))
			graph, err := builder.Build()
			require.NoError(t, err)
			dispatcher := scheduler.DispatcherFunc(func(_ context.Context, node *dependency.Node) (scheduler.Result, error) {
				if node.Component == "a" {
					return scheduler.Result{}, errors.New("operation failed")
				}
				return scheduler.Result{}, nil
			})
			result := scheduler.New(graph, dispatcher, scheduler.WithMaxConcurrency(1), scheduler.WithFailFast(tc.failFast)).Run(t.Context())
			require.Error(t, result.Err)
			reportLifecycleResults("plan", result.Results, 0, 0)
			assert.Contains(t, output.String(), "plan terraform/dev/b: "+tc.dependent)
			assert.Contains(t, output.String(), "plan terraform/dev/z: "+tc.independent)
			assert.NotContains(t, output.String(), "scheduler node skipped")
			assert.NotContains(t, output.String(), lifecycle.NodeID("terraform", "dev", "a"))
		})
	}
}

func TestReportLifecycleResultsExplainsCancellation(t *testing.T) {
	for _, reason := range []error{context.Canceled, context.DeadlineExceeded, errors.New("custom skip reason")} {
		t.Run(reason.Error(), func(t *testing.T) {
			output := captureLifecycle(t)
			reportLifecycleResults("plan", []scheduler.Result{{
				Node: dependency.Node{Type: "terraform", Stack: "dev", Component: "app"}, Status: scheduler.StatusSkipped, Err: reason,
			}}, 0, 0)
			assert.Contains(t, output.String(), "plan terraform/dev/app: skipped: "+reason.Error())
			assert.Contains(t, output.String(), "0 succeeded, 0 failed, 1 skipped, 0 excluded")
		})
	}
}
