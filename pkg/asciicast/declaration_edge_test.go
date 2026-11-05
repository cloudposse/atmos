package asciicast

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/castplan"
)

// writeDeclarationEvents writes a v3 recording whose events are the given stream/data pairs.
func writeDeclarationEvents(t *testing.T, header string, events ...Event) string {
	t.Helper()
	content := header + "\n"
	for _, event := range events {
		encoded, err := json.Marshal([]any{0.1, event.Stream, event.Data})
		require.NoError(t, err)
		content += string(encoded) + "\n"
	}
	path := filepath.Join(t.TempDir(), "recording.cast")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func planMarker(kind, fingerprint string) Event {
	return Event{Stream: "m", Data: castplan.Marker{Kind: kind, Fingerprint: fingerprint}.Encode()}
}

func TestValidateDeclarationReadErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := ValidateDeclaration(filepath.Join(t.TempDir(), "missing.cast"), &castplan.Plan{})
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("empty file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.cast")
		require.NoError(t, os.WriteFile(path, nil, 0o600))
		_, err := ValidateDeclaration(path, &castplan.Plan{})
		require.ErrorIs(t, err, ErrEmptyCastFile)
	})

	t.Run("invalid header", func(t *testing.T) {
		path := writeDeclarationEvents(t, `{"version":`)
		_, err := ValidateDeclaration(path, &castplan.Plan{})
		require.ErrorIs(t, err, errUtils.ErrAutomation)
		require.ErrorContains(t, err, "invalid cast header")
	})

	t.Run("unserializable declaration cannot be fingerprinted", func(t *testing.T) {
		path := declarationRecording(t, 3, `[0.25,"o","hello"]`)
		plan := &castplan.Plan{Configuration: map[string]any{"bad": make(chan int)}}
		_, err := ValidateDeclaration(path, plan)
		require.ErrorIs(t, err, errUtils.ErrAutomation)
		require.ErrorContains(t, err, "cast declaration")
	})
}

func TestValidateDeclarationSkipsCommentLines(t *testing.T) {
	plan := &castplan.Plan{}
	fingerprint, err := plan.Fingerprint()
	require.NoError(t, err)
	path := writeDeclarationEvents(
		t, `{"version":3,"width":80,"height":24}`,
		planMarker("plan", fingerprint),
		Event{Stream: "o", Data: "visible"},
		planMarker("complete", fingerprint),
	)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	// Insert a comment line between the header and the first event.
	header, rest, found := splitFirstLine(string(content))
	require.True(t, found)
	require.NoError(t, os.WriteFile(path, []byte(header+"# a comment line\n"+rest), 0o600))
	text, err := ValidateDeclaration(path, plan)
	require.NoError(t, err)
	require.Equal(t, "visible", text)
}

func splitFirstLine(s string) (string, string, bool) {
	for i := range len(s) {
		if s[i] == '\n' {
			return s[:i+1], s[i+1:], true
		}
	}
	return s, "", false
}

func TestValidateDeclarationEventsRejectsBadEvidence(t *testing.T) {
	plan := &castplan.Plan{}
	fingerprint, err := plan.Fingerprint()
	require.NoError(t, err)
	plan0 := planMarker("plan", fingerprint)
	complete := planMarker("complete", fingerprint)

	tests := []struct {
		name     string
		events   []Event
		contains string
	}{
		{
			name:     "output after the completion marker",
			events:   []Event{plan0, complete, {Stream: "o", Data: "late"}},
			contains: "output follows completion marker",
		},
		{
			name:     "error stream output after the completion marker",
			events:   []Event{plan0, complete, {Stream: "e", Data: "late"}},
			contains: "output follows completion marker",
		},
		{
			name:     "extra marker after completion",
			events:   []Event{plan0, complete, planMarker("complete", fingerprint)},
			contains: "unexpected metadata after completion",
		},
		{
			name:     "marker payload is not JSON",
			events:   []Event{{Stream: "m", Data: castplan.MarkerPrefix + "{not json"}},
			contains: "invalid metadata",
		},
		{
			name: "plan marker carries an exit status",
			events: []Event{
				{Stream: "m", Data: castplan.Marker{Kind: "plan", Fingerprint: fingerprint, ExitCode: 2}.Encode()},
				complete,
			},
			contains: "unexpected exit status in plan marker",
		},
		{
			name:     "plan fingerprint differs",
			events:   []Event{planMarker("plan", "deadbeef"), complete},
			contains: "ordering changed",
		},
		{
			name:     "marker kind out of order",
			events:   []Event{complete},
			contains: "ordering changed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateDeclarationEvents(tt.events, plan)
			require.ErrorIs(t, err, errUtils.ErrAutomation)
			require.ErrorContains(t, err, tt.contains)
		})
	}

	t.Run("non-marker metadata is ignored", func(t *testing.T) {
		raw, err := validateDeclarationEvents([]Event{plan0, {Stream: "m", Data: "legacy/path.png"}, complete}, plan)
		require.NoError(t, err)
		require.Empty(t, raw)
	})
}

