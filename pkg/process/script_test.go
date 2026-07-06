package process

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
