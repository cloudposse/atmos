package stack

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"al.essio.dev/pkg/shellescape"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/hooks"
	ioLayer "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	// The childExitEnv variable makes the test binary act as a lifecycle child that exits with the given status.
	childExitEnv = "_ATMOS_STACK_TEST_CHILD_EXIT"
	// The childLogEnv variable names a file that receives each child's command line.
	childLogEnv = "_ATMOS_STACK_TEST_CHILD_LOG"
	// The childCaptureEnv variable makes a child write its own CI output and summary.
	childCaptureEnv = "_ATMOS_STACK_TEST_CHILD_CAPTURE"
)

// TestMain lets lifecycle tests use the test binary itself as the child process, which is portable to
// Windows unlike shell helpers. A child never runs tests.
func TestMain(m *testing.M) {
	if code := os.Getenv(childExitEnv); code != "" {
		os.Exit(runLifecycleTestChild(code))
	}
	os.Exit(m.Run())
}

func runLifecycleTestChild(code string) int {
	status, err := strconv.Atoi(code)
	if err != nil {
		return 99
	}
	if path := os.Getenv(childLogEnv); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			_, _ = f.WriteString(strings.Join(os.Args[1:], " ") + "\n")
			_ = f.Close()
		}
	}
	if os.Getenv(childCaptureEnv) != "" {
		if path := os.Getenv("ATMOS_CI_OUTPUT"); path != "" {
			_ = os.WriteFile(path, []byte("child_key=child_value\nsummary=duplicated\n"), 0o600)
		}
		if path := os.Getenv("ATMOS_CI_SUMMARY"); path != "" {
			_ = os.WriteFile(path, []byte("## child summary\n"), 0o600)
		}
	}
	return status
}

func captureLifecycle(t *testing.T) *bytes.Buffer {
	t.Helper()
	out := &bytes.Buffer{}
	ctx, err := ioLayer.NewContext(ioLayer.WithStreams(stackValidateTestStreams{output: out}))
	require.NoError(t, err)
	ui.InitFormatter(ctx)
	t.Cleanup(ui.Reset)
	return out
}

func TestLifecycleOptionValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values map[string]any
		want   string
	}{
		{"selection required", nil, "exactly one"},
		{"both modes", map[string]any{"all": true, "affected": true}, "exactly one"},
		{"allow requires affected", map[string]any{"all": true, "allow-destroy": true}, "requires --affected"},
		{"zero concurrency", map[string]any{"all": true, "max-concurrency": 0}, "--max-concurrency must be at least 1, got 0"},
		{"bad failure mode", map[string]any{"all": true, "failure-mode": "ignore"}, `invalid --failure-mode "ignore"`},
		{"bad log order", map[string]any{"all": true, "log-order": "random"}, `invalid --log-order "random"`},
		{"bad depth", map[string]any{"all": true, "include-dependencies": "zero"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := viper.New()
			v.Set("max-concurrency", 1)
			v.Set("failure-mode", "fail-fast")
			v.Set("log-order", "stream")
			for k, val := range tc.values {
				v.Set(k, val)
			}
			_, err := parseLifecycleOptions("deploy", v)
			require.Error(t, err)
			if tc.want != "" {
				assert.ErrorIs(t, err, errUtils.ErrInvalidFlag)
				assert.Contains(t, err.Error(), tc.want)
			}
		})
	}
	cmd := newLifecycleCommand("deploy")
	require.NoError(t, cmd.ParseFlags([]string{"--affected", "--include-dependencies", "--include-dependents=2", "--dry-run"}))
	assert.Equal(t, "all", cmd.Flag("include-dependencies").Value.String())
	assert.Error(t, cmd.Args(cmd, []string{"component"}))
	assert.Nil(t, newLifecycleCommand("destroy").Flag("allow-destroy"))
}

