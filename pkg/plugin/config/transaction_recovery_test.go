package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func writeJournal(t *testing.T, project string, raw []byte) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(journalPath(project)), 0o700))
	require.NoError(t, os.WriteFile(journalPath(project), raw, 0o600))
}

func journalQuarantines(t *testing.T, project string) []string {
	t.Helper()
	matches, err := filepath.Glob(journalPath(project) + QuarantineSuffix + "*")
	require.NoError(t, err)
	return matches
}

func TestMutateQuarantinesUnparseableJournalAndProceeds(t *testing.T) {
	for name, garbage := range map[string]string{"garbage": "{not json", "empty": "", "wrong shape": `{"name":"atmos.yaml"}`} {
		t.Run(name, func(t *testing.T) {
			project := t.TempDir()
			writeJournal(t, project, []byte(garbage))
			require.ErrorIs(t, CheckTransaction(project), errUtils.ErrPluginTransactionPending)

			prepared := false
			require.NoError(t, Mutate(t.Context(), project, func() ([]Change, error) {
				prepared = true
				return []Change{{Name: "atmos.yaml", After: []byte("new")}}, nil
			}))
			require.True(t, prepared, "the mutation proceeds after the journal is moved aside")
			actual, err := os.ReadFile(filepath.Join(project, "atmos.yaml"))
			require.NoError(t, err)
			require.Equal(t, "new", string(actual))
			require.NoFileExists(t, journalPath(project))
			moved := journalQuarantines(t, project)
			require.Len(t, moved, 1, "the unparseable journal is kept for inspection")
			kept, err := os.ReadFile(moved[0])
			require.NoError(t, err)
			require.Equal(t, garbage, string(kept))
			require.NoError(t, CheckTransaction(project), "ordinary commands work again")
		})
	}
}

func TestMutatePreservesConflictingJournalWithPreciseHint(t *testing.T) {
	project := t.TempDir()
	raw, err := json.Marshal([]Change{{Name: "atmos.yaml", Before: []byte("before"), After: []byte("after")}})
	require.NoError(t, err)
	writeJournal(t, project, raw)
	require.NoError(t, os.WriteFile(filepath.Join(project, "atmos.yaml"), []byte("external edit"), 0o644))

	err = Mutate(t.Context(), project, func() ([]Change, error) { t.Fatal("must not prepare over a conflict"); return nil, nil })
	require.ErrorIs(t, err, errUtils.ErrPluginTransactionPending)
	hints := strings.Join(errUtils.AllHints(err), "\n")
	require.Contains(t, hints, journalPath(project), "the hint names the journal")
	require.Contains(t, hints, "atmos.yaml", "the hint names the conflicting file")
	require.Contains(t, hints, "remove", "the hint says how to resolve the conflict")
	require.NotContains(t, err.Error(), "run atmos plugin install to recover", "no circular advice")
	require.Equal(t, string(raw), readFile(t, journalPath(project)), "a parseable journal is never auto-discarded")
	require.Empty(t, journalQuarantines(t, project))
	require.Equal(t, "external edit", readFile(t, filepath.Join(project, "atmos.yaml")))
}

func TestMutateRecoversCleanJournalWithoutQuarantine(t *testing.T) {
	project := t.TempDir()
	raw, err := json.Marshal([]Change{{Name: "atmos.yaml", Before: []byte("before"), After: []byte("after")}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(project, "atmos.yaml"), []byte("before"), 0o644))
	writeJournal(t, project, raw)

	require.NoError(t, Mutate(t.Context(), project, func() ([]Change, error) { return nil, nil }))
	require.Equal(t, "after", readFile(t, filepath.Join(project, "atmos.yaml")))
	require.NoFileExists(t, journalPath(project))
	require.Empty(t, journalQuarantines(t, project), "a valid journal is recovered, not quarantined")
}

func TestCheckTransactionHintsNameTheJournalAndTheRepair(t *testing.T) {
	require.NoError(t, CheckTransaction(t.TempDir()))

	garbage := t.TempDir()
	writeJournal(t, garbage, []byte("{not json"))
	err := CheckTransaction(garbage)
	require.ErrorIs(t, err, errUtils.ErrPluginTransactionPending)
	hints := strings.Join(errUtils.AllHints(err), "\n")
	require.Contains(t, hints, journalPath(garbage))
	require.Contains(t, hints, "atmos plugin install")
	require.Contains(t, hints, "unparseable")

	conflict := t.TempDir()
	raw, marshalErr := json.Marshal([]Change{{Name: "atmos.yaml", Before: []byte("before"), After: []byte("after")}})
	require.NoError(t, marshalErr)
	writeJournal(t, conflict, raw)
	require.NoError(t, os.WriteFile(filepath.Join(conflict, "atmos.yaml"), []byte("external edit"), 0o644))
	err = CheckTransaction(conflict)
	require.ErrorIs(t, err, errUtils.ErrPluginTransactionPending)
	hints = strings.Join(errUtils.AllHints(err), "\n")
	require.Contains(t, hints, journalPath(conflict))
	require.Contains(t, hints, "atmos.yaml")
	require.Contains(t, hints, "only an unparseable journal")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(raw)
}

func TestCheckTransactionPreservesReadFailure(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.MkdirAll(journalPath(project), 0o700))

	err := CheckTransaction(project)
	require.ErrorIs(t, err, errUtils.ErrPluginTransactionPending)
	hints := strings.Join(errUtils.AllHints(err), "\n")
	require.Contains(t, hints, journalPath(project))
	require.NotContains(t, hints, "not valid JSON")
	require.NotContains(t, hints, "atmos plugin install")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	var readErr *os.PathError
	require.ErrorAs(t, err, &readErr)
	require.Empty(t, journalQuarantines(t, project))
	require.DirExists(t, journalPath(project))
}