func TestValidateDeclarationEventsOrderingNamesTheCheckpoint(t *testing.T) {
	plan := &castplan.Plan{Actions: []castplan.Action{{Kind: "simulate", Fields: map[string]any{"text": "hi"}}}}
	fingerprint, err := plan.Fingerprint()
	require.NoError(t, err)
	events := []Event{
		planMarker("plan", fingerprint),
		{Stream: "m", Data: castplan.Marker{Kind: "begin", ID: "7"}.Encode()},
	}
	_, err = validateDeclarationEvents(events, plan)
	require.ErrorContains(t, err, "ordering changed at 0")
}

func sessionPlan(children ...castplan.Action) *castplan.Plan {
	return &castplan.Plan{Actions: []castplan.Action{{Kind: "session", Children: children}}}
}

func sessionEvents(t *testing.T, plan *castplan.Plan, output map[string]string, middle ...castplan.Marker) []Event {
	t.Helper()
	fingerprint, err := plan.Fingerprint()
	require.NoError(t, err)
	events := []Event{planMarker("plan", fingerprint), {Stream: "m", Data: castplan.Marker{Kind: "begin", ID: "0"}.Encode()}}
	for _, marker := range middle {
		if text := output[marker.ID]; text != "" {
			events = append(events, Event{Stream: "o", Data: text})
		}
		events = append(events, Event{Stream: "m", Data: marker.Encode()})
	}
	events = append(
		events,
		Event{Stream: "m", Data: castplan.Marker{Kind: "end", ID: "0"}.Encode()},
		planMarker("complete", fingerprint),
	)
	return events
}

func TestValidateDeclarationSessionWaits(t *testing.T) {
	plan := sessionPlan(
		castplan.Action{Kind: "write", Fields: map[string]any{"text": "ignored"}},
		castplan.Action{Kind: "wait", Fields: map[string]any{"regex": `ready-[0-9]+`}},
		castplan.Action{Kind: "hide"},
		castplan.Action{Kind: "wait", Fields: map[string]any{"text": "hidden-only"}},
		castplan.Action{Kind: "show"},
		castplan.Action{Kind: "wait", Fields: map[string]any{"text": "plain"}},
	)
	markers := []castplan.Marker{
		{Kind: "wait", ID: "0.1"},
		{Kind: "wait", ID: "0.3"},
		{Kind: "show", ID: "0.4"},
		{Kind: "wait", ID: "0.5"},
	}

	t.Run("regex wait is checked against the visible output", func(t *testing.T) {
		events := sessionEvents(t, plan, map[string]string{"0.1": "ready-42", "0.5": "plain text"}, markers...)
		raw, err := validateDeclarationEvents(events, plan)
		require.NoError(t, err)
		require.Equal(t, "ready-42plain text", raw)
	})

	t.Run("regex wait that never matched fails", func(t *testing.T) {
		events := sessionEvents(t, plan, map[string]string{"0.1": "ready-x", "0.5": "plain text"}, markers...)
		_, err := validateDeclarationEvents(events, plan)
		require.ErrorIs(t, err, errUtils.ErrAutomation)
		require.ErrorContains(t, err, `missing "ready-[0-9]+"`)
	})

	t.Run("text wait that never matched fails", func(t *testing.T) {
		events := sessionEvents(t, plan, map[string]string{"0.1": "ready-1", "0.5": "something else"}, markers...)
		_, err := validateDeclarationEvents(events, plan)
		require.ErrorContains(t, err, `missing "plain"`)
	})

	t.Run("a wait while hidden is not asserted against recorded output", func(t *testing.T) {
		// The hidden wait at 0.3 has no recorded output and must still pass.
		events := sessionEvents(t, plan, map[string]string{"0.1": "ready-9", "0.5": "plain"}, markers...)
		_, err := validateDeclarationEvents(events, plan)
		require.NoError(t, err)
	})

	t.Run("missing wait marker means the recording is incomplete", func(t *testing.T) {
		events := sessionEvents(t, plan, map[string]string{"0.1": "ready-9", "0.5": "plain"}, markers[:3]...)
		_, err := validateDeclarationEvents(events, plan)
		require.ErrorContains(t, err, "ordering changed")
	})
}

func TestValidateBeforeCommitReportsFlushFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.cast")
	rec, err := Start(&Options{Path: path, Overwrite: true})
	require.NoError(t, err)
	require.NoError(t, rec.Event("o", "buffered output that cannot be flushed"))
	// Closing the staged file under the recorder makes the buffered flush fail.
	require.NoError(t, rec.file.Close())
	called := false
	err = rec.ValidateBeforeCommit(func(string) error {
		called = true
		return nil
	})
	require.Error(t, err)
	require.False(t, called, "the validator must not run when staged evidence cannot be flushed")
	// Discard reports the same closed-file failure but must never publish the cast.
	require.Error(t, rec.Discard())
	require.NoFileExists(t, path)
}

func TestValidateBeforeCommitCommitsWhenValidatorPasses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.cast")
	rec, err := Start(&Options{Path: path, Overwrite: true})
	require.NoError(t, err)
	require.NoError(t, rec.Event("o", "hello"))
	var staged string
	require.NoError(t, rec.ValidateBeforeCommit(func(p string) error {
		staged = p
		return nil
	}))
	require.NotEqual(t, path, staged)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(content), "hello")
}
