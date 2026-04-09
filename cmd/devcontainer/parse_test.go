//nolint:dupl // Table-driven test boilerplate - structural similarity across parse function tests is intentional.
package devcontainer

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseTestHelper is a generic helper for testing parse functions.
// It creates a viper instance with the given settings and calls the parse function.
func parseTestHelper[T any](
	t *testing.T,
	viperSettings map[string]interface{},
	parseFunc func(*cobra.Command, *viper.Viper, []string) (T, error),
) T {
	t.Helper()

	// Create a fresh viper instance for this test.
	v := viper.New()
	for key, value := range viperSettings {
		v.Set(key, value)
	}

	// Create a dummy command (not needed for parsing, but required by signature).
	cmd := &cobra.Command{}

	// Parse options.
	opts, err := parseFunc(cmd, v, []string{})
	require.NoError(t, err)

	return opts
}

func TestParseAttachOptions(t *testing.T) {
	tests := []struct {
		name            string
		viperSettings   map[string]interface{}
		expectedOptions *AttachOptions
	}{
		{
			name: "default values",
			viperSettings: map[string]interface{}{
				"instance": "default",
				"pty":      false,
			},
			expectedOptions: &AttachOptions{
				Instance: "default",
				UsePTY:   false,
			},
		},
		{
			name: "custom instance with PTY",
			viperSettings: map[string]interface{}{
				"instance": "my-custom-instance",
				"pty":      true,
			},
			expectedOptions: &AttachOptions{
				Instance: "my-custom-instance",
				UsePTY:   true,
			},
		},
		{
			name: "PTY enabled without instance override",
			viperSettings: map[string]interface{}{
				"instance": "dev",
				"pty":      true,
			},
			expectedOptions: &AttachOptions{
				Instance: "dev",
				UsePTY:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseTestHelper(t, tt.viperSettings, parseAttachOptions)

			// Verify parsed options.
			assert.Equal(t, tt.expectedOptions.Instance, opts.Instance)
			assert.Equal(t, tt.expectedOptions.UsePTY, opts.UsePTY)
		})
	}
}

func TestParseExecOptions(t *testing.T) {
	tests := []struct {
		name            string
		viperSettings   map[string]interface{}
		expectedOptions *ExecOptions
	}{
		{
			name: "default values",
			viperSettings: map[string]interface{}{
				"instance":    "default",
				"interactive": false,
				"pty":         false,
			},
			expectedOptions: &ExecOptions{
				Instance:    "default",
				Interactive: false,
				UsePTY:      false,
			},
		},
		{
			name: "interactive mode enabled",
			viperSettings: map[string]interface{}{
				"instance":    "test-instance",
				"interactive": true,
				"pty":         false,
			},
			expectedOptions: &ExecOptions{
				Instance:    "test-instance",
				Interactive: true,
				UsePTY:      false,
			},
		},
		{
			name: "PTY mode enabled",
			viperSettings: map[string]interface{}{
				"instance":    "prod-instance",
				"interactive": false,
				"pty":         true,
			},
			expectedOptions: &ExecOptions{
				Instance:    "prod-instance",
				Interactive: false,
				UsePTY:      true,
			},
		},
		{
			name: "both interactive and PTY enabled",
			viperSettings: map[string]interface{}{
				"instance":    "staging",
				"interactive": true,
				"pty":         true,
			},
			expectedOptions: &ExecOptions{
				Instance:    "staging",
				Interactive: true,
				UsePTY:      true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseTestHelper(t, tt.viperSettings, parseExecOptions)

			// Verify parsed options.
			assert.Equal(t, tt.expectedOptions.Instance, opts.Instance)
			assert.Equal(t, tt.expectedOptions.Interactive, opts.Interactive)
			assert.Equal(t, tt.expectedOptions.UsePTY, opts.UsePTY)
		})
	}
}

func TestParseLogsOptions(t *testing.T) {
	tests := []struct {
		name            string
		viperSettings   map[string]interface{}
		expectedOptions *LogsOptions
	}{
		{
			name: "default values",
			viperSettings: map[string]interface{}{
				"instance": "default",
				"follow":   false,
				"tail":     "",
			},
			expectedOptions: &LogsOptions{
				Instance: "default",
				Follow:   false,
				Tail:     "",
			},
		},
		{
			name: "follow mode enabled",
			viperSettings: map[string]interface{}{
				"instance": "test-instance",
				"follow":   true,
				"tail":     "",
			},
			expectedOptions: &LogsOptions{
				Instance: "test-instance",
				Follow:   true,
				Tail:     "",
			},
		},
		{
			name: "tail last 100 lines",
			viperSettings: map[string]interface{}{
				"instance": "prod-instance",
				"follow":   false,
				"tail":     "100",
			},
			expectedOptions: &LogsOptions{
				Instance: "prod-instance",
				Follow:   false,
				Tail:     "100",
			},
		},
		{
			name: "follow with tail",
			viperSettings: map[string]interface{}{
				"instance": "staging",
				"follow":   true,
				"tail":     "50",
			},
			expectedOptions: &LogsOptions{
				Instance: "staging",
				Follow:   true,
				Tail:     "50",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseTestHelper(t, tt.viperSettings, parseLogsOptions)

			// Verify parsed options.
			assert.Equal(t, tt.expectedOptions.Instance, opts.Instance)
			assert.Equal(t, tt.expectedOptions.Follow, opts.Follow)
			assert.Equal(t, tt.expectedOptions.Tail, opts.Tail)
		})
	}
}

