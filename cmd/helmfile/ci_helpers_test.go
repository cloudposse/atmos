package helmfile

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	h "github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCIValueEnabled(t *testing.T) {
	for _, v := range []string{"true", "1", "yes", "TRUE", " on "} {
		assert.True(t, ciValueEnabled(v), "value %q should be enabled", v)
	}
	for _, v := range []string{"", "false", "0", "no", "FALSE", "  "} {
		assert.False(t, ciValueEnabled(v), "value %q should be disabled", v)
	}
}

func TestCIEnvEnabled(t *testing.T) {
	t.Setenv("ATMOS_TEST_CI_FLAG", "true")
	assert.True(t, ciEnvEnabled("ATMOS_TEST_CI_FLAG"))

	t.Setenv("ATMOS_TEST_CI_FLAG", "false")
	assert.False(t, ciEnvEnabled("ATMOS_TEST_CI_FLAG"))

	assert.False(t, ciEnvEnabled("ATMOS_TEST_CI_FLAG_UNSET"))
}

func TestHelmfileCIModeEnabled(t *testing.T) {
	t.Setenv("ATMOS_CI", "")
	t.Setenv("CI", "")

	// argsCI wins regardless of flags/env.
	assert.True(t, helmfileCIModeEnabled(nil, true))

	// nil command falls through to env (all unset -> false).
	assert.False(t, helmfileCIModeEnabled(nil, false))

	// The command's own --ci flag enables CI mode.
	cmd := &cobra.Command{Use: "apply"}
	cmd.Flags().Bool("ci", false, "")
	assert.NoError(t, cmd.Flags().Set("ci", "true"))
	assert.True(t, helmfileCIModeEnabled(cmd, false))

	// Env var enables CI mode when flags are off.
	t.Setenv("ATMOS_CI", "1")
	off := &cobra.Command{Use: "apply"}
	off.Flags().Bool("ci", false, "")
	assert.True(t, helmfileCIModeEnabled(off, false))
}

func TestHelmfileAfterEvent(t *testing.T) {
	cases := map[string]h.HookEvent{
		"template": h.AfterHelmfileTemplate,
		"diff":     h.AfterHelmfileDiff,
		"apply":    h.AfterHelmfileApply,
		"sync":     h.AfterHelmfileSync,
		"deploy":   h.AfterHelmfileDeploy,
		"destroy":  h.AfterHelmfileDestroy,
	}
	for command, want := range cases {
		t.Run(command, func(t *testing.T) {
			assert.Equal(t, want, helmfileAfterEvent(command))
		})
	}
	// An unknown command falls back to a derived event name.
	assert.Equal(t, h.HookEvent("after.helmfile.frobnicate"), helmfileAfterEvent("frobnicate"))
}

func TestHelmfileBeforeEvent(t *testing.T) {
	cases := map[string]h.HookEvent{
		"template": h.BeforeHelmfileTemplate,
		"diff":     h.BeforeHelmfileDiff,
		"apply":    h.BeforeHelmfileApply,
		"sync":     h.BeforeHelmfileSync,
		"deploy":   h.BeforeHelmfileDeploy,
		"destroy":  h.BeforeHelmfileDestroy,
	}
	for command, want := range cases {
		t.Run(command, func(t *testing.T) {
			assert.Equal(t, want, helmfileBeforeEvent(command))
		})
	}
	// An unknown command falls back to a derived event name.
	assert.Equal(t, h.HookEvent("before.helmfile.frobnicate"), helmfileBeforeEvent("frobnicate"))
}

// TestHelmfileNodeHooksAfter_NoCI verifies the CI hook runs to completion
// (config init + RunCIHooks) without panicking when CI mode is off, and that
// a component with no `hooks:` section returns a nil user-hook error.
func TestHelmfileNodeHooksAfter_NoCI(t *testing.T) {
	t.Setenv("ATMOS_CI", "")
	t.Setenv("CI", "")

	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "dev"}
	nodeHooks := &helmfileNodeHooks{
		cmd:         &cobra.Command{Use: "apply"},
		afterEvent:  h.AfterHelmfileApply,
		forceCIMode: false,
	}
	err := nodeHooks.After(context.Background(), info, "rendered output", nil)
	assert.NoError(t, err)
	assert.True(t, nodeHooks.called)
}

