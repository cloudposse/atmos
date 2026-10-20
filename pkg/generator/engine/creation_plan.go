package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ValidateCreation checks every planned output before hooks or writes begin.
func ValidateCreation(target string, files []File) error {
	defer perf.Track(nil, "engine.ValidateCreation")()

	seen := make(map[string]bool)
	for _, file := range files {
		if err := validateRenderedPath(file.Path, file.Path, nil); err != nil {
			return err
		}
		full := filepath.Join(target, file.Path)
		if err := validateWriteTarget(full, target); err != nil {
			return err
		}
		key := filepath.Clean(full)
		if seen[key] {
			return fmt.Errorf("%w: %s", errUtils.ErrScaffoldDuplicateOutputPath, file.Path)
		}
		seen[key] = true
		if _, err := os.Lstat(full); err == nil {
			return fmt.Errorf("%w: %s already exists", errUtils.ErrFileWrite, full)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	// A file cannot also be a parent directory of another output.
	for path := range seen {
		for parent := filepath.Dir(path); parent != filepath.Dir(parent); parent = filepath.Dir(parent) {
			if seen[parent] {
				return fmt.Errorf("%w: output is also a directory: %s", errUtils.ErrScaffoldDuplicateOutputPath, parent)
			}
		}
	}
	return nil
}

// ApplyCreation writes already-rendered bytes with exclusive creation, checking
// the complete plan again after hooks. It never overwrites a destination.
func ApplyCreation(ctx context.Context, target string, files []File) error {
	defer perf.Track(nil, "engine.ApplyCreation")()

	return applyCreation(ctx, target, files, writeCreationFile)
}

func applyCreation(ctx context.Context, target string, files []File, write func(string, *File) (*createdFile, error)) (err error) {
	if err := ValidateCreation(target, files); err != nil {
		return err
	}
	var created []*createdFile
	defer func() {
		if err != nil {
			err = errors.Join(err, rollbackCreation(created))
		}
	}()
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		full := filepath.Join(target, file.Path)
		if err := ensureDirectory(full); err != nil {
			return err
		}
		if err := validateWriteTarget(full, target); err != nil {
			return err
		}
		owned, err := write(full, &file)
		if owned != nil {
			created = append(created, owned)
		}
		if err != nil {
			return fmt.Errorf("%w: %s: %w", errUtils.ErrFileWrite, file.Path, err)
		}
	}
	return ctx.Err()
}