func TestHookPreviewShowsActionsConditionsAndSkips(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	n := &dependency.Node{Type: "terraform", Stack: "dev", Component: "app", Metadata: map[string]any{"hooks": map[string]any{
		"image":     map[string]any{"kind": "command", "command": "touch", "args": []any{marker}, "events": []any{"before-terraform-apply"}},
		"publish":   map[string]any{"kind": "step", "type": "container", "with": map[string]any{"image": "example/image:latest"}, "when": "always", "events": []any{"after.terraform.deploy"}},
		"unrelated": map[string]any{"kind": "command", "command": "wrong", "events": []any{"before.terraform.plan"}},
	}}}
	previews, err := previewHooks(n, []string{"before.terraform.deploy", "after.terraform.deploy"}, "")
	require.NoError(t, err)
	require.Len(t, previews, 2)
	text := strings.Join(previews, "\n")
	// Previews shell-quote arguments, so Windows paths with backslashes are quoted.
	assert.Contains(t, text, shellescape.QuoteCommand([]string{"touch", marker}))
	assert.Contains(t, text, "when=always")
	assert.NotContains(t, text, "unrelated")
	assert.Contains(t, text, "condition evaluated at execution")
	assert.NoFileExists(t, marker)
	previews, err = previewHooks(n, []string{"before.terraform.deploy", "after.terraform.deploy"}, "image")
	require.NoError(t, err)
	assert.Contains(t, previews[0], "skipped by --skip-hooks")
	previews, err = previewHooks(n, nil, "")
	require.NoError(t, err)
	assert.Empty(t, previews)
}

func TestNestedCustomDiscoveryAndCollisions(t *testing.T) {
	root := &cobra.Command{Use: "atmos"}
	tools := &cobra.Command{Use: "tools"}
	custom := &cobra.Command{Use: "deploy", Annotations: map[string]string{"customCommand": "true"}}
	tools.AddCommand(custom)
	root.AddCommand(tools)
	config := &schema.AtmosConfiguration{Commands: []schema.Command{{Name: "tools", Flags: []schema.CommandFlag{{Name: "environment", Provides: "stack"}}, Commands: []schema.Command{{Name: "deploy", Component: &schema.CommandComponent{Type: "widget"}, Arguments: []schema.CommandArgument{{Name: "target", Provides: "component"}}}}}}}
	commands := discoverLifecycleCommands(root, config, "deploy")
	require.Contains(t, commands, "widget")
	args, err := commands["widget"].Arguments(&dependency.Node{Stack: "dev", Component: "api"})
	require.NoError(t, err)
	assert.Equal(t, []string{"tools", "deploy", "--environment=dev", "api"}, args)
	second := &cobra.Command{Use: "deploy", Annotations: map[string]string{"customCommand": "true"}}
	root.AddCommand(second)
	config.Commands = append(config.Commands, schema.Command{Name: "deploy", Component: &schema.CommandComponent{Type: "widget"}})
	commands = discoverLifecycleCommands(root, config, "deploy")
	require.ErrorContains(t, commands["widget"].Err, "ambiguous")
	// A declaration shadowed by an actual built-in is ignored.
	delete(second.Annotations, "customCommand")
	commands = discoverLifecycleCommands(root, config, "deploy")
	require.NoError(t, commands["widget"].Err)
}

func TestDeletedTargetsFailBeforeAnyOperations(t *testing.T) {
	output := captureLifecycle(t)
	graph := dependency.NewGraph()
	plan := &lifecycle.Plan{Graph: graph}
	comparison := []schema.Affected{{ComponentType: "terraform", Stack: "gone", Component: "vpc", Deleted: true}}
	opts := lifecycleOptions{verb: "deploy", affected: true, allowDestroy: true, dryRun: true}
	err := executeLifecyclePlans(&cobra.Command{}, &lifecycleRun{config: &schema.AtmosConfiguration{}, plan: plan, options: &opts}, comparison)
	require.ErrorContains(t, err, "no component operations were started")
	assert.Contains(t, output.String(), "Blocked destroy terraform/gone/vpc")
	opts.stack = "dev"
	require.NoError(t, executeLifecyclePlans(&cobra.Command{}, &lifecycleRun{config: &schema.AtmosConfiguration{}, plan: plan, options: &opts}, comparison))
}

// lifecycleTestCommand builds an unattached command with the real parser, so tests
// exercise the same flag registration and option resolution as RunE.
func lifecycleTestCommand(t *testing.T, verb string, args ...string) (lifecycleOptions, error) {
	t.Helper()
	parser := buildLifecycleParser(verb)
	cmd := &cobra.Command{Use: verb}
	parser.RegisterFlags(cmd)
	require.NoError(t, cmd.ParseFlags(args))
	return resolveLifecycleOptions(cmd, parser, verb)
}

