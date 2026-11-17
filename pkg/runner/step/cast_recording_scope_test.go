package step

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/muesli/termenv"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/asciicast"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Compile-time sentinels so a schema field rename fails the build here.
var _ = schema.WorkflowStep{Name: "x", Type: schema.TaskTypeCast, CastOutput: &schema.CastOutput{Cast: "x"}}

// setupCastUIEnvTest initializes the UI formatter and clears every env var that
// influences color detection so assertions do not depend on the host.
func setupCastUIEnvTest(t *testing.T) {
	t.Helper()
	require.NoError(t, iolib.Initialize())
	require.NoError(t, viper.BindEnv("force-color", "ATMOS_FORCE_COLOR", "CLICOLOR_FORCE"))
	for _, key := range castUIEnvKeys {
		withRestoredEnv(t, key)
	}
	ui.InitFormatter(iolib.GetContext())
	t.Cleanup(ui.ReinitFormatter)
	t.Cleanup(func() {
		castUIEnv.mu.Lock()
		defer castUIEnv.mu.Unlock()
		require.Zero(t, castUIEnv.depth, "forceRecordingUIEnv scopes leaked")
	})
}

func newTestRecordingStep(t *testing.T, name string) (*schema.WorkflowStep, string) {
	t.Helper()
	castPath := filepath.Join(t.TempDir(), name+".cast")
	return &schema.WorkflowStep{
		Name:       name,
		Type:       schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{Cast: castPath},
	}, castPath
}

// TestForceRecordingUIEnvRestoresWhenFirstScopeExitsFirst covers overlapping
// (non-nested) scopes: the recording that started first exits first, and the
// last one to exit must still restore the original environment and formatter.
func TestForceRecordingUIEnvRestoresWhenFirstScopeExitsFirst(t *testing.T) {
	setupCastUIEnvTest(t)
	const key = "ATMOS_FORCE_COLOR"
	before := ui.GetColorProfile()
	require.NotEqual(t, termenv.TrueColor, before, "test precondition: color must not be forced")

	first := forceRecordingUIEnv(map[string]string{key: "1"})
	second := forceRecordingUIEnv(map[string]string{key: "1"})
	assert.Equal(t, termenv.TrueColor, ui.GetColorProfile())

	first()
	got, ok := os.LookupEnv(key)
	assert.True(t, ok)
	assert.Equal(t, "1", got, "env must stay forced while another recording is active")
	assert.Equal(t, termenv.TrueColor, ui.GetColorProfile())

	second()
	_, ok = os.LookupEnv(key)
	assert.False(t, ok, "env must be restored when the last recording exits")
	assert.Equal(t, before, ui.GetColorProfile(), "formatter must be restored when the last recording exits")
}

// TestForceRecordingUIEnvRestoresPreexistingValueAndLaterKeys checks that the
// original value of every forced key is restored, including keys that only a
// later overlapping recording forced, regardless of exit order.
func TestForceRecordingUIEnvRestoresPreexistingValueAndLaterKeys(t *testing.T) {
	setupCastUIEnvTest(t)
	const colorKey, ttyKey = "ATMOS_FORCE_COLOR", "ATMOS_FORCE_TTY"
	t.Setenv(colorKey, "0")

	first := forceRecordingUIEnv(map[string]string{colorKey: "1"})
	second := forceRecordingUIEnv(map[string]string{ttyKey: "1"})

	got, ok := os.LookupEnv(ttyKey)
	assert.True(t, ok)
	assert.Equal(t, "1", got, "a later overlapping recording must still force its own keys")

	first()
	got, ok = os.LookupEnv(colorKey)
	assert.True(t, ok)
	assert.Equal(t, "1", got)

	second()
	got, ok = os.LookupEnv(colorKey)
	assert.True(t, ok)
	assert.Equal(t, "0", got, "pre-existing value must be restored")
	_, ok = os.LookupEnv(ttyKey)
	assert.False(t, ok, "key forced only by the later recording must be unset")
}

func TestForceRecordingUIEnvRestoreIsIdempotent(t *testing.T) {
	setupCastUIEnvTest(t)
	const key = "FORCE_COLOR"

	outer := forceRecordingUIEnv(map[string]string{key: "1"})
	inner := forceRecordingUIEnv(map[string]string{key: "1"})
	inner()
	inner() // A repeated restore must not release the outer scope.

	got, ok := os.LookupEnv(key)
	assert.True(t, ok)
	assert.Equal(t, "1", got, "double restore must not end the outer scope")

	outer()
	_, ok = os.LookupEnv(key)
	assert.False(t, ok)
}

