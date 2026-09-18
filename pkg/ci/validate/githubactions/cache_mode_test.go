package githubactions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rhysd/actionlint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheModeDiagnosticsUnavailableSource(t *testing.T) {
	t.Parallel()
	for _, contents := range []string{"", "[", "jobs: []", "jobs:\n  build: invalid", "cache-mode: read"} {
		t.Run(contents, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := filepath.Join(root, "workflow.yml")
			if contents != "" {
				require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
			}
			diagnostic := &actionlint.Error{Filepath: path, Line: 50, Column: 1, Kind: "syntax-check", Message: `unexpected key "cache-mode"`}
			result, changed := cacheModeDiagnostics(root, []*actionlint.Error{diagnostic})
			assert.False(t, changed)
			assert.Equal(t, []*actionlint.Error{diagnostic}, result)
		})
	}
}

func TestCacheModeAlias(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "workflow.yml")
	require.NoError(t, os.WriteFile(path, []byte("cache-mode: &mode read\njobs:\n  build:\n    cache-mode: *mode\n"), 0o600))
	diagnostic := &actionlint.Error{Filepath: path, Line: 4, Column: 5, Kind: "syntax-check", Message: `unexpected key "cache-mode"`}
	result, changed := cacheModeDiagnostics(root, []*actionlint.Error{diagnostic})
	assert.True(t, changed)
	assert.Empty(t, result)
}

func TestSelfReferenceNormalizeIgnoresOtherReferences(t *testing.T) {
	t.Parallel()
	rule := &selfReferenceRule{RuleBase: actionlint.NewRuleBase("action", "test")}
	rule.normalize(nil)
	uses := &actionlint.String{Value: "actions/checkout@v6"}
	rule.normalize(uses)
	assert.Equal(t, "actions/checkout@v6", uses.Value)
	assert.Empty(t, rule.Errs())
}
