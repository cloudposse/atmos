package stack

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/dependency"
	ioLayer "github.com/cloudposse/atmos/pkg/io"
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
		{"zero concurrency", map[string]any{"all": true, "max-concurrency": 0}, "invalid concurrency"},
		{"bad failure mode", map[string]any{"all": true, "failure-mode": "ignore"}, "invalid concurrency"},
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
	assert.Contains(t, text, "touch "+marker)
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
