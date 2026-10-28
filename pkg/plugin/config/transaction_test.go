package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
)

func TestTransactionRecoversPartialCommitAndProtectsExternalEdits(t *testing.T) {
	project := t.TempDir()
	changes := []Change{
		{Name: "atmos.yaml", Before: []byte("before"), After: []byte("after")},
		{Name: ".atmos.plugins.lock.yaml", Before: nil, After: []byte("locked")},
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(journalPath(project)), 0o700))
	raw, err := json.Marshal(changes)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(journalPath(project), raw, 0o600))
	// Simulate a crash after the first rename and before the lock replacement.
	require.NoError(t, os.WriteFile(filepath.Join(project, "atmos.yaml"), []byte("after"), 0o644))
	require.ErrorIs(t, CheckTransaction(t.Context(), project), errUtils.ErrPluginTransactionPending)
	require.NoError(t, Mutate(t.Context(), project, func() ([]Change, error) { return nil, nil }))
	locked, err := os.ReadFile(filepath.Join(project, ".atmos.plugins.lock.yaml"))
	require.NoError(t, err)
	require.Equal(t, "locked", string(locked))
	require.NoError(t, CheckTransaction(t.Context(), project))
	require.NoError(t, os.WriteFile(journalPath(project), raw, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(project, "atmos.yaml"), []byte("external edit"), 0o644))
	require.ErrorIs(t, Mutate(t.Context(), project, func() ([]Change, error) { return nil, nil }), errUtils.ErrPluginTransactionPending)
	actual, err := os.ReadFile(filepath.Join(project, "atmos.yaml"))
	require.NoError(t, err)
	require.Equal(t, "external edit", string(actual))
	require.FileExists(t, journalPath(project), "recovery copies must survive a conflict")
}

func TestTransactionPreservesPrivateConfigurationPermissions(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, "atmos.yaml")
	require.NoError(t, os.WriteFile(path, []byte("before"), 0o600))
	original, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, Mutate(t.Context(), project, func() ([]Change, error) {
		return []Change{{Name: "atmos.yaml", Before: []byte("before"), After: []byte("after")}}, nil
	}))
	info, err := os.Stat(path)
	require.NoError(t, err)
	// Compare the permissions the platform actually exposes: Windows does not implement POSIX mode bits.
	require.Equal(t, original.Mode().Perm(), info.Mode().Perm())
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "after", string(contents))
}

// A journal is not evidence of a crash while another process still owns its transaction lock.
func TestCheckTransactionWaitsForActiveMutation(t *testing.T) {
	for _, outcome := range []string{"completed", "interrupted", "conflicting"} {
		t.Run(outcome, func(t *testing.T) {
			project := t.TempDir()
			changes := []Change{{Name: "atmos.yaml", After: []byte("committed")}}
			raw, err := json.Marshal(changes)
			require.NoError(t, err)
			ready := make(chan struct{})
			release := make(chan struct{})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			mutation := make(chan error, 1)
			go func() {
				mutation <- Mutate(t.Context(), project, func() ([]Change, error) {
					// Pause at the journaled state with the actual Mutate lock held.
					if err := os.WriteFile(journalPath(project), raw, 0o600); err != nil {
						return nil, err
					}
					close(ready)
					<-release
					if outcome != "completed" {
						return nil, errUtils.ErrPluginTransactionPending
					}
					return changes, nil
				})
			}()
			select {
			case <-ready:
			case err := <-mutation:
				t.Fatalf("mutation failed before journaling: %v", err)
			}
			if outcome == "conflicting" {
				require.NoError(t, os.WriteFile(filepath.Join(project, "atmos.yaml"), []byte("external edit"), 0o600))
			}
			checked := make(chan error, 1)
			go func() { checked <- CheckTransaction(t.Context(), project) }()
			select {
			case err := <-checked:
				t.Fatalf("an active transaction must be awaited, got %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			close(release)
			if outcome == "completed" {
				require.NoError(t, <-mutation)
				require.NoError(t, <-checked)
				require.Equal(t, "committed", readFile(t, filepath.Join(project, "atmos.yaml")))
				require.NoFileExists(t, journalPath(project))
			} else {
				require.ErrorIs(t, <-mutation, errUtils.ErrPluginTransactionPending)
				require.ErrorIs(t, <-checked, errUtils.ErrPluginTransactionPending)
				require.Equal(t, string(raw), readFile(t, journalPath(project)))
			}
		})
	}
}

func TestCheckTransactionCanceledWhileMutationHoldsLock(t *testing.T) {
	project := t.TempDir()
	writeJournal(t, project, []byte(`[]`))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	require.NoError(t, filelock.New(journalPath(project)+".lock").WithExclusive(t.Context(), func() error {
		cancel()
		err := CheckTransaction(ctx, project)
		require.ErrorIs(t, err, context.Canceled)
		require.ErrorIs(t, err, filelock.ErrAcquire)
		require.NotErrorIs(t, err, errUtils.ErrPluginTransactionPending)
		return nil
	}))
	require.Equal(t, "[]", readFile(t, journalPath(project)), "canceling a reader must preserve the writer's journal")
}

func TestCheckTransactionCleanProjectCreatesNoFiles(t *testing.T) {
	for _, exists := range []bool{true, false} {
		project := filepath.Join(t.TempDir(), "project")
		if exists {
			require.NoError(t, os.Mkdir(project, 0o700))
		}
		require.NoError(t, CheckTransaction(t.Context(), project))
		require.NoDirExists(t, filepath.Join(project, ".atmos"))
		if exists {
			entries, err := os.ReadDir(project)
			require.NoError(t, err)
			require.Empty(t, entries)
		} else {
			require.NoDirExists(t, project)
		}
	}
}
