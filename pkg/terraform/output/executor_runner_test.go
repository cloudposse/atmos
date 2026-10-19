package output

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	tfplugin "github.com/cloudposse/atmos/pkg/terraform/plugin"
)

// initOptionString renders a tfexec.InitOption via fmt for content assertions.
// The tfexec.Reconfigure/Upgrade constructors return pointers to structs with
// unexported bool fields and no exported accessor, so this (fmt's %+v verb
// reads unexported fields even though reflection alone cannot) is the only way
// to assert the actual flag value passed to `terraform init` from outside the
// tfexec package. Confirmed format: tfexec.Reconfigure(true) -> "&{reconfigure:true}".
func initOptionString(opt tfexec.InitOption) string {
	return fmt.Sprintf("%+v", opt)
}

type blockingInitRunner struct {
	started chan struct{}
	release <-chan struct{}
	initErr error

	mu        sync.Mutex
	active    int
	maxActive int
}

func (r *blockingInitRunner) Init(context.Context, ...tfexec.InitOption) error {
	r.mu.Lock()
	r.active++
	if r.active > r.maxActive {
		r.maxActive = r.active
	}
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.active--
		r.mu.Unlock()
	}()

	if r.started != nil {
		r.started <- struct{}{}
	}
	if r.release != nil {
		<-r.release
	}
	return r.initErr
}

func TestRunInitReturnsRunnerErrorWithoutLockWrapping(t *testing.T) {
	runnerErr := errors.New("runner init failed")
	runner := &blockingInitRunner{initErr: runnerErr}
	err := (&Executor{}).runInit(
		context.Background(),
		runner,
		&ComponentConfig{ComponentPath: t.TempDir()},
		"component",
		"stack",
		nil,
		tfplugin.Cache{Directory: t.TempDir()},
		false,
		false,
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTerraformInit)
	assert.ErrorIs(t, err, runnerErr)
	assert.Contains(t, err.Error(), "runner init failed")
	assert.NotContains(t, err.Error(), "lock provider plugin cache")
}

func (r *blockingInitRunner) WorkspaceNew(context.Context, string, ...tfexec.WorkspaceNewCmdOption) error {
	return nil
}

func (r *blockingInitRunner) WorkspaceSelect(context.Context, string, ...tfexec.WorkspaceSelectOption) error {
	return nil
}

func (r *blockingInitRunner) Output(context.Context, ...tfexec.OutputOption) (map[string]tfexec.OutputMeta, error) {
	return nil, nil
}
func (r *blockingInitRunner) SetStdout(io.Writer)            {}
func (r *blockingInitRunner) SetStderr(io.Writer)            {}
func (r *blockingInitRunner) SetEnv(map[string]string) error { return nil }

func TestRunInitSerializesSharedPluginCache(t *testing.T) {
	release := make(chan struct{})
	runner := &blockingInitRunner{started: make(chan struct{}, 2), release: release}
	cache := tfplugin.Cache{Directory: t.TempDir()}
	executor := &Executor{}
	errs := make(chan error, 2)

	go func() {
		errs <- executor.runInit(context.Background(), runner, &ComponentConfig{}, "one", "stack", nil, cache, false, false)
	}()
	require.Eventually(t, func() bool { return len(runner.started) == 1 }, time.Second, 10*time.Millisecond)

	go func() {
		errs <- executor.runInit(context.Background(), runner, &ComponentConfig{}, "two", "stack", nil, cache, false, false)
	}()
	assert.Never(t, func() bool { return len(runner.started) == 2 }, 100*time.Millisecond, 10*time.Millisecond)

	close(release)
	require.Eventually(t, func() bool { return len(runner.started) == 2 }, time.Second, 10*time.Millisecond)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)

	// maxActive records the peak number of concurrently active Init calls,
	// updated under a mutex at each call's entry/exit. It stays accurate
	// regardless of scheduling delays: if the lock ever let both calls run
	// concurrently, this would deterministically show 2 no matter when the
	// second goroutine happened to be scheduled.
	runner.mu.Lock()
	defer runner.mu.Unlock()
	assert.Equal(t, 1, runner.maxActive, "runInit must serialize concurrent Init calls sharing one plugin cache")
}

