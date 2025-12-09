package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestNewDefaultCommandRunner tests the constructor.
func TestNewDefaultCommandRunner(t *testing.T) {
	shellExec := func(command, name, dir string, env []string, dryRun bool) error {
		return nil
	}
	atmosExec := func(params *AtmosExecParams) error {
		return nil
	}

	runner := NewDefaultCommandRunner(shellExec, atmosExec)

	assert.NotNil(t, runner)
	assert.NotNil(t, runner.shellExecutor)
	assert.NotNil(t, runner.atmosExecutor)
}

// TestDefaultCommandRunner_RunShell tests the RunShell method.
func TestDefaultCommandRunner_RunShell(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		cmdName     string
		dir         string
		env         []string
		dryRun      bool
		returnError error
		wantError   bool
	}{
		{
			name:        "successful execution",
			command:     "echo hello",
			cmdName:     "test-cmd",
			dir:         ".",
			env:         []string{"VAR=value"},
			dryRun:      false,
			returnError: nil,
			wantError:   false,
		},
		{
			name:        "dry run",
			command:     "echo hello",
			cmdName:     "test-cmd",
			dir:         "/tmp",
			env:         nil,
			dryRun:      true,
			returnError: nil,
			wantError:   false,
		},
		{
			name:        "execution failure",
			command:     "exit 1",
			cmdName:     "failing-cmd",
			dir:         ".",
			env:         nil,
			dryRun:      false,
			returnError: errors.New("command failed"),
			wantError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedCommand, capturedName, capturedDir string
			var capturedEnv []string
			var capturedDryRun bool

			shellExec := func(command, name, dir string, env []string, dryRun bool) error {
				capturedCommand = command
				capturedName = name
				capturedDir = dir
				capturedEnv = env
				capturedDryRun = dryRun
				return tt.returnError
			}

			runner := NewDefaultCommandRunner(shellExec, nil)
			err := runner.RunShell(tt.command, tt.cmdName, tt.dir, tt.env, tt.dryRun)

			if tt.wantError {
				require.Error(t, err)
				assert.Equal(t, tt.returnError, err)
			} else {
				require.NoError(t, err)
			}

			// Verify parameters were passed correctly.
			assert.Equal(t, tt.command, capturedCommand)
			assert.Equal(t, tt.cmdName, capturedName)
			assert.Equal(t, tt.dir, capturedDir)
			assert.Equal(t, tt.env, capturedEnv)
			assert.Equal(t, tt.dryRun, capturedDryRun)
		})
	}
}

// TestDefaultCommandRunner_RunShell_NilExecutor tests error on nil shell executor.
func TestDefaultCommandRunner_RunShell_NilExecutor(t *testing.T) {
	runner := &DefaultCommandRunner{
		shellExecutor: nil,
		atmosExecutor: nil,
	}

	err := runner.RunShell("echo hello", "test", ".", nil, false)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)
}

// TestDefaultCommandRunner_RunAtmos tests the RunAtmos method.
func TestDefaultCommandRunner_RunAtmos(t *testing.T) {
	tests := []struct {
		name        string
		params      *AtmosExecParams
		returnError error
		wantError   bool
	}{
		{
			name: "successful execution",
			params: &AtmosExecParams{
				Ctx:         context.Background(),
				AtmosConfig: &schema.AtmosConfiguration{},
				Args:        []string{"terraform", "plan", "vpc"},
				Dir:         ".",
				Env:         []string{"VAR=value"},
				DryRun:      false,
			},
			returnError: nil,
			wantError:   false,
		},
		{
			name: "dry run",
			params: &AtmosExecParams{
				Ctx:         context.Background(),
				AtmosConfig: &schema.AtmosConfiguration{},
				Args:        []string{"version"},
				Dir:         "/tmp",
				Env:         nil,
				DryRun:      true,
			},
			returnError: nil,
			wantError:   false,
		},
		{
			name: "execution failure",
			params: &AtmosExecParams{
				Ctx:         context.Background(),
				AtmosConfig: &schema.AtmosConfiguration{},
				Args:        []string{"invalid", "command"},
				Dir:         ".",
				Env:         nil,
				DryRun:      false,
			},
			returnError: errors.New("command failed"),
			wantError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedParams *AtmosExecParams

			atmosExec := func(params *AtmosExecParams) error {
				capturedParams = params
				return tt.returnError
			}

			runner := NewDefaultCommandRunner(nil, atmosExec)
			err := runner.RunAtmos(tt.params)

			if tt.wantError {
				require.Error(t, err)
				assert.Equal(t, tt.returnError, err)
			} else {
				require.NoError(t, err)
			}

			// Verify the params were passed correctly.
			assert.Equal(t, tt.params, capturedParams)
		})
	}
}

// TestDefaultCommandRunner_RunAtmos_NilExecutor tests error on nil atmos executor.
func TestDefaultCommandRunner_RunAtmos_NilExecutor(t *testing.T) {
	runner := &DefaultCommandRunner{
		shellExecutor: nil,
		atmosExecutor: nil,
	}

	params := &AtmosExecParams{
		Ctx:         context.Background(),
		AtmosConfig: &schema.AtmosConfiguration{},
		Args:        []string{"version"},
		Dir:         ".",
	}

	err := runner.RunAtmos(params)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)
}

// TestDefaultCommandRunner_RunAtmos_NilParams tests error on nil params.
func TestDefaultCommandRunner_RunAtmos_NilParams(t *testing.T) {
	runner := &DefaultCommandRunner{
		shellExecutor: nil,
		atmosExecutor: func(params *AtmosExecParams) error { return nil },
	}

	err := runner.RunAtmos(nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)
}

// TestDefaultCommandRunner_RunAtmos_NilAtmosConfig tests error on nil AtmosConfig.
func TestDefaultCommandRunner_RunAtmos_NilAtmosConfig(t *testing.T) {
	runner := &DefaultCommandRunner{
		shellExecutor: nil,
		atmosExecutor: func(params *AtmosExecParams) error { return nil },
	}

	params := &AtmosExecParams{
		Ctx:         context.Background(),
		AtmosConfig: nil,
		Args:        []string{"version"},
		Dir:         ".",
	}

	err := runner.RunAtmos(params)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)
}

// TestDefaultCommandRunner_InterfaceCompliance verifies that DefaultCommandRunner implements the CommandRunner interface.
func TestDefaultCommandRunner_InterfaceCompliance(t *testing.T) {
	var _ CommandRunner = (*DefaultCommandRunner)(nil)
}