func TestParseRebuildOptions(t *testing.T) {
	tests := []struct {
		name            string
		viperSettings   map[string]interface{}
		expectedOptions *RebuildOptions
	}{
		{
			name: "default values",
			viperSettings: map[string]interface{}{
				"instance": "default",
				"attach":   false,
				"no-pull":  false,
				"identity": "",
			},
			expectedOptions: &RebuildOptions{
				Instance: "default",
				Attach:   false,
				NoPull:   false,
				Identity: "",
			},
		},
		{
			name: "attach enabled",
			viperSettings: map[string]interface{}{
				"instance": "test-instance",
				"attach":   true,
				"no-pull":  false,
				"identity": "",
			},
			expectedOptions: &RebuildOptions{
				Instance: "test-instance",
				Attach:   true,
				NoPull:   false,
				Identity: "",
			},
		},
		{
			name: "no-pull enabled with identity",
			viperSettings: map[string]interface{}{
				"instance": "prod-rebuild",
				"attach":   false,
				"no-pull":  true,
				"identity": "user@example.com",
			},
			expectedOptions: &RebuildOptions{
				Instance: "prod-rebuild",
				Attach:   false,
				NoPull:   true,
				Identity: "user@example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseTestHelper(t, tt.viperSettings, parseRebuildOptions)

			// Verify parsed options.
			assert.Equal(t, tt.expectedOptions.Instance, opts.Instance)
			assert.Equal(t, tt.expectedOptions.Attach, opts.Attach)
			assert.Equal(t, tt.expectedOptions.NoPull, opts.NoPull)
			assert.Equal(t, tt.expectedOptions.Identity, opts.Identity)
		})
	}
}

func TestParseRemoveOptions(t *testing.T) {
	tests := []struct {
		name            string
		viperSettings   map[string]interface{}
		expectedOptions *RemoveOptions
	}{
		{
			name: "default values",
			viperSettings: map[string]interface{}{
				"instance": "default",
				"force":    false,
			},
			expectedOptions: &RemoveOptions{
				Instance: "default",
				Force:    false,
			},
		},
		{
			name: "force remove enabled",
			viperSettings: map[string]interface{}{
				"instance": "test-instance",
				"force":    true,
			},
			expectedOptions: &RemoveOptions{
				Instance: "test-instance",
				Force:    true,
			},
		},
		{
			name: "custom instance without force",
			viperSettings: map[string]interface{}{
				"instance": "old-container",
				"force":    false,
			},
			expectedOptions: &RemoveOptions{
				Instance: "old-container",
				Force:    false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseTestHelper(t, tt.viperSettings, parseRemoveOptions)

			// Verify parsed options.
			assert.Equal(t, tt.expectedOptions.Instance, opts.Instance)
			assert.Equal(t, tt.expectedOptions.Force, opts.Force)
		})
	}
}

func TestParseStartOptions(t *testing.T) {
	tests := []struct {
		name            string
		viperSettings   map[string]interface{}
		expectedOptions *StartOptions
	}{
		{
			name: "default values",
			viperSettings: map[string]interface{}{
				"instance": "default",
				"attach":   false,
				"identity": "",
			},
			expectedOptions: &StartOptions{
				Instance: "default",
				Attach:   false,
				Identity: "",
			},
		},
		{
			name: "attach mode enabled",
			viperSettings: map[string]interface{}{
				"instance": "test-instance",
				"attach":   true,
				"identity": "",
			},
			expectedOptions: &StartOptions{
				Instance: "test-instance",
				Attach:   true,
				Identity: "",
			},
		},
		{
			name: "custom instance without attach",
			viperSettings: map[string]interface{}{
				"instance": "background-container",
				"attach":   false,
				"identity": "",
			},
			expectedOptions: &StartOptions{
				Instance: "background-container",
				Attach:   false,
				Identity: "",
			},
		},
		{
			name: "with identity authentication",
			viperSettings: map[string]interface{}{
				"instance": "auth-container",
				"attach":   true,
				"identity": "user@example.com",
			},
			expectedOptions: &StartOptions{
				Instance: "auth-container",
				Attach:   true,
				Identity: "user@example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseTestHelper(t, tt.viperSettings, parseStartOptions)

			// Verify parsed options.
			assert.Equal(t, tt.expectedOptions.Instance, opts.Instance)
			assert.Equal(t, tt.expectedOptions.Attach, opts.Attach)
			assert.Equal(t, tt.expectedOptions.Identity, opts.Identity)
		})
	}
}

