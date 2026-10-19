package exec

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDescribeIncludeQuotedEvalDoesNotExecute(t *testing.T) {
	for _, edition := range []string{"", "2026-10-12", "2026-10-13"} {
		t.Run("edition="+edition, func(t *testing.T) {
			t.Setenv("ATMOS_EDITION", edition)
			t.Setenv("ATMOS_SETTINGS_EXPERIMENTAL", "silence")
			for _, tc := range []struct {
				name    string
				query   string
				want    any
				execute bool
			}{
				{"explicit opt in", ".foo | eval", map[string]any{"value": "EXECUTED"}, true},
				{"quoted eval", `.foo | "eval"`, "eval", false},
				{"quoted eval pipeline", `.foo | "eval" | length`, 4, false},
				{"data only", ".foo", map[string]any{"value": "echo EXECUTED > marker.txt; echo EXECUTED"}, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					writeManifestLoadFixture(t, "dev.yaml", fmt.Sprintf(`vars:
  stage: dev
components:
  terraform:
    app:
      vars:
        included: !include '../../fragment.yaml %s'
`, tc.query))
					// echo and redirection are interpreter builtins, so this works on Windows too.
					require.NoError(t, os.WriteFile("fragment.yaml", []byte("foo:\n  value: !exec 'echo EXECUTED > marker.txt; echo EXECUTED'\n"), 0o644))
					require.NoFileExists(t, "marker.txt")
					sections, err := describeWithDeadline(t, "app", "dev")
					require.NoError(t, err)
					vars, ok := sections["vars"].(map[string]any)
					require.True(t, ok)
					assert.Equal(t, tc.want, vars["included"])
					if tc.execute {
						content, err := os.ReadFile("marker.txt")
						require.NoError(t, err)
						assert.Equal(t, "EXECUTED\n", string(content))
					} else {
						assert.NoFileExists(t, "marker.txt")
					}
				})
			}
		})
	}
}
