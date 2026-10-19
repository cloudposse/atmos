package planfile

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
)

const (
	artifactPermissions = 0o600
	maxPlanJSONBytes    = 64 * 1024 * 1024
)

// boundedBuffer bounds memory without printing or truncating input into a valid plan.
type boundedBuffer struct{ buffer bytes.Buffer }

func (b *boundedBuffer) Write(data []byte) (int, error) {
	defer perf.Track(nil, "planfile.boundedBuffer.Write")()

	if len(data) > maxPlanJSONBytes-b.buffer.Len() {
		return 0, fmt.Errorf("%w: plan JSON exceeds 64 MiB", errUtils.ErrPolicyInputInvalid)
	}
	return b.buffer.Write(data)
}

func readPlanJSON(ctx context.Context, runner process.Runner, invocation *Invocation, path string) ([]byte, error) {
	var output boundedBuffer
	result := runner.Run(ctx, process.TaskSpec{
		Command: invocation.Command,
		Args:    []string{"show", "-json", path},
		Dir:     invocation.Dir,
		Env:     invocation.Env,
		Streams: process.Streams{Stdout: &output, Stderr: io.Discard},
	})
	if !result.Started || !result.Success() || result.Canceled || result.Signaled {
		return nil, fmt.Errorf("%w: terraform show -json failed (output withheld)", errUtils.ErrPolicyPlan)
	}
	return output.buffer.Bytes(), nil
}

func openArtifact(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return nil, fmt.Errorf("%w: saved artifact must be a nonempty regular file", errUtils.ErrPolicyPlan)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		file.Close()
		return nil, fmt.Errorf("%w: artifact changed while opening", errUtils.ErrPolicyPlan)
	}
	return file, nil
}

func copyArtifact(source, destination string) error {
	input, err := openArtifact(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, artifactPermissions)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// publish replaces the requested output only after policy success.
// The staging file is on the destination filesystem, with mode 0600 on POSIX.
// Windows uses inherited ACLs, and os.Rename does not promise atomic replacement there.
func publish(source, destination string) error {
	file, err := os.CreateTemp(filepath.Dir(destination), ".atmos-approved-plan-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	input, err := openArtifact(source)
	if err != nil {
		file.Close()
		return err
	}
	defer input.Close()
	_, copyErr := io.Copy(file, input)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	// The destination is the caller's explicit Terraform -out path, not a confined workspace path.
	return os.Rename(file.Name(), destination)
}
