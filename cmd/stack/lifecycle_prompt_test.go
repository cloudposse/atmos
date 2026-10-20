package stack

import (
	"context"
	"io"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/charmbracelet/huh"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

func lifecycleInteractiveTest(t *testing.T, answers ...string) *int {
	t.Helper()
	t.Setenv("ATMOS_FORCE_TTY", "true")
	t.Setenv("ATMOS_CI", "false")
	preserved := telemetry.PreserveCIEnvVars()
	t.Cleanup(func() { telemetry.RestoreCIEnvVars(preserved) })
	previous := viper.Get("interactive")
	viper.Set("interactive", true)
	t.Cleanup(func() { viper.Set("interactive", previous) })
	calls := 0
	restore := flags.SetFormRunnerForTest(func(form *huh.Form) error {
		require.Less(t, calls, len(answers), "unexpected prompt")
		input := answers[calls]
		calls++
		return form.WithAccessible(true).
			WithInput(iotest.OneByteReader(strings.NewReader(input))).
			WithOutput(io.Discard).Run()
	})
	t.Cleanup(restore)
	return &calls
}

func TestLifecycleInteractiveSelection(t *testing.T) {
	tests := []struct {
		name    string
		flags   []string
		args    []string
		answers []string
		want    string
	}{
		{"stack then component", nil, nil, []string{"1\n", "1\n"}, "terraform/dev/app"},
		{"supplied stack", []string{"--stack=dev"}, nil, []string{"1\n"}, "terraform/dev/app"},
		{"named component filters stacks", nil, []string{"ingress"}, []string{"1\n"}, "helmfile/platform/ingress"},
		{"explicit component and stack", []string{"--stack=platform"}, []string{"ingress"}, nil, "helmfile/platform/ingress"},
		{"all bypasses prompts", []string{"--all", "--stack=platform"}, nil, nil, "terraform/platform/registry"},
		{"tags only prompt for stack", []string{"--tags=data"}, nil, []string{"1\n"}, "terraform/dev/database"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _ := lifecycleFixtureCommand(t, "deploy", append(tt.flags, "--dry-run")...)
			output := captureLifecycle(t)
			calls := lifecycleInteractiveTest(t, tt.answers...)
			require.NoError(t, cmd.RunE(cmd, tt.args))
			require.Equal(t, len(tt.answers), *calls)
			require.Contains(t, output.String(), "Would deploy "+tt.want)
			if len(tt.flags) == 0 && len(tt.args) == 0 {
				require.NotContains(t, output.String(), "Would deploy terraform/dev/network")
			}
		})
	}
}

func TestLifecyclePromptCancellation(t *testing.T) {
	for _, cancelAt := range []int{1, 2} {
		t.Run(strconv.Itoa(cancelAt), func(t *testing.T) {
			cmd, _ := lifecycleFixtureCommand(t, "deploy")
			output := captureLifecycle(t)
			lifecycleInteractiveTest(t)
			calls := 0
			restore := flags.SetFormRunnerForTest(func(form *huh.Form) error {
				calls++
				if calls == cancelAt {
					return huh.ErrUserAborted
				}
				return form.WithAccessible(true).WithInput(strings.NewReader("1\n")).WithOutput(io.Discard).Run()
			})
			t.Cleanup(restore)
			require.ErrorIs(t, cmd.RunE(cmd, nil), errUtils.ErrUserAborted)
			require.Equal(t, cancelAt, calls)
			require.NotContains(t, output.String(), "Would deploy")
			require.NotContains(t, output.String(), "succeeded")
		})
	}
}

func TestLifecycleNoninteractiveRequiresExplicitSelection(t *testing.T) {
	lifecycleInteractiveTest(t)
	viper.Set("interactive", false)
	tests := []struct {
		flags []string
		args  []string
		want  error
	}{
		{nil, nil, errUtils.ErrInvalidFlag},
		{nil, []string{"app"}, errUtils.ErrInvalidFlag},
		{[]string{"--stack=dev"}, nil, errUtils.ErrInvalidPositionalArgs},
		{[]string{"--tags=data"}, nil, errUtils.ErrInvalidFlag},
	}
	for _, tt := range tests {
		cmd, _ := lifecycleFixtureCommand(t, "deploy", tt.flags...)
		require.ErrorIs(t, cmd.RunE(cmd, tt.args), tt.want)
	}
}

