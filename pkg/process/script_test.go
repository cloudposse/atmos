package process

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain lets the test binary itself act as a cross-platform "exit 0" or
// "exit 1" command for tests that need a real, portable executable to run as
// a script interpreter (avoids depending on bash/sh, which don't exist on
// Windows).
func TestMain(m *testing.M) {
	// If _ATMOS_TEST_EXIT_ZERO is set, exit immediately with code 0.
	if os.Getenv("_ATMOS_TEST_EXIT_ZERO") == "1" {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestScriptInvocation(t *testing.T) {
	tests := []struct {
		name      string
		interp    string
		script    string
		wantArgv  []string
		wantStdin string
		hasStdin  bool
	}{
		{
			name:      "python3 uses stdin",
			interp:    "python3",
			script:    "print('ok')",
			wantArgv:  []string{"python3", "-"},
			wantStdin: "print('ok')",
			hasStdin:  true,
		},
		{
			name:     "bash uses command flag",
			interp:   "bash",
			script:   "echo ok",
			wantArgv: []string{"bash", "-c", "echo ok"},
		},
		{
			name:     "node uses eval flag",
			interp:   "node",
			script:   "console.log('ok')",
			wantArgv: []string{"node", "-e", "console.log('ok')"},
		},
		{
			name:      "pwsh uses command stdin",
			interp:    "pwsh",
			script:    "Write-Output ok",
			wantArgv:  []string{"pwsh", "-NoProfile", "-NonInteractive", "-Command", "-"},
			wantStdin: "Write-Output ok",
			hasStdin:  true,
		},
		{
			name:     "cmd uses command shell",
			interp:   "cmd.exe",
			script:   "echo ok",
			wantArgv: []string{"cmd.exe", "/S", "/C", "echo ok"},
		},
		{
			name:      "unknown uses stdin",
			interp:    "ruby",
			script:    "puts 'ok'",
			wantArgv:  []string{"ruby", "-"},
			wantStdin: "puts 'ok'",
			hasStdin:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			argv, stdin := ScriptInvocation(tt.interp, tt.script)
			assert.Equal(t, tt.wantArgv, argv)
			if tt.hasStdin {
				require.NotNil(t, stdin)
				got, err := io.ReadAll(stdin)
				require.NoError(t, err)
				assert.Equal(t, tt.wantStdin, string(got))
				return
			}
			assert.Nil(t, stdin)
		})
	}
}

func TestFormatScriptDisplay(t *testing.T) {
	display := FormatScriptDisplay("python3", "print('ok')")
	assert.True(t, strings.HasPrefix(display, "python3 <<'SCRIPT'\n"))
	assert.Contains(t, display, "print('ok')")
	assert.True(t, strings.HasSuffix(display, "\nSCRIPT"))
}

func TestRunScriptWrapsErrors(t *testing.T) {
	err := RunScript(context.Background(), &ScriptSpec{
		Interpreter: "definitely-not-a-real-interpreter",
		Script:      "echo ok",
	}, io.Discard, io.Discard)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrProcessWaitFailed), "error = %v", err)
}

func TestRunScript_DryRunSkipsExecution(t *testing.T) {
	err := RunScript(context.Background(), &ScriptSpec{
		Interpreter: "definitely-not-a-real-interpreter",
		Script:      "echo ok",
		DryRun:      true,
	}, io.Discard, io.Discard)

	require.NoError(t, err)
}

// TestRunScript_Success uses the current test binary as the "interpreter" so
// the test stays cross-platform (no reliance on bash/sh being present). The
// _ATMOS_TEST_EXIT_ZERO env var (handled by TestMain) makes the test binary
// exit 0 immediately without running any tests, simulating a successful
// script execution.
func TestRunScript_Success(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	runErr := RunScript(context.Background(), &ScriptSpec{
		Interpreter: exePath,
		Script:      "",
		Env:         append(os.Environ(), "_ATMOS_TEST_EXIT_ZERO=1"),
	}, &stdout, &stderr)

	require.NoError(t, runErr)
}

func TestNewScriptCommand_NilContextDefaultsToBackground(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)

	//nolint:staticcheck // Intentionally passing nil to exercise the ctx==nil default branch.
	cmd := NewScriptCommand(nil, &ScriptSpec{
		Interpreter: exePath,
		Script:      "",
	})

	require.NotNil(t, cmd)
	assert.NotNil(t, cmd.Env)
}

func TestFormatScriptDisplay_EmptyInputsReturnEmpty(t *testing.T) {
	assert.Equal(t, "", FormatScriptDisplay("", "echo ok"))
	assert.Equal(t, "", FormatScriptDisplay("bash", ""))
	assert.Equal(t, "", FormatScriptDisplay("   ", "   "))
}
