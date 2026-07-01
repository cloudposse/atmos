package process

import (
	"io"
	"strings"
	"testing"

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
