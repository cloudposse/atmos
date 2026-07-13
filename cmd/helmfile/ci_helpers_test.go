package helmfile

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

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

// TestRunHelmfileCIHook_NoCI verifies the CI hook runs to completion (config
// init + RunCIHooks) without panicking when CI mode is off.
func TestRunHelmfileCIHook_NoCI(t *testing.T) {
	t.Setenv("ATMOS_CI", "")
	t.Setenv("CI", "")

	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "app", Stack: "dev"}
	runHelmfileCIHook("apply", info, "rendered output", nil, false)
}
