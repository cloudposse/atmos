package agent

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.json")
	_, err := ReadState(path)
	require.ErrorIs(t, err, ErrNoState)

	want := State{PID: 4242, Version: "1.2.3", Started: time.Date(2026, 11, 12, 1, 2, 3, 0, time.UTC)}
	require.NoError(t, WriteState(path, want))
	got, err := ReadState(path)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestRemoveStateOnlyOwnRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.json")
	require.NoError(t, WriteState(path, State{PID: 10, Version: "v"}))

	require.NoError(t, RemoveState(path, 11))
	_, err := os.Stat(path)
	require.NoError(t, err, "another daemon's record stays")

	require.NoError(t, RemoveState(path, 10))
	_, err = os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)

	require.NoError(t, RemoveState(path, 10), "removing a missing record is fine")
}

func TestAuditLogWritesJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	log, err := OpenAuditLog(path)
	require.NoError(t, err)
	require.NoError(t, log.Record(AuditEntry{Identity: "ops", Operation: OpSign, Result: ResultOK, Classification: KindGitSignature, Namespace: "git"}))
	require.NoError(t, log.Record(AuditEntry{Identity: "ops", Operation: OpList, Result: ResultOK}))
	require.NoError(t, log.Close())

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	var entries []AuditEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e AuditEntry
		require.NoError(t, json.Unmarshal(sc.Bytes(), &e))
		entries = append(entries, e)
	}
	require.Len(t, entries, 2)
	assert.False(t, entries[0].Time.IsZero())
	assert.Equal(t, "git", entries[0].Namespace)
	assert.Equal(t, OpList, entries[1].Operation)

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestNilAuditLogDiscards(t *testing.T) {
	var log *AuditLog
	require.NoError(t, log.Record(AuditEntry{}))
	require.NoError(t, log.Close())
}
