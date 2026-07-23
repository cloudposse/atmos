package output

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tfplugin "github.com/cloudposse/atmos/pkg/terraform/plugin"
)

type blockingInitRunner struct {
	started chan struct{}
	release <-chan struct{}
}

func (r *blockingInitRunner) Init(context.Context, ...tfexec.InitOption) error {
	r.started <- struct{}{}
	<-r.release
	return nil
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
		errs <- executor.runInit(context.Background(), runner, &ComponentConfig{}, "one", "stack", nil, cache)
	}()
	require.Eventually(t, func() bool { return len(runner.started) == 1 }, time.Second, 10*time.Millisecond)

	go func() {
		errs <- executor.runInit(context.Background(), runner, &ComponentConfig{}, "two", "stack", nil, cache)
	}()
	assert.Never(t, func() bool { return len(runner.started) == 2 }, 100*time.Millisecond, 10*time.Millisecond)

	close(release)
	require.Eventually(t, func() bool { return len(runner.started) == 2 }, time.Second, 10*time.Millisecond)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
}
