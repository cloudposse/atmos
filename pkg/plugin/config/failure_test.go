package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestTransactionRejectsInvalidChangesBeforeWriting(t *testing.T) {
	tests := map[string][]Change{
		"duplicate":           {{Name: "atmos.yaml", After: []byte("a")}, {Name: "atmos.yaml", After: []byte("b")}},
		"escape":              {{Name: "../atmos.yaml", After: []byte("a")}},
		"empty output":        {{Name: "atmos.yaml"}},
		"special permissions": {{Name: "atmos.yaml", After: []byte("a"), Mode: 0o4000}},
	}
	for name, changes := range tests {
		t.Run(name, func(t *testing.T) {
			project := t.TempDir()
			require.ErrorIs(t, Mutate(t.Context(), project, func() ([]Change, error) { return changes, nil }), errUtils.ErrPluginInvalidConfig)
			require.NoFileExists(t, journalPath(project))
			require.NoFileExists(t, filepath.Join(project, "atmos.yaml"))
		})
	}
	project := t.TempDir()
	require.ErrorIs(t, Mutate(t.Context(), project, func() ([]Change, error) { return nil, os.ErrPermission }), os.ErrPermission)
	ctx, cancel := context.WithCancel(t.Context())
	require.ErrorIs(t, Mutate(ctx, project, func() ([]Change, error) { cancel(); return []Change{{Name: "atmos.yaml", After: []byte("new")}}, nil }), context.Canceled)
	require.NoFileExists(t, filepath.Join(project, "atmos.yaml"))
	require.NoFileExists(t, journalPath(project))
}

func TestTransactionRejectsMalformedJournalAndNonFiles(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Dir(journalPath(project)), 0o700))
	require.NoError(t, os.WriteFile(journalPath(project), []byte("invalid"), 0o600))
	// An unparseable journal is quarantined and the mutation proceeds (see transaction_recovery_test.go).
	require.NoError(t, Mutate(t.Context(), project, func() ([]Change, error) { return nil, nil }))
	require.NoFileExists(t, journalPath(project))
	require.NoError(t, os.Mkdir(filepath.Join(project, "atmos.yaml"), 0o700))
	require.Error(t, Mutate(t.Context(), project, func() ([]Change, error) { return []Change{{Name: "atmos.yaml", After: []byte("new")}}, nil }))
	_, err := replacementMode(project, "atmos.yaml")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, setChangeModes(project, []Change{{Name: "atmos.yaml"}}), errUtils.ErrPluginInvalidConfig)
	blocked := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocked, []byte("file"), 0o600))
	require.Error(t, Mutate(t.Context(), blocked, func() ([]Change, error) { return nil, nil }))
	require.ErrorIs(t, CheckTransaction(t.Context(), blocked), errUtils.ErrPluginInvalidConfig)
}
