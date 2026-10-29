package build

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/cloudposse/atmos/pkg/process"
)

const maxConcurrentBuilds = 4

// compileTargets resolves dependencies once before starting independent compilers.
// Each compiler owns its environment and output; diagnostics and successful paths
// are returned in platform order, independent of completion order.
func compileTargets(ctx context.Context, directory string, targets []buildTarget, config *settings) ([]string, error) {
	config.environment = targets[0].platform.Environment(os.Environ())
	if len(targets) == 1 {
		if err := compile(ctx, directory, targets[0].executable, config); err != nil {
			return nil, err
		}
		return []string{filepath.Join(directory, targets[0].executable)}, nil
	}
	if err := prepareModule(ctx, directory, config); err != nil {
		return nil, err
	}
	config.prepared = true
	stdout, stderr := make([]bytes.Buffer, len(targets)), make([]bytes.Buffer, len(targets))
	completed, failures := make([]string, len(targets)), make([]error, len(targets))
	var group errgroup.Group
	group.SetLimit(maxConcurrentBuilds)
	for index, target := range targets {
		group.Go(func() error {
			local := *config
			local.environment = target.platform.Environment(os.Environ())
			local.streams = process.Streams{Stdout: &stdout[index], Stderr: &stderr[index]}
			if err := ctx.Err(); err != nil {
				failures[index] = err
				return err
			}
			failures[index] = compile(ctx, directory, target.executable, &local)
			if failures[index] == nil {
				completed[index] = filepath.Join(directory, target.executable)
			}
			return nil
		})
	}
	_ = group.Wait()
	var paths []string
	for index, path := range completed {
		failures[index] = errors.Join(failures[index], copyDiagnostics(config.streams.Stdout, &stdout[index]), copyDiagnostics(config.streams.Stderr, &stderr[index]))
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths, errors.Join(failures...)
}

func copyDiagnostics(destination io.Writer, source io.Reader) error {
	if destination == nil {
		destination = io.Discard
	}
	_, err := io.Copy(destination, source)
	return err
}
