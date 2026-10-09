package skill

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/skills/source"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestMergeSourceListEntries(t *testing.T) {
	entries := []listEntry{
		{name: "zebra", displayName: "Zebra", available: true},
		{name: "demo", displayName: "Demo", available: true, description: "Catalog description", version: "v1"},
	}
	statuses := []source.Status{
		{Source: "team", Name: "demo", Scope: "project", Client: "claude-code", Status: "current"},
		{Source: "team", Name: "demo", Scope: "project", Client: "atmos", Path: "canonical", Status: "current"},
		{Source: "team", Name: "demo", Scope: "project", Client: "gemini", Status: "missing"},
		{Source: "alpha", Status: "stale"},
	}
	result := mergeSourceListEntries(entries, statuses)
	require.Len(t, result, 3)
	require.Equal(t, "alpha", result[0].name)
	require.False(t, result[0].installed)
	require.Equal(t, "zebra", result[2].name)
	demo := result[1]
	require.Equal(t, "demo", demo.name)
	require.Equal(t, "Catalog description", demo.description)
	require.Equal(t, "Demo", demo.displayName)
	require.Empty(t, demo.version)
	require.Equal(t, "team", demo.source)
	require.Equal(t, "canonical", demo.sourceStatus.Path)
	require.Equal(t, "current; gemini: missing", demo.sourceStatus.Status)
	available, installed := countEntries(result)
	require.Equal(t, 1, available)
	require.Equal(t, 1, installed)
}

func TestSourceListKeepsDistinctScopes(t *testing.T) {
	statuses := []source.Status{
		{Source: "team", Name: "demo", Scope: "project", Client: "atmos", Path: "project-copy", Status: "stale"},
		{Source: "team", Name: "demo", Scope: "user", Client: "atmos", Path: "user-copy", Status: "current"},
	}
	result := mergeSourceListEntries(nil, statuses)
	require.Len(t, result, 2)
	require.Equal(t, statuses[0], *result[0].sourceStatus)
	require.True(t, result[0].installed)
	require.Equal(t, statuses[1], *result[1].sourceStatus)
}

func TestSourceListStatusErrorsPreserveCatalog(t *testing.T) {
	for _, failure := range []string{"unlocked-version", "pending-journal", "moved-project"} {
		t.Run(failure, func(t *testing.T) {
			project, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			home, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			config := &schema.AtmosConfiguration{BasePath: project}
			engine := &source.Engine{Config: config, Project: project, Home: home}
			stateDir := filepath.Join(project, ".atmos", "skills")
			switch failure {
			case "unlocked-version":
				config.AI.Skills = map[string]*schema.AISkillConfig{"team": {Source: "owner/repo", Ref: schema.SkillRef{Dependency: "skills"}}}
			case "pending-journal":
				require.NoError(t, os.MkdirAll(stateDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(stateDir, "transaction.json"), []byte("{}"), 0o600))
			case "moved-project":
				state := source.State{Version: 1, Records: []*source.Record{{Project: filepath.Join(project, "old"), Scope: "project"}}}
				raw, err := json.Marshal(state)
				require.NoError(t, err)
				require.NoError(t, os.MkdirAll(stateDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(stateDir, "installations.json"), raw, 0o600))
			}
			_, err = engine.Status(context.Background(), source.Options{})
			require.Error(t, err)
			var stderr bytes.Buffer
			ioCtx, err := iolib.NewContext(iolib.WithStreams(testStreams{input: &bytes.Buffer{}, output: &bytes.Buffer{}, error: &stderr}))
			require.NoError(t, err)
			ui.InitFormatter(ioCtx)
			t.Cleanup(ui.Reset)
			catalog := []listEntry{{name: "bundled", available: true}}
			require.Equal(t, catalog, appendSourceListEntries(context.Background(), catalog, engine))
			require.Contains(t, stderr.String(), "Declared skill sources unavailable")
		})
	}
}