// TestStartStepRecorderRejectsOverlappingRecording verifies that a second
// recording cannot take over the process-wide recorder while one is active,
// and that the active recording's output is not misrouted.
func TestStartStepRecorderRejectsOverlappingRecording(t *testing.T) {
	require.NoError(t, iolib.Initialize())
	firstStep, firstPath := newTestRecordingStep(t, "first")
	secondStep, secondPath := newTestRecordingStep(t, "second")

	first, restoreFirst, err := startStepRecorder(firstStep, NewVariables())
	require.NoError(t, err)
	t.Cleanup(restoreFirst)

	second, restoreSecond, err := startStepRecorder(secondStep, NewVariables())
	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrCastRecordingActive)
	assert.Nil(t, second)
	assert.Nil(t, restoreSecond)
	assert.NoFileExists(t, secondPath, "a rejected recording must not create its cast file")

	// Output after the rejected attempt still reaches the first recording.
	iolib.RecordMarker("during-first")
	restoreFirst()
	iolib.RecordMarker("after-restore")
	require.NoError(t, first.Close())

	content, err := os.ReadFile(firstPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "during-first")
	assert.NotContains(t, string(content), "after-restore", "restore must uninstall the recorder")
}

// TestStartStepRecorderAllowsSequentialRecordings is the negative path: once the
// previous recording restored, a new one starts, and each cast only holds its
// own events.
func TestStartStepRecorderAllowsSequentialRecordings(t *testing.T) {
	require.NoError(t, iolib.Initialize())
	var contents []string
	for _, name := range []string{"one", "two"} {
		step, path := newTestRecordingStep(t, name)
		rec, restore, err := startStepRecorder(step, NewVariables())
		require.NoError(t, err, name)
		iolib.RecordMarker("marker-" + name)
		restore()
		require.NoError(t, rec.Close())
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		contents = append(contents, string(data))
	}
	assert.Contains(t, contents[0], "marker-one")
	assert.NotContains(t, contents[0], "marker-two")
	assert.Contains(t, contents[1], "marker-two")
	assert.NotContains(t, contents[1], "marker-one")
}

// TestStartStepRecorderReleasesGuardOnFailure ensures failed starts and a
// repeated restore never leave the recording guard held.
func TestStartStepRecorderReleasesGuardOnFailure(t *testing.T) {
	require.NoError(t, iolib.Initialize())

	blockingFile := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(blockingFile, []byte("x"), 0o644))
	failing := &schema.WorkflowStep{
		Name:       "failing",
		Type:       schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{Cast: filepath.Join(blockingFile, "nested", "demo.cast")},
	}
	_, _, err := startStepRecorder(failing, NewVariables())
	require.Error(t, err)
	assert.False(t, errors.Is(err, errUtils.ErrCastRecordingActive))

	badRate := &schema.WorkflowStep{Name: "bad-rate", Type: schema.TaskTypeCast, Rate: "bad-duration"}
	_, _, err = startStepRecorder(badRate, NewVariables())
	require.Error(t, err)
	assert.False(t, errors.Is(err, errUtils.ErrCastRecordingActive))

	step, _ := newTestRecordingStep(t, "after-failure")
	rec, restore, err := startStepRecorder(step, NewVariables())
	require.NoError(t, err, "guard must be free after failed starts")
	restore()
	restore() // Idempotent: must not free a guard owned by a later recording.
	require.NoError(t, rec.Close())

	nextStep, _ := newTestRecordingStep(t, "next")
	next, nextRestore, err := startStepRecorder(nextStep, NewVariables())
	require.NoError(t, err)
	restore() // Stale restore from the earlier recording.
	_, _, err = startStepRecorder(step, NewVariables())
	require.ErrorIs(t, err, errUtils.ErrCastRecordingActive, "a stale restore call must not release the next recording's guard")
	nextRestore()
	require.NoError(t, next.Close())
}

// TestStartStepRecorderConcurrentStartsAdmitExactlyOne races many starts; with
// the guard only one may hold the recorder at a time.
func TestStartStepRecorderConcurrentStartsAdmitExactlyOne(t *testing.T) {
	require.NoError(t, iolib.Initialize())
	const workers = 8
	type outcome struct {
		rec     *asciicast.Recorder
		restore func()
		err     error
	}
	outcomes := make([]outcome, workers)
	ready := make(chan struct{})
	var wg sync.WaitGroup
	for i := range workers {
		step, _ := newTestRecordingStep(t, "worker-"+strings.Repeat("x", i+1))
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			rec, restore, err := startStepRecorder(step, NewVariables())
			outcomes[i] = outcome{rec: rec, restore: restore, err: err}
		}()
	}
	close(ready)
	wg.Wait()

	admitted := 0
	for _, o := range outcomes {
		if o.err == nil {
			admitted++
			o.restore()
			assert.NoError(t, o.rec.Close())
			continue
		}
		assert.ErrorIs(t, o.err, errUtils.ErrCastRecordingActive)
	}
	assert.Equal(t, 1, admitted)
}
