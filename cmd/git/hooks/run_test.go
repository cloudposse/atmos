package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ---- extractHookNameAndArgs ----

func TestExtractHookNameAndArgs_Basic(t *testing.T) {
	name, args := extractHookNameAndArgs([]string{"pre-commit"})
	assert.Equal(t, "pre-commit", name)
	assert.Empty(t, args)
}

func TestExtractHookNameAndArgs_WithArgs(t *testing.T) {
	name, args := extractHookNameAndArgs([]string{"commit-msg", ".git/COMMIT_EDITMSG"})
	assert.Equal(t, "commit-msg", name)
	assert.Equal(t, []string{".git/COMMIT_EDITMSG"}, args)
}

func TestExtractHookNameAndArgs_Empty(t *testing.T) {
	name, args := extractHookNameAndArgs(nil)
	assert.Equal(t, "", name)
	assert.Nil(t, args)
}

func TestExtractHookNameAndArgs_SkipsLeadingFlags(t *testing.T) {
	name, args := extractHookNameAndArgs([]string{"--verbose", "pre-commit", "extra"})
	assert.Equal(t, "pre-commit", name)
	assert.Equal(t, []string{"extra"}, args)
}

// ---- isHelpRequest ----

func TestIsHelpRequest(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"long help flag", []string{"--help"}, true},
		{"short help flag", []string{"-h"}, true},
		{"help after hook name", []string{"pre-commit", "--help"}, true},
		{"no help flag", []string{"pre-commit"}, false},
		{"empty", nil, false},
		{"help after separator is not ours", []string{"pre-commit", "--", "--help"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isHelpRequest(tt.args))
		})
	}
}

// TestRunCmd_HelpFlagShowsHelp verifies that a help flag yields help (nil error)
// rather than the ErrGitHookNotConfigured config error. DisableFlagParsing means
// cobra does not auto-handle help, so RunE must detect it explicitly.
func TestRunCmd_HelpFlagShowsHelp(t *testing.T) {
	require.NoError(t, runCmd.RunE(runCmd, []string{"--help"}))
}

// ---- config plumbing ----

func TestGitConfig_NilWhenUnset(t *testing.T) {
	original := atmosConfigPtr
	t.Cleanup(func() { atmosConfigPtr = original })

	SetAtmosConfig(nil)
	assert.Nil(t, gitConfig())
}

// ---- metrics default ----

func TestSuppressMetricsSummaryByDefault(t *testing.T) {
	enabled, disabled := true, false
	tests := []struct {
		name   string
		in     *bool
		expect bool
	}{
		{"unset defaults to off", nil, false},
		{"explicit true is preserved", &enabled, true},
		{"explicit false is preserved", &disabled, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			config.Settings.Metrics.Enabled = tt.in
			cfg.DisableMetricsSummaryByDefault(config)
			require.NotNil(t, config.Settings.Metrics.Enabled)
			assert.Equal(t, tt.expect, *config.Settings.Metrics.Enabled)
		})
	}

	t.Run("nil config is a no-op", func(t *testing.T) {
		assert.NotPanics(t, func() { cfg.DisableMetricsSummaryByDefault(nil) })
	})
}