func TestLifecycleOptionsIgnoreForeignEnvironment(t *testing.T) {
	// Other commands bind these keys on the global viper; none may change selection.
	t.Setenv("ATMOS_TERRAFORM_DESTROY_ALL", "true")
	t.Setenv("ATMOS_ALL", "true")
	t.Setenv("ATMOS_INCLUDE_DEPENDENCIES", "true")
	t.Setenv("ATMOS_INCLUDE_DEPENDENTS", "true")
	t.Setenv("ATMOS_TERRAFORM_DEPLOY_MAX_CONCURRENCY", "0")
	t.Setenv("ATMOS_AFFECTED", "true")
	t.Run("selection still required", func(t *testing.T) {
		_, err := lifecycleTestCommand(t, "deploy", "--stack=prod", "--dry-run")
		require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
		assert.Contains(t, err.Error(), "exactly one of --all or --affected")
	})
	t.Run("unrelated variables leave options at defaults", func(t *testing.T) {
		opts, err := lifecycleTestCommand(t, "deploy", "--all", "--stack=prod")
		require.NoError(t, err)
		assert.True(t, opts.all)
		assert.False(t, opts.affected)
		assert.Zero(t, opts.dependencies)
		assert.Zero(t, opts.dependents)
		assert.Equal(t, 1, opts.concurrency)
	})
}

func TestLifecycleOptionsHonorOwnEnvironment(t *testing.T) {
	t.Setenv("ATMOS_STACK", "prod")
	t.Setenv("ATMOS_IDENTITY", "off")
	t.Setenv("ATMOS_DRY_RUN", "true")
	opts, err := lifecycleTestCommand(t, "destroy", "--all")
	require.NoError(t, err)
	assert.Equal(t, "prod", opts.stack)
	assert.Equal(t, cfg.IdentityFlagDisabledValue, opts.identity)
	assert.True(t, opts.dryRun)
	// An explicit flag still wins over the environment.
	opts, err = lifecycleTestCommand(t, "destroy", "--all", "--dry-run=false", "--stack=dev")
	require.NoError(t, err)
	assert.False(t, opts.dryRun)
	assert.Equal(t, "dev", opts.stack)
}

func TestLifecycleIdentityNormalization(t *testing.T) {
	for _, value := range []string{"false", "False", "off", "NO", "0"} {
		t.Run(value, func(t *testing.T) {
			opts, err := lifecycleTestCommand(t, "deploy", "--all", "--identity="+value)
			require.NoError(t, err)
			assert.Equal(t, cfg.IdentityFlagDisabledValue, opts.identity)
			args := lifecycleGlobalArgs(&cobra.Command{}, &schema.AtmosConfiguration{}, &opts)
			assert.Contains(t, args, "--identity="+cfg.IdentityFlagDisabledValue)
		})
	}
	opts, err := lifecycleTestCommand(t, "deploy", "--all", "--identity=dev-admin")
	require.NoError(t, err)
	assert.Equal(t, "dev-admin", opts.identity)
}

func TestPreviewArgumentsRendersBareIdentity(t *testing.T) {
	input := []string{"--base-path=/x", "--identity=" + cfg.IdentityFlagSelectValue, "terraform", "deploy"}
	got := previewArguments(input)
	assert.Equal(t, []string{"--base-path=/x", "--identity", "terraform", "deploy"}, got)
	// The real arguments keep the sentinel.
	assert.Equal(t, "--identity="+cfg.IdentityFlagSelectValue, input[1])
	assert.Equal(t, []string{"--identity=dev"}, previewArguments([]string{"--identity=dev"}))
}

func TestAffectedOnlyFlagsRequireAffected(t *testing.T) {
	for _, name := range affectedOnlyFlags {
		t.Run(name, func(t *testing.T) {
			arg := "--" + name + "=value"
			if name == "clone-target-ref" {
				arg = "--clone-target-ref"
			}
			_, err := lifecycleTestCommand(t, "deploy", "--all", arg)
			require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
			assert.Contains(t, err.Error(), "--"+name+" requires --affected")
			_, err = lifecycleTestCommand(t, "deploy", "--affected", arg)
			assert.NotContains(t, errString(err), "requires --affected")
		})
	}
	_, err := lifecycleTestCommand(t, "deploy", "--all")
	require.NoError(t, err)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestLifecycleRejectsPositionalArguments(t *testing.T) {
	cmd := newLifecycleCommand("deploy")
	require.NoError(t, cmd.Args(cmd, nil))
	err := cmd.Args(cmd, []string{"network"})
	require.ErrorIs(t, err, errUtils.ErrInvalidPositionalArgs)
	assert.Contains(t, err.Error(), `"network"`)
	assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "--all, --affected, --tags, or --labels")
}

