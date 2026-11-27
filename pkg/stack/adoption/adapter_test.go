package adoption

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

type fixtureAdapter struct {
	kind  string
	match Match
	err   error
}

func (a fixtureAdapter) Kind() string      { return a.kind }
func (a fixtureAdapter) Options() []string { return []string{"name"} }

func (a fixtureAdapter) Detect(Source) (Match, error)                       { return a.match, a.err }
func (a fixtureAdapter) Adopt(context.Context, *Registration, string) error { return a.err }

func TestAdapterPrecedence(t *testing.T) {
	dir := t.TempDir()
	native := fixtureAdapter{kind: "native", match: Match{Path: dir}}
	wrapper := fixtureAdapter{kind: "wrapper", match: Match{Path: filepath.Join(dir, "unit"), Wrapper: true}}
	for _, candidates := range []Adapters{{native, wrapper}, {wrapper, native}, {native, native, wrapper}} {
		adapter, path, err := candidates.Detect(dir)
		require.NoError(t, err)
		require.Equal(t, wrapper, adapter)
		require.Equal(t, wrapper.match.Path, path)
	}
	_, _, err := (Adapters{native, native}).Detect(dir)
	require.ErrorContains(t, err, "multiple tool configurations")
	_, _, err = (Adapters{wrapper, wrapper}).Detect(dir)
	require.ErrorContains(t, err, "multiple tool configurations")
	_, _, err = (Adapters{fixtureAdapter{}}).Detect(dir)
	require.ErrorIs(t, err, ErrNoMatch)
	probeFailure := errors.New("invalid marker")
	_, _, err = (Adapters{fixtureAdapter{err: probeFailure}}).Detect(dir)
	require.ErrorIs(t, err, probeFailure)
}

func TestInspectSource(t *testing.T) {
	dir := t.TempDir()
	for _, file := range []string{"b", "~backup", ".hidden", "a"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, file), nil, 0o600))
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "nested"), 0o700))
	source, err := inspectSource(dir)
	require.NoError(t, err)
	require.True(t, source.IsDir)
	require.Equal(t, []string{"a", "b"}, source.Files)
	source, err = inspectSource(filepath.Join(dir, "a"))
	require.NoError(t, err)
	require.False(t, source.IsDir)
	_, err = inspectSource(filepath.Join(dir, "missing"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLocalComponentRetryAndConflict(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dir := filepath.Join(root, "services", "hourly")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	r := Registration{Project: root, Dir: dir, Stack: "dev", Output: io.Discard}
	entry := map[string]any{"vars": map[string]any{"interval": 10}}
	source := LocalComponent{Kind: "scheduler", Name: "hourly", Dir: dir, NextCommand: "check", Entry: func(string, string) (map[string]any, error) { return entry, nil }}
	require.Error(t, r.WriteLocalComponent(nil))
	r.DryRun = true
	require.NoError(t, r.WriteLocalComponent(&source))
	require.NoFileExists(t, filepath.Join(root, "atmos.yaml"))
	r.DryRun = false
	require.NoError(t, r.WriteLocalComponent(&source))
	r.Config = &schema.AtmosConfiguration{}
	r.Config.Stacks.IncludedPaths = []string{"*.yaml"}
	r.Config.StacksBaseAbsolutePath = filepath.Join(root, "stacks")
	source.ConfiguredBase = "services"
	file := filepath.Join(root, "stacks", "dev.yaml")
	before, err := os.ReadFile(file)
	require.NoError(t, err)
	require.NoError(t, r.WriteLocalComponent(&source))
	entry["vars"] = map[string]any{"interval": 20}
	require.ErrorContains(t, r.WriteLocalComponent(&source), "different settings")
	after, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestUnsupportedSourceOptions(t *testing.T) {
	a := fixtureAdapter{}
	for _, r := range []Registration{{Workspace: "blue"}, {ShareState: true}, {ReplaceVarFiles: true}, {Explicit: "tofu"}, {AtlantisProject: "p"}, {Values: []string{"values.yaml"}}, {ReleaseName: "canary"}, {Namespace: "n"}, {VarFiles: []string{"x"}}, {Name: "bad/name"}} {
		require.Error(t, r.CheckOptions(a))
	}
	r := Registration{Name: "valid"}
	require.NoError(t, r.CheckOptions(a))
}
