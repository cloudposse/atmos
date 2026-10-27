package hooks

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// repositoryCheck executes the same Starlark functions used by the hook suite.
func repositoryCheck(t *testing.T, name, dir string) *schema.GitConfig {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "checks", "checks.star"))
	require.NoError(t, err)
	function := strings.ReplaceAll(name, "-", "_")
	task := schema.Task{Name: name, Type: "script", Interpreter: "starlark", WorkingDirectory: dir, Script: string(content) + "\n" + function + "()\n"}
	return &schema.GitConfig{Hooks: map[string]schema.GitHookEntry{"pre-commit": {Steps: schema.Tasks{task}}}}
}

func TestRepositoryFileSizeCheck(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := repositoryCheck(t, "check-agent-file-sizes", dir)
	var stdout bytes.Buffer
	run := func() error { return Run(cfg, "pre-commit", nil, WithOutputWriters(&stdout, &stdout)) }
	require.NoError(t, run(), "missing optional files are allowed")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(strings.Repeat("é", 20000)), 0o600))
	require.NoError(t, run(), "exact byte limit is accepted")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(strings.Repeat("é", 20001)), 0o600))
	err := run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Agent instruction files exceed size limits")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), nil, 0o600))
	agents := filepath.Join(dir, ".claude", "agents")
	require.NoError(t, os.MkdirAll(agents, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(agents, "review.md"), []byte(strings.Repeat("x", 25001)), 0o600))
	require.Error(t, run(), "agent files have their own limit")
}

func TestRepositorySymlinkCheck(t *testing.T) {
	t.Parallel()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is unavailable")
	}
	dir := t.TempDir()
	link := filepath.Join(dir, "link with spaces")
	if err = os.Symlink("target", link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	for _, args := range [][]string{{"init"}, {"add", "--", "link with spaces"}} {
		cmd := exec.Command(git, args...)
		cmd.Dir = dir
		out, runErr := cmd.CombinedOutput()
		require.NoError(t, runErr, string(out))
	}
	cfg := repositoryCheck(t, "check-symlinks", dir)
	var stdout bytes.Buffer
	run := func() error { return Run(cfg, "pre-commit", nil, WithOutputWriters(&stdout, &stdout)) }
	require.Error(t, run(), "dangling tracked link must fail")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target"), []byte("ok"), 0o600))
	require.NoError(t, run())
	require.NoError(t, os.Symlink("missing", filepath.Join(dir, "untracked")))
	require.NoError(t, run(), "untracked broken links are excluded")
	require.NoError(t, os.Remove(link))
	require.Error(t, run(), "tracked path missing in checkout must fail")
}
