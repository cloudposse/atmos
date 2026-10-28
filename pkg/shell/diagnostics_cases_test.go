package shell

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/diagnostics"
	"github.com/cloudposse/atmos/pkg/process"
)

func TestProcessEndDiagnostics(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, tc := range []struct {
		name         string
		result       *process.Result
		wantDuration int64
	}{
		{name: "nil does not emit"},
		{name: "explicit timestamps", result: &process.Result{Started: true, StartedAt: start, FinishedAt: start.Add(250 * time.Millisecond)}, wantDuration: 250},
		{name: "fallback start", result: &process.Result{FinishedAt: start.Add(time.Second)}, wantDuration: 1000},
		{name: "missing finish", result: &process.Result{StartedAt: time.Now()}, wantDuration: -1},
		{name: "signal metadata", result: &process.Result{Started: true, StartedAt: start, FinishedAt: start.Add(time.Second), ExitCode: 143, Signaled: true, Signal: "terminated", SignalNumber: 15, Err: os.ErrProcessDone}, wantDuration: 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "events.jsonl")
			emitProcessEndDiagnostics(diagnostics.Config{Enabled: true, File: path}, "process-example", start, tc.result)
			data, err := os.ReadFile(path)
			if tc.result == nil {
				require.ErrorIs(t, err, os.ErrNotExist)
				return
			}
			require.NoError(t, err)
			var event diagnostics.Event
			require.NoError(t, json.Unmarshal(data, &event))
			assert.Equal(t, "process.end", event.Type)
			assert.Equal(t, "process-example", event.ID)
			require.NotNil(t, event.DurationMS)
			if tc.wantDuration >= 0 {
				assert.Equal(t, tc.wantDuration, *event.DurationMS)
			} else {
				assert.GreaterOrEqual(t, *event.DurationMS, int64(0))
			}
			require.NotNil(t, event.Success)
			assert.Equal(t, tc.result.Err == nil, *event.Success)
			if tc.result.Signaled {
				require.NotNil(t, event.SignalNumber)
				assert.Equal(t, 15, *event.SignalNumber)
				assert.Equal(t, "terminated", event.Signal)
				assert.Equal(t, os.ErrProcessDone.Error(), event.Error)
			} else {
				assert.Nil(t, event.SignalNumber)
			}
		})
	}
}

func TestDiagnosticsPathAndNonTerminalInputs(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	for _, tc := range []struct{ name, dir, want string }{
		{name: "current directory", want: cwd},
		{name: "relative path", dir: filepath.Join("child", "nested"), want: filepath.Join(cwd, "child", "nested")},
		{name: "absolute path", dir: filepath.Join(cwd, "child"), want: filepath.Join(cwd, "child")},
	} {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.want, resolveDiagnosticsCWD(tc.dir)) })
	}
	file, err := os.CreateTemp(t.TempDir(), "input")
	require.NoError(t, err)
	defer file.Close()
	assert.False(t, isTTYReader(file))
	assert.False(t, isTTYReader(strings.NewReader("input")))
	var nilFile *os.File
	assert.False(t, isTTYReader(nilFile))
}