func TestLifecycleShortDescriptions(t *testing.T) {
	assert.Equal(t, "Destroy stack components in reverse dependency order", newLifecycleCommand("destroy").Short)
	assert.Equal(t, "Deploy stack components in dependency order", newLifecycleCommand("deploy").Short)
	assert.Contains(t, newLifecycleCommand("deploy").Flag("include-dependencies").Usage, "--include-dependencies=N")
	assert.Contains(t, newLifecycleCommand("deploy").Flag("log-order").Usage, "--max-concurrency > 1")
}

func TestCheckLifecycleStackExists(t *testing.T) {
	stacks := map[string]any{"dev": map[string]any{}, "prod": map[string]any{}}
	affected := []schema.Affected{{Stack: "gone"}}
	require.NoError(t, checkLifecycleStackExists(stacks, nil, ""))
	require.NoError(t, checkLifecycleStackExists(stacks, nil, "dev"))
	require.NoError(t, checkLifecycleStackExists(stacks, affected, "gone"))
	err := checkLifecycleStackExists(stacks, affected, "dve")
	require.ErrorIs(t, err, errUtils.ErrStackNotFound)
	assert.Contains(t, err.Error(), `"dve"`)
	assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "atmos list stacks")
	// Negative path: an empty inventory also rejects a named stack.
	require.ErrorIs(t, checkLifecycleStackExists(nil, nil, "dev"), errUtils.ErrStackNotFound)
}

func approvalPlan(t *testing.T) *lifecycle.Plan {
	t.Helper()
	graph := dependency.NewGraph()
	for _, n := range []*dependency.Node{
		{ID: "tf-b", Type: "terraform", Stack: "dev", Component: "b"},
		{ID: "tf-a", Type: "terraform", Stack: "dev", Component: "a"},
		{ID: "cn-c", Type: "container", Stack: "dev", Component: "c"},
	} {
		require.NoError(t, graph.AddNode(n))
	}
	return &lifecycle.Plan{Graph: graph, Handlers: map[string]*lifecycle.Handler{
		"terraform": {ApprovalArgs: []string{"-auto-approve"}},
		"container": {},
	}}
}

func TestCheckLifecycleApproval(t *testing.T) {
	plan := approvalPlan(t)
	err := checkLifecycleApproval(plan, &lifecycleOptions{concurrency: 2})
	require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	assert.Contains(t, err.Error(), "terraform/dev/a, terraform/dev/b")
	assert.NotContains(t, err.Error(), "container/dev/c")
	hint := strings.Join(errUtils.AllHints(err), "\n")
	assert.Contains(t, hint, "--auto-approve")
	assert.Contains(t, hint, "--max-concurrency=1")
	// Negative paths: the preflight must not trigger when approval cannot prompt or is given.
	require.NoError(t, checkLifecycleApproval(plan, &lifecycleOptions{concurrency: 2, autoApprove: true}))
	require.NoError(t, checkLifecycleApproval(plan, &lifecycleOptions{concurrency: 2, dryRun: true}))
	require.NoError(t, checkLifecycleApproval(plan, &lifecycleOptions{concurrency: 1}))
	plan.Handlers["terraform"].ApprovalArgs = nil
	require.NoError(t, checkLifecycleApproval(plan, &lifecycleOptions{concurrency: 2}))
}

