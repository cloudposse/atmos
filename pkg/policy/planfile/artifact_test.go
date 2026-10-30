package planfile

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/process"
)

func TestArtifactFiles(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	destination := filepath.Join(directory, "copy")
	_, err := openArtifact(source)
	require.Error(t, err)
	_, err = openArtifact(directory)
	require.Error(t, err)
	require.NoError(t, os.WriteFile(source, nil, 0o600))
	_, err = openArtifact(source)
	require.Error(t, err)
	require.NoError(t, os.WriteFile(source, []byte("binary"), 0o600))
	require.NoError(t, copyArtifact(source, destination))
	require.Error(t, copyArtifact(source, destination))
	require.Error(t, copyArtifact(filepath.Join(directory, "missing"), destination))
	require.Error(t, publish(source, filepath.Join(directory, "missing", "plan")))
	require.Error(t, publish(filepath.Join(directory, "missing"), destination))
	require.Error(t, publish(source, directory))
	link := filepath.Join(directory, "link")
	require.NoError(t, os.Symlink(source, link))
	_, err = openArtifact(link)
	require.Error(t, err)
	_, err = artifactDigest(link)
	require.Error(t, err)
	assertClean(t, directory)
}

func TestBoundedPlanJSON(t *testing.T) {
	var buffer boundedBuffer
	_, err := buffer.Write([]byte("prefix"))
	require.NoError(t, err)
	_, err = buffer.Write(make([]byte, maxPlanJSONBytes))
	require.Error(t, err)
	// A promoted bytes.Buffer.ReadFrom would allow os/exec's io.Copy to bypass the limit.
	_, unbounded := any(&buffer).(io.ReaderFrom)
	require.False(t, unbounded)
}

func TestPlanNotProduced(t *testing.T) {
	guard, _ := newTestGuard(t, "false")
	directory := t.TempDir()
	_, err := guard.Execute(context.Background(), &Invocation{Dir: directory, Args: []string{"plan", "-out=p"}}, func([]string) process.Result {
		return process.Result{Started: true}
	})
	require.ErrorContains(t, err, "read saved artifact")
	assertClean(t, directory)
}

func TestPublishFailure(t *testing.T) {
	guard, runner := newTestGuard(t, "false")
	expectShow(t, runner, validPlan)
	directory := t.TempDir()
	_, err := guard.Execute(context.Background(), &Invocation{Dir: directory, Args: []string{"plan", "-out", filepath.Join(directory, "missing", "p")}}, func(args []string) process.Result {
		require.NoError(t, os.WriteFile(args[2], []byte("binary"), 0o600))
		return process.Result{Started: true}
	})
	require.ErrorContains(t, err, "publish approved plan")
	assertClean(t, directory)
}
