package skill

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
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

func TestSourceListInstalledWithMissingCanonicalCopy(t *testing.T) {
	for _, tt := range []struct {
		name      string
		state     string
		source    string
		skill     string
		scope     string
		installed bool
	}{
		{name: "current client", state: "current", installed: true},
		{name: "stale client", state: "stale", installed: true},
		{name: "drifted client", state: "drifted", installed: true},
		{name: "obsolete client", state: "obsolete", installed: true},
		{name: "missing client", state: "missing"},
		{name: "different source", state: "current", source: "other"},
		{name: "different skill", state: "current", skill: "other"},
		{name: "different scope", state: "current", scope: "user"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			canonical := source.Status{Source: "team", Name: "demo", Scope: "project", Client: "atmos", Path: "canonical", Status: "missing"}
			client := canonical
			client.Client, client.Path, client.Status = "claude-code", "client-copy", tt.state
			if tt.source != "" {
				client.Source = tt.source
			}
			if tt.skill != "" {
				client.Name = tt.skill
			}
			if tt.scope != "" {
				client.Scope = tt.scope
			}
			entries := mergeSourceListEntries(nil, []source.Status{canonical, client})
			require.Len(t, entries, 1)
			require.Equal(t, tt.installed, entries[0].installed)
			require.Contains(t, entries[0].sourceStatus.Status, "missing")
			require.Equal(t, tt.installed, len(filterInstalled(entries)) == 1)
		})
	}
}

func TestSourceListPreservesUnrelatedInstalledCatalogEntry(t *testing.T) {
	root := t.TempDir()
	legacy := &marketplace.InstalledSkill{Name: "demo", Source: "legacy/repo", Version: "v1", Path: filepath.Join(root, "legacy")}
	catalog := listEntry{name: "demo", displayName: "Demo", available: true, installed: true, skill: legacy, version: "v1", source: legacy.Source, updateAvailable: true}
	for _, tt := range []struct {
		name string
		path string
	}{
		{name: "different installation", path: filepath.Join(root, "project")},
		{name: "unresolved declaration"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			status := source.Status{Source: "team", Name: "demo", Scope: "project", Client: "atmos", Path: tt.path, Status: "missing"}
			entries := mergeSourceListEntries([]listEntry{catalog}, []source.Status{status})
			require.Len(t, entries, 2)
			require.Equal(t, catalog, entries[0], "retain the original installation details")
			require.Equal(t, &status, entries[1].sourceStatus)
			require.Equal(t, []listEntry{catalog}, filterInstalled(entries))
		})
	}

	status := source.Status{Source: "team", Name: "demo", Scope: "user", Client: "atmos", Path: legacy.Path, Status: "current"}
	entries := mergeSourceListEntries([]listEntry{catalog}, []source.Status{status})
	require.Len(t, entries, 1, "the same installation must not appear twice")
	require.Equal(t, &status, entries[0].sourceStatus)
	require.Len(t, filterInstalled(entries), 1)
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