func TestReportLifecycleResultsTally(t *testing.T) {
	output := captureLifecycle(t)
	node := func(name string) dependency.Node {
		return dependency.Node{Type: "terraform", Stack: "dev", Component: name}
	}
	results := []scheduler.Result{
		{Node: node("a"), Status: scheduler.StatusSucceeded},
		{Node: node("b"), Status: scheduler.StatusSucceeded},
		{Node: node("c"), Status: scheduler.StatusFailed},
		{Node: node("d"), Status: scheduler.StatusSkipped},
	}
	assert.Equal(t, "2 succeeded, 1 failed, 1 skipped, 3 excluded", lifecycleTally(results, 3))
	reportLifecycleResults("destroy", results, 3)
	text := output.String()
	assert.Contains(t, text, "destroy terraform/dev/c: failed")
	assert.Contains(t, text, "destroy terraform/dev/d: skipped")
	assert.Contains(t, text, "2 succeeded, 1 failed, 1 skipped, 3 excluded")
	assert.Equal(t, 1, strings.Count(text, "succeeded,"))
	assert.Equal(t, "0 succeeded, 0 failed, 0 skipped, 0 excluded", lifecycleTally(nil, 0))
}

func TestLifecycleGlobalArgsForwardExplicitlySetFlags(t *testing.T) {
	newCommand := func() *cobra.Command {
		cmd := &cobra.Command{Use: "deploy"}
		for _, name := range []string{"config", "config-path", "profile"} {
			cmd.Flags().StringSlice(name, nil, "")
		}
		for _, name := range []string{"edition", "settings-list-merge-strategy", "logs-level"} {
			cmd.Flags().String(name, "", "")
		}
		cmd.Flags().Bool("no-color", false, "")
		cmd.Flags().Bool("mask", true, "")
		return cmd
	}
	config := &schema.AtmosConfiguration{BasePathAbsolute: filepath.Join(t.TempDir(), "project")}
	base := "--base-path=" + config.BasePathAbsolute

	// Negative path: flags the user did not set are never forwarded.
	assert.Equal(t, []string{base}, lifecycleGlobalArgs(newCommand(), config, &lifecycleOptions{}))

	cmd := newCommand()
	for name, value := range map[string]string{
		"config": "custom.yaml", "config-path": "configs", "profile": "dev,ci",
		"edition": "2026", "settings-list-merge-strategy": "append", "logs-level": "Debug", "no-color": "true", "mask": "false",
	} {
		require.NoError(t, cmd.Flags().Set(name, value))
	}
	absolute := func(path string) string {
		abs, err := filepath.Abs(path)
		require.NoError(t, err)
		return abs
	}
	got := lifecycleGlobalArgs(cmd, config, &lifecycleOptions{identity: "dev-admin"})
	assert.Equal(t, []string{
		base, "--identity=dev-admin",
		"--config=" + absolute("custom.yaml"), "--config-path=" + absolute("configs"),
		// Profiles are names, not paths, and are never made absolute.
		"--profile=dev", "--profile=ci",
		"--edition=2026", "--settings-list-merge-strategy=append", "--logs-level=Debug", "--no-color=true", "--mask=false",
	}, got)
}

func TestLifecycleEnvironmentPropagatesSkipHooksExactlyOnce(t *testing.T) {
	clearCIEnvironment(t)
	t.Setenv("ATMOS_SKIP_HOOKS", "inherited")
	count := func(env []string) (n int) {
		for _, entry := range env {
			if strings.HasPrefix(entry, "ATMOS_SKIP_HOOKS=") {
				n++
			}
		}
		return n
	}
	newCommand := func(args ...string) *cobra.Command {
		cmd := &cobra.Command{Use: "deploy"}
		cmd.Flags().String(flagSkipHooks, "", "")
		cmd.Flags().Lookup(flagSkipHooks).NoOptDefVal = "*"
		require.NoError(t, cmd.ParseFlags(args))
		return cmd
	}
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"named hooks replace the inherited value": {[]string{"--skip-hooks=lint,notify"}, "lint,notify"},
		"bare flag skips everything":              {[]string{"--skip-hooks"}, "*"},
		"explicit empty value clears the skip":    {[]string{"--skip-hooks="}, ""},
		"unset flag leaves the inherited value":   {nil, "inherited"},
	} {
		t.Run(name, func(t *testing.T) {
			env := lifecycleEnvironment(newCommand(tc.args...), false)
			assert.Equal(t, 1, count(env), "children never see a duplicated key")
			assert.Equal(t, tc.want, envValue(env, "ATMOS_SKIP_HOOKS"))
		})
	}
}

