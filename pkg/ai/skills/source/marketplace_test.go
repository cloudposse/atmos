package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeFetcher struct {
	heads map[string]string
	trees map[string]string
	calls []Repository
}

func (f *fakeFetcher) Fetch(_ context.Context, r Repository, dest string) (Repository, error) {
	f.calls = append(f.calls, r)
	if r.Commit == "" {
		r.Commit = f.heads[r.Source]
	}
	err := copyTree(f.trees[r.Source+"@"+r.Commit], dest)
	return r, err
}

func TestMarketplacePinsExternalPlugins(t *testing.T) {
	e, _ := fixture(t)
	market, one, two := realTemp(t), realTemp(t), realTemp(t)
	write(t, filepath.Join(market, ".claude-plugin", "marketplace.json"), `{"plugins":[{"name":"external","source":{"source":"github","repo":"org/plugin","ref":"main"}}]}`)
	for _, root := range []string{one, two} {
		write(t, filepath.Join(root, "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: Example\n---\n# Demo\n"+root)
	}
	a, b, c := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	fetch := &fakeFetcher{heads: map[string]string{"org/market": a, "org/plugin": b}, trees: map[string]string{"org/market@" + a: market, "org/plugin@" + b: one, "org/plugin@" + c: two}}
	e.Fetcher = fetch
	e.Config.AI.Skills["test"].Source = "org/market"
	run(t, e, Options{})
	fetch.heads["org/plugin"] = c
	run(t, e, Options{Frozen: true})
	var lock Lock
	require.NoError(t, readYAML(e.lockPath(), &lock))
	require.Equal(t, b, lock.Tracks["default"]["test"].Repositories[1].Commit)
	run(t, e, Options{Update: true})
	require.NoError(t, readYAML(e.lockPath(), &lock))
	require.Equal(t, c, lock.Tracks["default"]["test"].Repositories[1].Commit)
	require.Equal(t, a, lock.Tracks["default"]["test"].Repositories[0].Commit)
}

func TestMarketplaceRelativeAndSelections(t *testing.T) {
	e, repo := fixture(t)
	write(t, filepath.Join(repo, ".claude-plugin", "marketplace.json"), `{"plugins":[{"name":"local","source":"./plugin"}]}`)
	write(t, filepath.Join(repo, "plugin", "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: Example\n---\n# Demo\nBody")
	run(t, e, Options{})
	e.Config.AI.Skills["test"].Plugins = []string{"missing"}
	_, err := e.Run(context.Background(), Options{})
	require.ErrorContains(t, err, "not found")
	e.Config.AI.Skills["test"].Plugins = []string{"local"}
	e.Config.AI.Skills["test"].Exclude = []string{"demo"}
	statuses := run(t, e, Options{})
	require.Equal(t, "obsolete", statuses[0].Status)
	run(t, e, Options{Prune: true})
}

func TestRollbackRecoveryAndSerialization(t *testing.T) {
	e, repo := fixture(t)
	run(t, e, Options{})
	write(t, filepath.Join(repo, "skills", "demo", "new.txt"), "changed")
	calls := 0
	e.Rename = func(a, b string) error {
		calls++
		if calls >= 4 {
			return os.ErrPermission
		}
		return os.Rename(a, b)
	}
	_, err := e.Run(context.Background(), Options{Update: true})
	require.ErrorIs(t, err, ErrRecovery)
	_, err = e.Run(context.Background(), Options{})
	require.ErrorIs(t, err, ErrRecovery)
	e.Rename = os.Rename
	require.NoError(t, e.Recover())
	require.NoError(t, e.pending())
	unlock, err := e.acquire(false)
	require.NoError(t, err)
	defer unlock()
	_, err = e.Run(context.Background(), Options{})
	require.ErrorIs(t, err, ErrRecovery)
}

func TestMetadataWriteFailureRollsBackTrees(t *testing.T) {
	e, repo := fixture(t)
	run(t, e, Options{})
	write(t, filepath.Join(repo, "skills", "demo", "new.txt"), "changed")
	failed := false
	e.Rename = func(a, b string) error {
		if b == e.lockPath() && !failed {
			failed = true
			return os.ErrPermission
		}
		return os.Rename(a, b)
	}
	_, err := e.Run(context.Background(), Options{Update: true})
	require.ErrorIs(t, err, os.ErrPermission)
	target, _ := e.destination("project", "atmos", "demo")
	_, err = os.Stat(filepath.Join(target, "new.txt"))
	require.True(t, os.IsNotExist(err))
}

func TestManagedMarketplaceKeepsPluginPinsUntilVersionLockChanges(t *testing.T) {
	e, _ := fixture(t)
	market, pluginV1, pluginV2 := realTemp(t), realTemp(t), realTemp(t)
	write(t, filepath.Join(market, ".claude-plugin", "marketplace.json"), `{"plugins":[{"name":"external","source":{"source":"git-subdir","url":"org/plugin","path":"packages/plugin","ref":"main"}}]}`)
	for _, root := range []string{pluginV1, pluginV2} {
		write(t, filepath.Join(root, "packages", "plugin", "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: Example\n---\n# Demo\n"+root)
	}
	a, b, c := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	fetch := &fakeFetcher{heads: map[string]string{"org/market": a, "org/plugin": b}, trees: map[string]string{"org/market@" + a: market, "org/plugin@" + b: pluginV1, "org/plugin@" + c: pluginV2}}
	e.Fetcher = fetch
	e.Config.AI.Skills["test"].Source = "org/market"
	e.Config.AI.Skills["test"].Ref.Dependency = "skills"
	e.Config.Version.Track = "prod"
	e.Config.Version.LockFile = "locks/custom.yaml"
	lockPath := filepath.Join(e.Project, e.Config.Version.LockFile)
	write(t, lockPath, "version: 1\ntracks:\n  prod:\n    skills:\n      version: v1\n")
	run(t, e, Options{})
	fetch.heads["org/plugin"] = c
	run(t, e, Options{Update: true})
	var lock Lock
	require.NoError(t, readYAML(e.lockPath(), &lock))
	require.Equal(t, b, lock.Tracks["prod"]["test"].Repositories[1].Commit)
	write(t, lockPath, "version: 1\ntracks:\n  prod:\n    skills:\n      version: v2\n")
	_, err := e.Run(context.Background(), Options{Frozen: true})
	require.ErrorIs(t, err, ErrDrift)
	run(t, e, Options{})
	require.NoError(t, readYAML(e.lockPath(), &lock))
	require.Equal(t, c, lock.Tracks["prod"]["test"].Repositories[1].Commit)
	require.Equal(t, "v2", lock.Tracks["prod"]["test"].Version)
}
