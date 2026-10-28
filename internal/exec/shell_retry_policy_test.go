package exec

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy/recovery/recoverytest"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestShellRetryPolicyActualCommand(t *testing.T) {
	for _, tc := range []struct{ kind, phase, parent, nested, event string }{
		{"terraform", "init", "plan", "", "terraform.init.error"},
		{"", "init", "plan", "", "terraform.init.error"},
		{"terraform", "output", "output", "", "terraform.output.error"},
		{"terraform", "workspace-select", "apply", "", "terraform.workspace.select.error"},
		{"terraform", "workspace", "workspace", "select", "terraform.workspace.select.error"},
		{"terraform", "validate", "validate", "", "terraform.*.error"},
		{"helmfile", "sync", "sync", "", "helmfile.apply.error"},
		{"packer", "build", "build", "", "packer.build.error"},
		{"ansible", "playbook", "playbook", "", "ansible.playbook.error"},
	} {
		t.Run(tc.kind+"/"+tc.phase, func(t *testing.T) {
			recoverytest.Install(t, tc.event, "input.stderr.contains('transient') && input.stdout.contains('attempt')")
			info := &schema.ConfigAndStacksInfo{ComponentType: tc.kind, SubCommand: tc.parent, SubCommand2: tc.nested}
			var callerCapture, callerStderr bytes.Buffer
			attempts := 0
			err := ExecuteShellCommandWithRetry(&schema.AtmosConfiguration{}, info, tc.phase, func(options ...ShellCommandOption) error {
				attempts++
				var config shellCommandConfig
				for _, option := range options {
					option(&config)
				}
				_, _ = io.WriteString(config.stdoutCapture, "attempt\n")
				if attempts < 3 {
					_, _ = io.WriteString(config.stderrCapture, "transient\n")
					return errors.New("transient")
				}
				return nil
			}, WithStdoutCapture(&callerCapture), WithStderrCapture(&callerStderr))
			require.NoError(t, err)
			assert.Equal(t, 3, attempts)
			assert.Equal(t, "attempt\nattempt\nattempt\n", callerCapture.String())
			assert.Equal(t, "transient\ntransient\n", callerStderr.String())
		})
	}
}

func TestShellRetryPolicyPreservesPlanDetailedExitCode(t *testing.T) {
	recoverytest.Install(t, "terraform.plan.error", "true")
	info := &schema.ConfigAndStacksInfo{ComponentType: "terraform", SubCommand: "plan"}
	attempts := 0
	detailed := errUtils.ExitCodeError{Code: 2}
	err := ExecuteShellCommandWithRetry(&schema.AtmosConfiguration{}, info, "plan", func(...ShellCommandOption) error { attempts++; return detailed })
	assert.Equal(t, 1, attempts)
	assert.Equal(t, 2, resolveExitCode(err))
}

func TestShellRetryPolicyDoesNotReusePreviousOutput(t *testing.T) {
	recoverytest.Install(t, "terraform.init.error", "input.output.contains('transient')")
	info := &schema.ConfigAndStacksInfo{ComponentType: "terraform", SubCommand: "plan"}
	attempts := 0
	err := ExecuteShellCommandWithRetry(&schema.AtmosConfiguration{}, info, "init", func(options ...ShellCommandOption) error {
		attempts++
		var config shellCommandConfig
		for _, option := range options {
			option(&config)
		}
		if attempts == 1 {
			_, _ = io.WriteString(config.stderrCapture, "transient")
		}
		return errors.New("failed")
	})
	require.Error(t, err)
	assert.Equal(t, 2, attempts)
}

func TestShellRetryPolicyUnselectedEvent(t *testing.T) {
	recoverytest.Install(t, "terraform.init.error", "true")
	calls := 0
	failure := errors.New("output failed")
	err := ExecuteShellCommandWithRetry(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, "output", func(...ShellCommandOption) error { calls++; return failure })
	require.ErrorIs(t, err, failure)
	require.Equal(t, 1, calls)
}
