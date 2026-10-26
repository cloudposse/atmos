package script

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// PrepareSpec normalizes invocation paths and supplies discard streams. The caller
// must pass its own copy; input maps and slices remain read-only throughout execution.
func PrepareSpec(spec *Spec) error {
	defer perf.Track(nil, "script.PrepareSpec")()
	if spec.Stdout == nil {
		spec.Stdout = io.Discard
	}
	if spec.Stderr == nil {
		spec.Stderr = io.Discard
	}
	if spec.Name == "" {
		spec.Name = "<script>"
	}
	dir, err := filepath.Abs(spec.WorkingDirectory)
	if err != nil {
		return err
	}
	spec.WorkingDirectory = dir
	if spec.ProjectRoot != "" {
		if spec.ProjectRoot, err = filepath.Abs(spec.ProjectRoot); err != nil {
			return err
		}
	}
	if spec.SourcePath != "" {
		if spec.SourcePath, err = filepath.Abs(spec.SourcePath); err != nil {
			return err
		}
		if spec.File == nil {
			spec.File = &File{Path: spec.SourcePath}
		}
	}
	spec.AtmosWorkingDirectory, err = filepath.Abs(spec.AtmosWorkingDirectory)
	return err
}

// ValidateWorkingDirectory fails fast, before any code runs, when the directory is unusable.
func ValidateWorkingDirectory(dir string) error {
	defer perf.Track(nil, "script.ValidateWorkingDirectory")()
	info, err := os.Stat(dir)
	var failed error
	switch {
	case errors.Is(err, fs.ErrNotExist):
		failed = serviceFailure(errUtils.ErrScript, err, "working directory %q does not exist", dir)
	case err != nil:
		failed = serviceFailure(errUtils.ErrScript, err, "working directory %q is not accessible: %s", dir, err)
	case !info.IsDir():
		failed = serviceFailure(errUtils.ErrScript, nil, "working directory %q is not a directory", dir)
	default:
		return nil
	}
	return errUtils.Build(failed).WithHint("Check the `working_directory` setting; it must name an existing directory.").Err()
}