// --- runInit flag decision + diagnostic-retry tests ---

func TestRunInit_ReconfigureOnlyWhenDecided(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockTerraformRunner(ctrl)
	var captured []tfexec.InitOption
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, opts ...tfexec.InitOption) error {
			captured = opts
			return nil
		},
	)

	executor := &Executor{}
	err := executor.runInit(context.Background(), mockRunner, &ComponentConfig{ComponentPath: t.TempDir()}, "c", "s", nil, tfplugin.Cache{}, true, false)
	require.NoError(t, err)

	require.Len(t, captured, 2)
	strs := []string{initOptionString(captured[0]), initOptionString(captured[1])}
	assert.Contains(t, strs, "&{upgrade:false}")
	assert.Contains(t, strs, "&{reconfigure:true}")
}

func TestRunInit_UpgradeWhenDecided(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockTerraformRunner(ctrl)
	var captured []tfexec.InitOption
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, opts ...tfexec.InitOption) error {
			captured = opts
			return nil
		},
	)

	executor := &Executor{}
	err := executor.runInit(context.Background(), mockRunner, &ComponentConfig{ComponentPath: t.TempDir()}, "c", "s", nil, tfplugin.Cache{}, false, true)
	require.NoError(t, err)

	require.Len(t, captured, 1, "upgrade alone must not add -reconfigure")
	assert.Equal(t, "&{upgrade:true}", initOptionString(captured[0]))
}

// TestRunInitOnce_ClassificationIgnoresLeftoverStderrFromEarlierCommand guards against a
// regression where stderrCapture -- shared across every terraform-exec call within a single
// execute() invocation -- carried leftover stderr from an earlier, already-succeeded command
// (e.g. a workspace select, or a prior recovered `output` call) into diagnosticText/Classify for
// THIS init's own failure, potentially triggering the wrong recovery. The fix resets the capture
// before runInitOnce classifies its own failure, so only its own stderr is considered.
func TestRunInitOnce_ClassificationIgnoresLeftoverStderrFromEarlierCommand(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockTerraformRunner(ctrl)
	// Init fails with a plain (non-diagnostic) error; if leftover stderr from an earlier
	// command were still in the capture, Classify could wrongly diagnose an upgrade/reconfigure
	// requirement that this failure never actually reported.
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(errors.New("permission denied"))

	stderrCapture := newQuietModeWriter()
	// Simulate leftover stderr from an earlier, already-succeeded command sharing this writer.
	stderrCapture.buffer.WriteString("Error: Backend configuration changed\nmust use terraform init -upgrade")

	executor := &Executor{}
	config := &ComponentConfig{ComponentPath: t.TempDir()}
	err := executor.runInit(context.Background(), mockRunner, config, "c", "s", stderrCapture, tfplugin.Cache{}, false, false)

	require.Error(t, err, "a plain init failure must not be swallowed by a stale diagnostic match")
	assert.True(t, errors.Is(err, errUtils.ErrTerraformInit))
}

func TestRunInit_RetriesWithUpgradeOnDiagnostic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRunner := NewMockTerraformRunner(ctrl)
	callCount := 0
	var secondCallOpts []tfexec.InitOption
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, opts ...tfexec.InitOption) error {
			callCount++
			if callCount == 1 {
				return errors.New("Error: Inconsistent dependency lock file\n\nmust use terraform init -upgrade")
			}
			secondCallOpts = opts
			return nil
		},
	).Times(2)

	executor := &Executor{}
	config := &ComponentConfig{ComponentPath: t.TempDir()}
	err := executor.runInit(context.Background(), mockRunner, config, "c", "s", nil, tfplugin.Cache{}, false, false)
	require.NoError(t, err)

	assert.Equal(t, 2, callCount, "must retry init exactly once on an upgrade-required diagnostic")
	require.Len(t, secondCallOpts, 1, "the retry must still not add -reconfigure")
	assert.Equal(t, "&{upgrade:true}", initOptionString(secondCallOpts[0]), "the retry must add -upgrade")
}
