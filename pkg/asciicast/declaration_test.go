package asciicast

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/castplan"
)

func TestDeclarationMetadataAndAssertions(t *testing.T) {
	plan := &castplan.Plan{Actions: []castplan.Action{{Kind: "run", Fields: map[string]any{"command": "displayed"}, Expect: []castplan.Matcher{{Text: "answer"}}}}}
	hash, err := plan.Fingerprint()
	require.NoError(t, err)
	marker := func(kind, id string) Event {
		return Event{Stream: "m", Data: castplan.Marker{Kind: kind, ID: id}.Encode()}
	}
	events := []Event{
		{Stream: "m", Data: castplan.Marker{Kind: "plan", Fingerprint: hash}.Encode()},
		{Stream: "o", Data: "displayed answer"},
		marker("begin", "0"),
		{Stream: "o", Data: "\x1b[32mans"},
		{Stream: "o", Data: "wer\x1b[0m"},
		marker("end", "0"),
		{Stream: "m", Data: castplan.Marker{Kind: "complete", Fingerprint: hash}.Encode()},
	}
	raw, err := validateDeclarationEvents(events, plan)
	require.NoError(t, err)
	require.Contains(t, raw, "\x1b[32m")
	bad := append([]Event(nil), events...)
	bad[3].Data = ""
	bad[4].Data = ""
	_, err = validateDeclarationEvents(bad, plan)
	require.ErrorContains(t, err, "missing")
	_, err = validateDeclarationEvents(events[:len(events)-1], plan)
	require.ErrorContains(t, err, "incomplete")
	bad = append(append([]Event(nil), events...), marker("end", "0"))
	_, err = validateDeclarationEvents(bad, plan)
	require.ErrorContains(t, err, "unexpected")
	plan.Actions[0].Fields["command"] = "different"
	_, err = validateDeclarationEvents(events, plan)
	require.ErrorContains(t, err, "regenerate")
}

func TestValidationFailureDoesNotCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.cast")
	require.NoError(t, os.WriteFile(path, []byte("previous"), 0o600))
	rec, err := Start(&Options{Path: path, Overwrite: true})
	require.NoError(t, err)
	require.NoError(t, rec.Event("o", "new"))
	err = rec.ValidateBeforeCommit(func(staged string) error {
		require.NotEqual(t, path, staged)
		return declarationError("policy failure")
	})
	require.ErrorContains(t, err, "policy failure")
	require.NoError(t, rec.Discard())
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "previous", string(content))
}

func TestScopedWaitDoesNotMatchEarlierOutput(t *testing.T) {
	state := &sessionState{scopedWaits: true}
	state.output.WriteString("\x1b[32mready\x1b[0m")
	require.True(t, outputMatches(state, "ready", nil))
	require.False(t, outputMatches(state, "ready", nil))
	state.output.WriteString("rea")
	require.False(t, outputMatches(state, "ready", nil))
	state.output.WriteString("dy")
	require.True(t, outputMatches(state, "ready", nil))
}

func TestTerminalTypingCancellation(t *testing.T) {
	for _, kind := range []string{"write", "key"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			action := SessionAction{Type: kind, Text: "typing", Key: "enter", Repeat: 3, Rate: "1s", Interval: "1s"}
			done := make(chan error, 1)
			go func() {
				done <- runAction(ctx, &syncWriter{w: io.Discard}, &sessionState{}, &action, &SessionOptions{})
			}()
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(200 * time.Millisecond):
				t.Error("typing did not stop promptly after cancellation")
			}
		})
	}
}

func TestScopedCheckpointConsumesOutputThroughMarker(t *testing.T) {
	for _, kind := range []string{"wait", "show"} {
		t.Run(kind, func(t *testing.T) {
			state := &sessionState{scopedWaits: true}
			state.output.WriteString("output before checkpoint")
			observed := false
			opts := &SessionOptions{
				ScopedWaits: true,
				Actions:     []SessionAction{{Type: kind}},
				Observe: func(_ int, _ *SessionAction) error {
					observed = true
					require.Equal(t, state.output.Len(), state.waitOffset)
					return nil
				},
			}
			require.NoError(t, observeSessionAction(state, opts, 0))
			require.True(t, observed)
			require.False(t, outputMatches(state, "output before checkpoint", nil))
			state.output.WriteString("fresh output")
			require.True(t, outputMatches(state, "fresh output", nil))
		})
	}
}

// TestObserveHoldsOutputUntilCheckpointRecorded pins the contract that the
// checkpoint callback runs with the session output lock held: PTY output that
// arrives while the checkpoint marker is being recorded must wait, so it lands
// after the checkpoint instead of inside the span the wait already consumed.
func TestObserveHoldsOutputUntilCheckpointRecorded(t *testing.T) {
	state := &sessionState{scopedWaits: true, discardRecording: true, changed: make(chan struct{}, 1), done: make(chan error, 1)}
	state.output.WriteString("consumed")
	arrived := make(chan struct{})
	opts := &SessionOptions{
		ScopedWaits: true,
		Actions:     []SessionAction{{Type: "wait"}},
		Observe: func(_ int, _ *SessionAction) error {
			go func() {
				state.recordOutputChunk([]byte("late"))
				close(arrived)
			}()
			select {
			case <-arrived:
				t.Error("output was recorded while the checkpoint was being emitted")
			case <-time.After(100 * time.Millisecond):
			}
			return nil
		},
	}
	require.NoError(t, observeSessionAction(state, opts, 0))
	<-arrived
	state.mu.Lock()
	afterCheckpoint := state.output.String()[state.waitOffset:]
	state.mu.Unlock()
	require.Equal(t, "late", afterCheckpoint)
}