func TestParseStopOptions(t *testing.T) {
	tests := []struct {
		name            string
		viperSettings   map[string]interface{}
		expectedOptions *StopOptions
	}{
		{
			name: "default values",
			viperSettings: map[string]interface{}{
				"instance": "default",
				"timeout":  10,
				"rm":       false,
			},
			expectedOptions: &StopOptions{
				Instance: "default",
				Timeout:  10,
				Rm:       false,
			},
		},
		{
			name: "custom timeout",
			viperSettings: map[string]interface{}{
				"instance": "test-instance",
				"timeout":  30,
				"rm":       false,
			},
			expectedOptions: &StopOptions{
				Instance: "test-instance",
				Timeout:  30,
				Rm:       false,
			},
		},
		{
			name: "zero timeout (immediate)",
			viperSettings: map[string]interface{}{
				"instance": "immediate-stop",
				"timeout":  0,
				"rm":       false,
			},
			expectedOptions: &StopOptions{
				Instance: "immediate-stop",
				Timeout:  0,
				Rm:       false,
			},
		},
		{
			name: "long timeout",
			viperSettings: map[string]interface{}{
				"instance": "graceful-shutdown",
				"timeout":  120,
				"rm":       false,
			},
			expectedOptions: &StopOptions{
				Instance: "graceful-shutdown",
				Timeout:  120,
				Rm:       false,
			},
		},
		{
			name: "with auto-remove",
			viperSettings: map[string]interface{}{
				"instance": "ephemeral",
				"timeout":  10,
				"rm":       true,
			},
			expectedOptions: &StopOptions{
				Instance: "ephemeral",
				Timeout:  10,
				Rm:       true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseTestHelper(t, tt.viperSettings, parseStopOptions)

			// Verify parsed options.
			assert.Equal(t, tt.expectedOptions.Instance, opts.Instance)
			assert.Equal(t, tt.expectedOptions.Timeout, opts.Timeout)
			assert.Equal(t, tt.expectedOptions.Rm, opts.Rm)
		})
	}
}

func TestParseShellOptions(t *testing.T) {
	tests := []struct {
		name            string
		viperSettings   map[string]interface{}
		expectedOptions *ShellOptions
	}{
		{
			name: "default values",
			viperSettings: map[string]interface{}{
				"instance": "default",
				"identity": "",
				"pty":      false,
				"new":      false,
				"replace":  false,
				"rm":       false,
				"no-pull":  false,
			},
			expectedOptions: &ShellOptions{
				Instance: "default",
				Identity: "",
				UsePTY:   false,
				New:      false,
				Replace:  false,
				Rm:       false,
				NoPull:   false,
			},
		},
		{
			name: "PTY mode enabled",
			viperSettings: map[string]interface{}{
				"instance": "test-instance",
				"identity": "",
				"pty":      true,
				"new":      false,
				"replace":  false,
				"rm":       false,
				"no-pull":  false,
			},
			expectedOptions: &ShellOptions{
				Instance: "test-instance",
				Identity: "",
				UsePTY:   true,
				New:      false,
				Replace:  false,
				Rm:       false,
				NoPull:   false,
			},
		},
		{
			name: "new shell with identity",
			viperSettings: map[string]interface{}{
				"instance": "prod-instance",
				"identity": "user@example.com",
				"pty":      false,
				"new":      true,
				"replace":  false,
				"rm":       false,
				"no-pull":  false,
			},
			expectedOptions: &ShellOptions{
				Instance: "prod-instance",
				Identity: "user@example.com",
				UsePTY:   false,
				New:      true,
				Replace:  false,
				Rm:       false,
				NoPull:   false,
			},
		},
		{
			name: "replace existing shell",
			viperSettings: map[string]interface{}{
				"instance": "staging",
				"identity": "",
				"pty":      true,
				"new":      false,
				"replace":  true,
				"rm":       false,
				"no-pull":  false,
			},
			expectedOptions: &ShellOptions{
				Instance: "staging",
				Identity: "",
				UsePTY:   true,
				New:      false,
				Replace:  true,
				Rm:       false,
				NoPull:   false,
			},
		},
		{
			name: "with auto-remove and no-pull",
			viperSettings: map[string]interface{}{
				"instance": "ephemeral",
				"identity": "",
				"pty":      false,
				"new":      true,
				"replace":  false,
				"rm":       true,
				"no-pull":  true,
			},
			expectedOptions: &ShellOptions{
				Instance: "ephemeral",
				Identity: "",
				UsePTY:   false,
				New:      true,
				Replace:  false,
				Rm:       true,
				NoPull:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := parseTestHelper(t, tt.viperSettings, parseShellOptions)

			// Verify parsed options.
			assert.Equal(t, tt.expectedOptions.Instance, opts.Instance)
			assert.Equal(t, tt.expectedOptions.Identity, opts.Identity)
			assert.Equal(t, tt.expectedOptions.UsePTY, opts.UsePTY)
			assert.Equal(t, tt.expectedOptions.New, opts.New)
			assert.Equal(t, tt.expectedOptions.Replace, opts.Replace)
			assert.Equal(t, tt.expectedOptions.Rm, opts.Rm)
			assert.Equal(t, tt.expectedOptions.NoPull, opts.NoPull)
		})
	}
}
