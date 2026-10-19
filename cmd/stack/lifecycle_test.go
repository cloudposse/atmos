package stack

import (
	"bytes"
	"path/filepath"
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
	ioLayer "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
	"github.com/cloudposse/atmos/pkg/ui"
)

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
	assert.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "--all or --affected")
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