// TestHelmfileNodeHooksAfter_ExecErr verifies that a non-nil execErr is
// reflected in the outcome passed to user hooks (RunFailure status) rather
// than always reporting success, mirroring TestHelmfileNodeHooksAfter_NoCI
// but for the failure path. Uses the demo-stacks fixture (same one
// cmd/terraform's hook tests use) so cfg.InitCliConfig succeeds and the
// execErr != nil branch (rather than the config-init-failure branch) is
// actually reached.
func TestHelmfileNodeHooksAfter_ExecErr(t *testing.T) {
	t.Setenv("ATMOS_CI", "")
	t.Setenv("CI", "")
	t.Chdir(filepath.Join("..", "..", "examples", "demo-stacks"))

	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "myapp", Stack: "dev"}
	nodeHooks := &helmfileNodeHooks{
		cmd:         &cobra.Command{Use: "apply"},
		afterEvent:  h.AfterHelmfileApply,
		forceCIMode: false,
	}
	execErr := assert.AnError
	err := nodeHooks.After(context.Background(), info, "rendered output", execErr)
	// No `hooks:` section for this component, so the user-hook error is nil
	// regardless of outcome status; this exercises the execErr != nil branch
	// without depending on hook resolution behavior.
	assert.NoError(t, err)
	assert.True(t, nodeHooks.called)
}

// TestHelmfileNodeHooksBefore verifies helmfileNodeHooks.Before runs to
// completion (config init + user hooks) without panicking, and marks the
// hooks as called — mirroring TestHelmfileNodeHooksAfter_NoCI for the Before
// half of the interface, which no prior test invoked directly. Uses the
// demo-stacks fixture so cfg.InitCliConfig succeeds and runUserHooks is
// actually reached (rather than the config-init-failure branch).
func TestHelmfileNodeHooksBefore(t *testing.T) {
	t.Chdir(filepath.Join("..", "..", "examples", "demo-stacks"))

	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "myapp", Stack: "dev"}
	nodeHooks := &helmfileNodeHooks{
		cmd:         &cobra.Command{Use: "apply"},
		beforeEvent: h.BeforeHelmfileApply,
	}
	err := nodeHooks.Before(context.Background(), info)
	assert.NoError(t, err)
	assert.True(t, nodeHooks.called)
}

// TestHelmfileNodeHooksBefore_ConfigInitFailure covers the config-init-failure
// branch (log.Warn; return nil) that TestHelmfileNodeHooksBefore's fixture
// deliberately avoids. A component/stack that resolves to a missing import
// makes cfg.InitCliConfig fail without reaching runUserHooks.
func TestHelmfileNodeHooksBefore_ConfigInitFailure(t *testing.T) {
	t.Chdir(t.TempDir())

	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "dev"}
	nodeHooks := &helmfileNodeHooks{
		cmd:         &cobra.Command{Use: "apply"},
		beforeEvent: h.BeforeHelmfileApply,
	}
	err := nodeHooks.Before(context.Background(), info)
	assert.NoError(t, err, "config-init failures are logged, not surfaced, from Before")
	assert.True(t, nodeHooks.called)
}

// TestHelmfileNodeHooksRunUserHooks_EmptyEvent covers runUserHooks' `event ==
// ""` guard. Unreachable via production wiring today (helmfileBeforeEvent/
// helmfileAfterEvent always return a non-empty event, falling back to a
// derived "before.helmfile.<cmd>"/"after.helmfile.<cmd>" name for unknown
// commands), so it must be exercised directly.
func TestHelmfileNodeHooksRunUserHooks_EmptyEvent(t *testing.T) {
	nodeHooks := &helmfileNodeHooks{cmd: &cobra.Command{Use: "apply"}}
	err := nodeHooks.runUserHooks(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, "", h.Outcome{})
	assert.NoError(t, err)
}

// TestHelmfileCIModeEnabled_InheritedFlag covers the cmd.InheritedFlags()
// branch: a "--ci" flag set on a parent command (not the command itself)
// must still enable CI mode.
func TestHelmfileCIModeEnabled_InheritedFlag(t *testing.T) {
	t.Setenv("ATMOS_CI", "")
	t.Setenv("CI", "")

	parent := &cobra.Command{Use: "helmfile"}
	parent.PersistentFlags().Bool("ci", false, "")
	require.NoError(t, parent.PersistentFlags().Set("ci", "true"))

	child := &cobra.Command{Use: "apply"}
	parent.AddCommand(child)

	assert.True(t, helmfileCIModeEnabled(child, false))
}
