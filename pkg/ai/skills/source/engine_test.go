package source

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

func realTemp(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

func write(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func fixture(t *testing.T) (*Engine, string) {
	t.Helper()
	base, home, repo := realTemp(t), realTemp(t), realTemp(t)
	write(t, filepath.Join(repo, "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: Demo skill\n---\n# Demo\nOriginal\n")
	config := &schema.AtmosConfiguration{BasePath: base, AI: schema.AISettings{Skills: map[string]*schema.AISkillConfig{"test": {Source: repo, Clients: []string{"claude-code", "gemini"}}}}}
	e := &Engine{Config: config, Project: base, Home: home, Fetcher: &GitFetcher{Config: config}, Rename: os.Rename}
	return e, repo
}

//nolint:gocritic // The test helper mirrors the public value-options API.
func run(t *testing.T, e *Engine, o Options) []Status {
	t.Helper()
	statuses, err := e.Run(context.Background(), o)
	require.NoError(t, err)
	return statuses
}

func TestSyncLocalFrozenUpdateAndDeletedFiles(t *testing.T) {
	e, repo := fixture(t)
	script := filepath.Join(repo, "skills", "demo", "old.sh")
	write(t, script, "old")
	run(t, e, Options{})
	lock, err := os.ReadFile(e.lockPath())
	require.NoError(t, err)
	run(t, e, Options{Frozen: true})
	again, err := os.ReadFile(e.lockPath())
	require.NoError(t, err)
	require.Equal(t, lock, again)
	run(t, e, Options{Check: true})
	require.NoError(t, os.Remove(script))
	_, err = e.Run(context.Background(), Options{})
	require.ErrorIs(t, err, ErrDrift)
	run(t, e, Options{Update: true})
	for _, client := range []string{"atmos", "claude-code", "gemini"} {
		p, err := e.destination("project", client, "demo")
		require.NoError(t, err)
		_, err = os.Stat(filepath.Join(p, "old.sh"))
		require.True(t, os.IsNotExist(err))
	}
}

func TestReadOnlyFreshHome(t *testing.T) {
	for _, o := range []Options{{DryRun: true}, {Check: true}} {
		t.Run(map[bool]string{true: "check", false: "dry-run"}[o.Check], func(t *testing.T) {
			e, _ := fixture(t)
			_, err := e.Run(context.Background(), o)
			if o.Check {
				require.ErrorIs(t, err, ErrDrift)
			} else {
				require.NoError(t, err)
			}
			for _, root := range []string{e.Project, e.Home} {
				entries, err := os.ReadDir(root)
				require.NoError(t, err)
				require.Empty(t, entries)
			}
		})
	}
}

func TestOwnershipAndModifiedCopies(t *testing.T) {
	e, _ := fixture(t)
	path, _ := e.destination("project", "gemini", "demo")
	write(t, filepath.Join(path, "custom.txt"), "mine")
	_, err := e.Run(context.Background(), Options{Force: true})
	require.ErrorIs(t, err, ErrOwnership)
	require.NoError(t, os.RemoveAll(path))
	run(t, e, Options{})
	write(t, filepath.Join(path, "custom.txt"), "mine")
	_, err = e.Run(context.Background(), Options{})
	require.ErrorIs(t, err, ErrDrift)
	run(t, e, Options{Force: true})
	_, err = os.Stat(filepath.Join(path, "custom.txt"))
	require.True(t, os.IsNotExist(err))
}

func TestPruneExplicitAndClientLimitedUninstall(t *testing.T) {
	e, _ := fixture(t)
	run(t, e, Options{})
	run(t, e, Options{Uninstall: true, Name: "demo", Clients: []string{"claude-code"}})
	claude, _ := e.destination("project", "claude-code", "demo")
	_, err := os.Stat(claude)
	require.True(t, os.IsNotExist(err))
	canonical, _ := e.destination("project", "atmos", "demo")
	require.DirExists(t, canonical)
	delete(e.Config.AI.Skills, "test")
	statuses := run(t, e, Options{})
	require.Len(t, statuses, 2)
	require.DirExists(t, canonical)
	_, err = e.Run(context.Background(), Options{Check: true})
	require.ErrorIs(t, err, ErrDrift)
	run(t, e, Options{Prune: true})
	_, err = os.Stat(canonical)
	require.True(t, os.IsNotExist(err))
}

func TestRollbackAfterSecondDestinationFailure(t *testing.T) {
	e, repo := fixture(t)
	run(t, e, Options{})
	before, err := os.ReadFile(e.lockPath())
	require.NoError(t, err)
	write(t, filepath.Join(repo, "skills", "demo", "new.txt"), "new")
	calls := 0
	e.Rename = func(a, b string) error {
		calls++
		if calls == 4 {
			return errors.New("injected failure")
		}
		return os.Rename(a, b)
	}
	_, err = e.Run(context.Background(), Options{Update: true})
	require.ErrorContains(t, err, "injected failure")
	after, err := os.ReadFile(e.lockPath())
	require.NoError(t, err)
	require.Equal(t, before, after)
	for _, client := range []string{"atmos", "claude-code", "gemini"} {
		p, _ := e.destination("project", client, "demo")
		_, err = os.Stat(filepath.Join(p, "new.txt"))
		require.True(t, os.IsNotExist(err))
	}
	require.NoError(t, e.pending())
}

func TestProjectIsolationAndUserOwnership(t *testing.T) {
	first, _ := fixture(t)
	second, _ := fixture(t)
	second.Home = first.Home
	run(t, first, Options{})
	run(t, second, Options{})
	run(t, first, Options{Scope: "user"})
	_, err := second.Run(context.Background(), Options{Scope: "user", Force: true})
	require.ErrorIs(t, err, ErrOwnership)
}

func TestVersionTracks(t *testing.T) {
	e, _ := fixture(t)
	e.Config.AI.Skills["test"].Ref = schema.SkillRef{Dependency: "skills"}
	_, err := e.Run(context.Background(), Options{})
	require.Error(t, err)
	write(t, filepath.Join(e.Project, "versions.lock.yaml"), "version: 1\ntracks:\n  prod:\n    skills:\n      version: v1\n  dev:\n    skills:\n      version: v2\n")
	run(t, e, Options{Track: "prod"})
	run(t, e, Options{Track: "dev"})
	var lock Lock
	require.NoError(t, readYAML(e.lockPath(), &lock))
	require.Len(t, lock.Tracks, 2)
	require.Equal(t, "v1", lock.Tracks["prod"]["test"].Version)
	states, err := e.loadStates()
	require.NoError(t, err)
	require.Equal(t, "dev", states["project"].Records[0].Track)
}

func TestContainment(t *testing.T) {
	e, repo := fixture(t)
	for _, path := range []string{"../escape", "/absolute", "foo/../../escape", "foo\\bar"} {
		_, err := within(repo, path)
		require.Error(t, err)
	}
	require.NoError(t, os.Symlink(repo, filepath.Join(repo, "skills", "demo", "escape")))
	_, err := e.Run(context.Background(), Options{})
	require.ErrorIs(t, err, ErrInvalid)
}

func TestSkillRefYAMLRoundTrip(t *testing.T) {
	var d schema.AISkillConfig
	require.NoError(t, yaml.Unmarshal([]byte("source: example/repo\nref: !version tool\n"), &d))
	require.Equal(t, "tool", d.Ref.Dependency)
	raw, err := yaml.Marshal(d)
	require.NoError(t, err)
	require.Contains(t, string(raw), "!version tool")
	literal := schema.AISkillConfig{Ref: schema.SkillRef{Literal: "!version tool"}}
	raw, err = yaml.Marshal(literal)
	require.NoError(t, err)
	var out schema.AISkillConfig
	require.NoError(t, yaml.Unmarshal(raw, &out))
	require.Equal(t, literal.Ref, out.Ref)
}

func TestStateContainsDestinations(t *testing.T) {
	e, _ := fixture(t)
	run(t, e, Options{})
	raw, err := os.ReadFile(filepath.Join(e.stateDir("project"), "installations.json"))
	require.NoError(t, err)
	var state State
	require.NoError(t, json.Unmarshal(raw, &state))
	require.Len(t, state.Records, 3)
}
