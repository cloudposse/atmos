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
		{"stack then components", []string{"--component"}, nil, []string{"1\n", "1\n0\n"}, "terraform/dev/app"},
		{"supplied stack selects all", []string{"--stack=dev"}, nil, nil, "terraform/dev/network"},
		{"stack picker selects all", nil, nil, []string{"1\n"}, "terraform/dev/network"},
		{"multiselect", []string{"--stack=dev", "--component"}, nil, []string{"1\n2\n0\n"}, "terraform/dev/cache"},
		{"all with picker", []string{"--all", "--component"}, nil, []string{"1\n0\n"}, "terraform/dev/app"},
		{"explicit names preselected", []string{"--stack=dev", "--component=app", "--component"}, nil, []string{"0\n"}, "terraform/dev/app"},
		{"named component filters stacks", []string{"--component=ingress"}, nil, []string{"1\n"}, "helmfile/platform/ingress"},
		{"explicit component and stack", []string{"--stack=platform", "--component=ingress"}, nil, nil, "helmfile/platform/ingress"},
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
			if tt.name == "multiselect" {
				require.Contains(t, output.String(), "Would deploy terraform/dev/app")
				require.NotContains(t, output.String(), "Would deploy terraform/dev/network")
			}
			if tt.name == "stack then components" {
				require.NotContains(t, output.String(), "Would deploy terraform/dev/network")
			}
		})
	}
}

func TestLifecyclePromptCancellation(t *testing.T) {
	for _, cancelAt := range []int{1, 2} {
		t.Run(strconv.Itoa(cancelAt), func(t *testing.T) {
			cmd, _ := lifecycleFixtureCommand(t, "deploy", "--component")
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
		{[]string{"--component=app"}, nil, errUtils.ErrInvalidFlag},
		{[]string{"--stack=dev", "--component"}, nil, errUtils.ErrInvalidFlag},
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
			cmd, _ := lifecycleFixtureCommand(t, verb, "--stack=dev", "--component=app,typo", "--dry-run")
			output := captureLifecycle(t)
			require.ErrorIs(t, cmd.RunE(cmd, nil), errUtils.ErrInvalidComponent)
			require.NotContains(t, output.String(), "Would ")
		})
	}
	opts := &lifecycleOptions{components: []string{"gone"}, stack: "dev", affected: true}
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
	require.ErrorIs(t, resolveLifecycleTargets(cmd, &lifecycleOptions{stack: "dev", pickComponents: true},
		map[string]any{"dev": map[string]any{}}, nil), errUtils.ErrNoComponentsToSelect)
}

func TestNamedSelectionIgnoresOtherDeletedComponents(t *testing.T) {
	captureLifecycle(t)
	opts := &lifecycleOptions{verb: "deploy", components: []string{"app"}, stack: "dev", affected: true, allowDestroy: true, dryRun: true}
	run := &lifecycleRun{config: &schema.AtmosConfiguration{}, plan: &lifecycle.Plan{Graph: dependency.NewGraph()}, options: opts}
	deleted := []schema.Affected{{ComponentType: "terraform", Stack: "dev", Component: "network", Deleted: true}}
	require.NoError(t, executeLifecyclePlans(newLifecycleCommand("deploy"), run, deleted))
	deleted[0].Component = "app"
	require.ErrorIs(t, executeLifecyclePlans(newLifecycleCommand("deploy"), run, deleted), errUtils.ErrInvalidConfig)
}

func TestLifecycleComponentFlagForms(t *testing.T) {
	for _, verb := range []string{"deploy", "destroy"} {
		for _, args := range [][]string{
			{"--component=app,network"},
			{"--component=app", "--component=network"},
			{"--component=app,network", "--component=app"},
		} {
			t.Run(verb+strings.Join(args, " "), func(t *testing.T) {
				opts, err := lifecycleTestCommand(t, verb, args...)
				require.NoError(t, err)
				require.Equal(t, []string{"app", "network"}, opts.components)
				require.False(t, opts.pickComponents)
			})
		}
	}
	for _, value := range []string{"", "app,", ",app", " "} {
		_, err := lifecycleTestCommand(t, "deploy", "--component="+value)
		require.ErrorIs(t, err, errUtils.ErrInvalidFlag)
	}
}