func TestLifecycleNamedComponentValidation(t *testing.T) {
	for _, verb := range []string{"deploy", "destroy"} {
		t.Run(verb, func(t *testing.T) {
			cmd, _ := lifecycleFixtureCommand(t, verb, "--stack=dev", "--dry-run")
			output := captureLifecycle(t)
			require.ErrorIs(t, cmd.RunE(cmd, []string{"typo"}), errUtils.ErrInvalidComponent)
			require.NotContains(t, output.String(), "Would ")
		})
	}
	opts := &lifecycleOptions{component: "gone", stack: "dev", affected: true}
	require.NoError(t, checkLifecycleComponentExists(nil, []schema.Affected{{Stack: "dev", Component: "gone", Deleted: true}}, opts))
	require.ErrorIs(t, checkLifecycleComponentExists(nil, []schema.Affected{{Stack: "prod", Component: "gone", Deleted: true}}, opts), errUtils.ErrInvalidComponent)
}

func TestLifecycleTargetPromptContextAndCI(t *testing.T) {
	lifecycleInteractiveTest(t)
	cmd := newLifecycleCommand("deploy")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd.SetContext(ctx)
	_, err := promptLifecycleTarget(cmd, "stack", "Choose a stack", []string{"dev"})
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, checkLifecyclePromptAvailability(&lifecycleOptions{ci: true}), errUtils.ErrInvalidFlag)
	require.NoError(t, resolveLifecycleTargets(cmd, &lifecycleOptions{affected: true}, nil, nil))
	require.NoError(t, resolveLifecycleTargets(cmd, &lifecycleOptions{all: true}, nil, nil))
}

func TestLifecyclePickerExcludesUnavailableComponents(t *testing.T) {
	stacks := map[string]any{"dev": map[string]any{"components": map[string]any{
		"terraform": map[string]any{
			"app":  map[string]any{},
			"base": map[string]any{"metadata": map[string]any{"type": "abstract"}},
			"off":  map[string]any{"metadata": map[string]any{"enabled": false}},
		},
		"helm":   map[string]any{"app": map[string]any{}},
		"packer": map[string]any{"image": map[string]any{}},
	}}}
	handlers := map[string]*lifecycle.Handler{"terraform": {}, "helm": {}}
	require.Equal(t, []string{"app"}, lifecycleComponentNames(stacks, "dev", handlers))
}

func TestLifecycleEmptyPickerOptions(t *testing.T) {
	lifecycleInteractiveTest(t)
	cmd := newLifecycleCommand("deploy")
	cmd.SetContext(context.Background())
	require.ErrorIs(t, resolveLifecycleTargets(cmd, &lifecycleOptions{}, nil, nil), errUtils.ErrNoStacksToSelect)
	require.ErrorIs(t, resolveLifecycleTargets(cmd, &lifecycleOptions{stack: "dev"},
		map[string]any{"dev": map[string]any{}}, nil), errUtils.ErrNoComponentsToSelect)
}

func TestNamedSelectionIgnoresOtherDeletedComponents(t *testing.T) {
	captureLifecycle(t)
	opts := &lifecycleOptions{verb: "deploy", component: "app", stack: "dev", affected: true, allowDestroy: true, dryRun: true}
	run := &lifecycleRun{config: &schema.AtmosConfiguration{}, plan: &lifecycle.Plan{Graph: dependency.NewGraph()}, options: opts}
	deleted := []schema.Affected{{ComponentType: "terraform", Stack: "dev", Component: "network", Deleted: true}}
	require.NoError(t, executeLifecyclePlans(newLifecycleCommand("deploy"), run, deleted))
	deleted[0].Component = "app"
	require.ErrorIs(t, executeLifecyclePlans(newLifecycleCommand("deploy"), run, deleted), errUtils.ErrInvalidConfig)
}