func TestPreviewHooksHandlesAbsentInvalidAndOpaqueDeclarations(t *testing.T) {
	events := []string{"after.terraform.deploy"}

	t.Run("no hooks section", func(t *testing.T) {
		previews, err := previewHooks(&dependency.Node{Type: "terraform", Stack: "dev", Component: "app", Metadata: map[string]any{}}, events, "")
		require.NoError(t, err)
		assert.Empty(t, previews)
	})
	t.Run("hooks that are not a mapping", func(t *testing.T) {
		node := &dependency.Node{Type: "terraform", Stack: "dev", Component: "app", Metadata: map[string]any{"hooks": []any{"not-a-hook"}}}
		_, err := previewHooks(node, events, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "terraform/dev/app hooks:")
	})
	t.Run("step payload that JSON cannot encode is rendered as YAML", func(t *testing.T) {
		// YAML allows .nan, which JSON cannot represent.
		action := hookPreviewAction(&hooks.Hook{Kind: "step", Type: "toast", With: math.NaN()})
		assert.Equal(t, " type=toast .nan", action)
	})
	t.Run("store hooks show their declaration", func(t *testing.T) {
		node := &dependency.Node{Type: "terraform", Stack: "dev", Component: "app", Metadata: map[string]any{"hooks": map[string]any{
			"publish": map[string]any{"kind": "store", "name": "ssm", "events": events, "outputs": map[string]any{"vpc_id": ".id"}},
		}}}
		previews, err := previewHooks(node, events, "")
		require.NoError(t, err)
		require.Len(t, previews, 1)
		assert.Contains(t, previews[0], "hook publish: event=after.terraform.deploy kind=store")
		assert.Contains(t, previews[0], "name: ssm")
		assert.Contains(t, previews[0], "vpc_id: .id")
		assert.NotContains(t, previews[0], "events:", "the matched events are reported separately")
	})
}

func cyclicPlan(t *testing.T) *lifecycle.Plan {
	t.Helper()
	graph := dependency.NewGraph()
	for _, id := range []string{"a", "b"} {
		require.NoError(t, graph.AddNode(&dependency.Node{ID: id, Type: "terraform", Stack: "dev", Component: id}))
	}
	require.NoError(t, graph.AddDependency("a", "b"))
	require.NoError(t, graph.AddDependency("b", "a"))
	return &lifecycle.Plan{Graph: graph, Commands: map[string][]string{}, Handlers: map[string]*lifecycle.Handler{}}
}

func TestPreviewLifecycleReportsUnorderableAndInvalidHookPlans(t *testing.T) {
	captureLifecycle(t)
	require.ErrorIs(t, previewLifecycle(cyclicPlan(t), "deploy", nil, &cobra.Command{}), dependency.ErrCircularDependency)

	graph := dependency.NewGraph()
	require.NoError(t, graph.AddNode(&dependency.Node{ID: "a", Type: "terraform", Stack: "dev", Component: "a", Metadata: map[string]any{"hooks": "invalid"}}))
	plan := &lifecycle.Plan{Graph: graph, Commands: map[string][]string{"a": {"terraform", "deploy"}}, Handlers: map[string]*lifecycle.Handler{"terraform": {HookEvents: []string{"after.terraform.deploy"}}}}
	err := previewLifecycle(plan, "deploy", nil, &cobra.Command{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "terraform/dev/a hooks:")
}

func TestBlockedDeletedTargetsStillSurfacePreviewFailures(t *testing.T) {
	output := captureLifecycle(t)
	comparison := []schema.Affected{{ComponentType: "terraform", Stack: "gone", Component: "vpc", Deleted: true}}
	opts := lifecycleOptions{verb: "destroy", affected: true, allowDestroy: true, dryRun: true}

	err := executeLifecyclePlans(&cobra.Command{}, &lifecycleRun{config: &schema.AtmosConfiguration{}, plan: cyclicPlan(t), options: &opts}, comparison)
	require.ErrorIs(t, err, dependency.ErrCircularDependency)
	assert.Contains(t, output.String(), "Blocked destroy terraform/gone/vpc")

	// Negative path: without --allow-destroy deleted targets are only reported by the plan, never blocked here.
	opts.allowDestroy = false
	output.Reset()
	err = executeLifecyclePlans(&cobra.Command{}, &lifecycleRun{config: &schema.AtmosConfiguration{}, plan: cyclicPlan(t), options: &opts}, comparison)
	require.ErrorIs(t, err, dependency.ErrCircularDependency)
	assert.NotContains(t, output.String(), "Blocked destroy")
}

func TestResolveLifecycleSelection(t *testing.T) {
	stacks := map[string]any{"dev": map[string]any{}}
	opts := &lifecycleOptions{verb: "destroy", stack: "dev", dependencies: 2, dependents: -1}

	selection, affected, err := resolveLifecycleSelection(&cobra.Command{}, &schema.AtmosConfiguration{}, opts, stacks)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.Selection{Stack: "dev", Dependencies: 2, Dependents: -1, Reverse: true}, selection)
	assert.Nil(t, affected)

	opts.verb = "deploy"
	selection, _, err = resolveLifecycleSelection(&cobra.Command{}, &schema.AtmosConfiguration{}, opts, stacks)
	require.NoError(t, err)
	assert.False(t, selection.Reverse, "only destroy reverses the order")

	opts.stack = "missing"
	_, _, err = resolveLifecycleSelection(&cobra.Command{}, &schema.AtmosConfiguration{}, opts, stacks)
	require.ErrorIs(t, err, errUtils.ErrStackNotFound)
}

