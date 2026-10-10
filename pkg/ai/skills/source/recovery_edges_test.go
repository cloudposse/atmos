package source

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecoveryRestoresManualTreeAndRemovesNewInstall(t *testing.T) {
	e, _ := fixture(t)
	manual := filepath.Join(realTemp(t), "manual")
	old := filepath.Join(manual, "demo")
	fresh := filepath.Join(e.stateDir(scopeProject), "content", "new")
	oldStage := filepath.Join(manual, ".skills-stage-old")
	newStage := filepath.Join(filepath.Dir(fresh), ".skills-stage-new")
	write(t, filepath.Join(oldStage+".backup", "SKILL.md"), "original bytes")
	write(t, filepath.Join(old, "SKILL.md"), "partial replacement")
	write(t, filepath.Join(fresh, "SKILL.md"), "new partial installation")
	j := journal{Project: e.Project, ManualRoots: []string{manual}, Entries: []journalEntry{
		{Target: old, Stage: oldStage, Backup: oldStage + ".backup", HadOriginal: true},
		{Target: fresh, Stage: newStage, Backup: newStage + ".backup"},
	}}
	require.NoError(t, e.saveJournal(j))
	require.NoError(t, e.Recover())
	raw, err := os.ReadFile(filepath.Join(old, "SKILL.md"))
	require.NoError(t, err)
	require.Equal(t, "original bytes", string(raw))
	require.NoDirExists(t, fresh)
	require.NoDirExists(t, oldStage+".backup")
	require.NoError(t, e.pending())
	require.NoError(t, e.Recover())
}

func TestRecoveryRejectsUntrustedJournalPathsWithoutMutation(t *testing.T) {
	for _, kind := range []string{"stage parent", "backup mismatch", "outside roots", "manual traversal", "manual root relative", "manual root filesystem root"} {
		t.Run(kind, func(t *testing.T) {
			e, _ := fixture(t)
			outside := realTemp(t)
			target := filepath.Join(e.stateDir(scopeProject), "content", "demo")
			roots := []string{}
			switch kind {
			case "outside roots":
				target = filepath.Join(outside, "demo")
			case "manual traversal":
				target = filepath.Join(outside, "nested", "demo")
				roots = []string{outside}
			case "manual root relative":
				target = filepath.Join(outside, "demo")
				roots = []string{"relative"}
			case "manual root filesystem root":
				target = filepath.Join(outside, "demo")
				roots = []string{filepath.VolumeName(outside) + string(filepath.Separator)}
			}
			stage := filepath.Join(filepath.Dir(target), ".skills-stage-untrusted")
			if kind == "stage parent" {
				stage = filepath.Join(outside, ".skills-stage-untrusted")
			}
			entry := journalEntry{Target: target, Stage: stage, Backup: stage + ".backup", HadOriginal: true}
			if kind == "backup mismatch" {
				entry.Backup = stage + ".wrong"
			}
			write(t, filepath.Join(target, "keep.txt"), "owned by user")
			write(t, filepath.Join(stage, "new.txt"), "staged data")
			require.NoError(t, e.saveJournal(journal{Project: e.Project, ManualRoots: roots, Entries: []journalEntry{entry}}))
			require.ErrorIs(t, e.Recover(), ErrInvalid)
			raw, err := os.ReadFile(filepath.Join(target, "keep.txt"))
			require.NoError(t, err)
			require.Equal(t, "owned by user", string(raw))
			require.FileExists(t, filepath.Join(stage, "new.txt"))
			require.FileExists(t, e.journals()[0])
		})
	}
}

func TestJournalWriteFailureCleansStagesAndPreservesOriginal(t *testing.T) {
	e, _ := fixture(t)
	target := filepath.Join(e.Project, "document")
	write(t, target, "original")
	require.NoError(t, os.MkdirAll(e.journals()[0], 0o755))
	err := e.apply([]operation{{Target: target, Data: []byte("replacement")}})
	require.Error(t, err)
	raw, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "original", string(raw))
	stages, err := filepath.Glob(filepath.Join(e.Project, ".skills-stage-*"))
	require.NoError(t, err)
	require.Empty(t, stages)
	temporary, err := filepath.Glob(filepath.Join(e.stateDir(scopeProject), ".skills-write-*"))
	require.NoError(t, err)
	require.Empty(t, temporary)
}

func TestRecoveryRejectsCorruptedJournalAndLockedEngine(t *testing.T) {
	e, _ := fixture(t)
	for _, directory := range []bool{false, true} {
		if directory {
			require.NoError(t, os.Remove(e.journals()[0]))
			require.NoError(t, os.Mkdir(e.journals()[0], 0o755))
		} else {
			write(t, e.journals()[0], "{broken")
		}
		require.Error(t, e.Recover())
	}
	require.NoError(t, os.Remove(e.journals()[0]))
	unlock, err := e.acquire(false)
	require.NoError(t, err)
	defer unlock()
	require.ErrorIs(t, e.Recover(), ErrRecovery)
}

func TestInvalidStateBlocksReadOnlyQueries(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "invalid json", true: "directory"}[directory], func(t *testing.T) {
			e, _ := fixture(t)
			path := filepath.Join(e.stateDir(scopeProject), "installations.json")
			if directory {
				require.NoError(t, os.MkdirAll(path, 0o755))
			} else {
				write(t, path, "not json")
			}
			_, err := e.InstalledSources("", "")
			require.Error(t, err)
			_, err = e.ResolveInstalled("demo", "")
			require.Error(t, err)
			_, err = e.InstalledConfig("")
			require.Error(t, err)
			_, err = e.AdHocConfig("")
			require.Error(t, err)
			e.Config.AI.Skills = nil
			_, err = e.Status(context.Background(), Options{})
			require.Error(t, err)
		})
	}
}

func TestStateRejectsForgedAndDuplicateDestinations(t *testing.T) {
	for _, kind := range []string{"duplicate", "unknown client", "invalid name", "manual relative", "manual root", "wrong scope", "wrong path"} {
		t.Run(kind, func(t *testing.T) {
			e, _ := fixture(t)
			target, err := e.destination(scopeProject, canonicalClient, "demo")
			require.NoError(t, err)
			r := Record{Name: "demo", Source: "test", Project: e.Project, Scope: scopeProject, Client: canonicalClient, Path: target}
			switch kind {
			case "unknown client":
				r.Client = "unsupported"
			case "invalid name":
				r.Name = "../escape"
			case "manual relative":
				r.Client = "manual"
				r.ManualRoot = "relative"
			case "manual root":
				r.Client = "manual"
				r.ManualRoot = filepath.VolumeName(e.Project) + string(filepath.Separator)
			case "wrong scope":
				r.Scope = scopeUser
			case "wrong path":
				r.Path = filepath.Join(e.Project, "unowned")
			}
			state := State{Version: 1, Records: []*Record{&r}}
			if kind == "duplicate" {
				state.Records = append(state.Records, &r)
			}
			raw, err := json.Marshal(state)
			require.NoError(t, err)
			path := filepath.Join(e.stateDir(scopeProject), "installations.json")
			write(t, path, string(raw))
			_, err = e.Run(context.Background(), Options{Check: true})
			require.ErrorIs(t, err, ErrInvalid)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, raw, after)
		})
	}
}
