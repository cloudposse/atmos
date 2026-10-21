package admission

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvocationPinsAgents(t *testing.T) {
	directory, err := filepath.Abs(filepath.Join("..", "..", "..", "policy-library", "command-deny-destroy"))
	require.NoError(t, err)
	t.Setenv("ATMOS_POLICY_DIR", directory)
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CODEX_THREAD_ID", "thread")
	end, err := BeginInvocation()
	require.NoError(t, err)
	t.Cleanup(end)
	t.Setenv("CLAUDECODE", "")
	t.Setenv("CODEX_THREAD_ID", "")
	names := AgentNames()
	require.Equal(t, []string{"claude-code", "codex"}, names)
	names[0] = "modified"
	require.Equal(t, []string{"claude-code", "codex"}, AgentNames())
}