func TestCompareLifecycleAffectedRejectsRepoPathWithComparisonOptions(t *testing.T) {
	for _, conflicting := range []string{"--ref=main", "--sha=abc123", "--base=main", "--ssh-key=/tmp/key", "--ssh-key-password=secret", "--clone-target-ref"} {
		t.Run(conflicting, func(t *testing.T) {
			cmd := newLifecycleCommand("deploy")
			require.NoError(t, cmd.ParseFlags([]string{"--affected", "--repo-path=" + t.TempDir(), conflicting}))
			opts := &lifecycleOptions{verb: "deploy", affected: true}
			_, err := compareLifecycleAffected(cmd, &schema.AtmosConfiguration{}, opts)
			require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
			assert.Contains(t, err.Error(), "--repo-path cannot be combined with ref, SHA, or clone options")

			// The same failure is reported by selection, before any plan is built.
			_, _, err = resolveLifecycleSelection(cmd, &schema.AtmosConfiguration{}, opts, nil)
			require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
		})
	}
}

func TestApplyLifecycleApprovalAppendsHandlerArguments(t *testing.T) {
	plan := approvalPlan(t)
	plan.Commands = map[string][]string{"tf-a": {"terraform", "destroy"}, "tf-b": {"terraform", "destroy"}, "cn-c": {"container", "destroy"}}

	applyLifecycleApproval(plan, false)
	assert.Equal(t, []string{"terraform", "destroy"}, plan.Commands["tf-a"], "nothing is added without --auto-approve")

	applyLifecycleApproval(plan, true)
	assert.Equal(t, []string{"terraform", "destroy", "-auto-approve"}, plan.Commands["tf-a"])
	assert.Equal(t, []string{"terraform", "destroy", "-auto-approve"}, plan.Commands["tf-b"])
	assert.Equal(t, []string{"container", "destroy"}, plan.Commands["cn-c"], "handlers without approval arguments are unchanged")
}

func TestDiscoveryRetainsCustomCommandLoadErrors(t *testing.T) {
	root := &cobra.Command{Use: "atmos"}
	custom := &cobra.Command{Use: "deploy", Annotations: map[string]string{"customCommand": "true"}}
	root.AddCommand(custom)
	broken := errors.New("step could not be decoded")
	config := &schema.AtmosConfiguration{Commands: []schema.Command{{Name: "deploy", Component: &schema.CommandComponent{Type: "widget"}, Steps: schema.Tasks{{Name: "run", LoadError: broken}}}}}

	commands := discoverLifecycleCommands(root, config, "deploy")
	require.Contains(t, commands, "widget")
	require.ErrorIs(t, commands["widget"].Err, broken)
	// The handler stays registered so only plans that select the kind fail.
	_, err := commands["widget"].Arguments(&dependency.Node{Stack: "dev", Component: "api"})
	require.ErrorIs(t, err, broken)

	config.Commands[0].Steps = nil
	assert.NoError(t, discoverLifecycleCommands(root, config, "deploy")["widget"].Err)
}