func TestLifecycleEmptyComponentSelectionDoesNotDeploy(t *testing.T) {
	cmd, _ := lifecycleFixtureCommand(t, "deploy", "--stack=dev", "--component", "--dry-run")
	output := captureLifecycle(t)
	lifecycleInteractiveTest(t, "0\n")
	require.ErrorIs(t, cmd.RunE(cmd, nil), errUtils.ErrMissingInput)
	require.NotContains(t, output.String(), "Would deploy")
}

func TestLifecycleMultipleComponentsPreview(t *testing.T) {
	for _, args := range [][]string{{"--component=app,database"}, {"--component=app", "--component=database"}} {
		cmd, _ := lifecycleFixtureCommand(t, "deploy", append(args, "--stack=dev", "--dry-run")...)
		output := captureLifecycle(t)
		lifecycleInteractiveTest(t)
		viper.Set("interactive", false)
		require.NoError(t, cmd.RunE(cmd, nil))
		require.Contains(t, output.String(), "Would deploy terraform/dev/app")
		require.Contains(t, output.String(), "Would deploy terraform/dev/database")
		require.NotContains(t, output.String(), "Would deploy terraform/dev/network")
	}
}

func TestLifecycleSingleComponentFlag(t *testing.T) {
	opts, err := lifecycleTestCommand(t, "deploy", "--stack=prod", "--component=foo")
	require.NoError(t, err)
	require.Equal(t, []string{"foo"}, opts.components)
	require.False(t, opts.pickComponents)
}

func TestLifecyclePickerRejectsUnknownPreselection(t *testing.T) {
	cmd, _ := lifecycleFixtureCommand(t, "deploy", "--stack=dev", "--component=typo", "--component", "--dry-run")
	output := captureLifecycle(t)
	lifecycleInteractiveTest(t)
	require.ErrorIs(t, cmd.RunE(cmd, nil), errUtils.ErrInvalidComponent)
	require.NotContains(t, output.String(), "Would deploy")
}

func TestLifecycleWholeStackWithoutAllNoninteractive(t *testing.T) {
	for _, verb := range []string{"deploy", "destroy"} {
		cmd, _ := lifecycleFixtureCommand(t, verb, "--stack=platform", "--dry-run")
		output := captureLifecycle(t)
		lifecycleInteractiveTest(t)
		viper.Set("interactive", false)
		require.NoError(t, cmd.RunE(cmd, nil))
		require.Contains(t, output.String(), "Would "+verb+" terraform/platform/registry")
		require.Contains(t, output.String(), "Would "+verb+" helmfile/platform/ingress")
	}
}

func TestLifecyclePickerRequiresTerminalEvenWithBulkScope(t *testing.T) {
	lifecycleInteractiveTest(t)
	for _, opts := range []*lifecycleOptions{
		{all: true, pickComponents: true, ci: true},
		{affected: true, pickComponents: true, ci: true},
	} {
		require.ErrorIs(t, checkLifecyclePromptAvailability(opts), errUtils.ErrInvalidFlag)
	}
}

func TestLifecycleStackPickerRequiresEverySuppliedName(t *testing.T) {
	cmd, _ := lifecycleFixtureCommand(t, "deploy", "--component=app,ingress", "--dry-run")
	output := captureLifecycle(t)
	lifecycleInteractiveTest(t)
	require.ErrorIs(t, cmd.RunE(cmd, nil), errUtils.ErrNoStacksToSelect)
	require.NotContains(t, output.String(), "Would deploy")
}
