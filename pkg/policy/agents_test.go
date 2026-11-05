package policy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectAgents(t *testing.T) {
	for _, tc := range []struct {
		name, claude, codex string
		want                []string
	}{
		{"human", "", "", []string{}},
		{"claude", "1", "", []string{"claude-code"}},
		{"codex", "", "thread-id-not-exposed", []string{"codex"}},
		{"both", "1", "thread", []string{"claude-code", "codex"}},
		{"claude disabled", "0", "", []string{}},
		{"claude noncanonical", "true", "", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CLAUDECODE", tc.claude)
			t.Setenv("CODEX_THREAD_ID", tc.codex)
			require.Equal(t, tc.want, DetectAgents())
		})
	}
}
