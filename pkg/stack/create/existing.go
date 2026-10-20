package create

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	cockroach "github.com/cockroachdb/errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/engine"
)

// existingFileDetail pulls the rendered destination out of the renderer's existing-file explanation.
var existingFileDetail = regexp.MustCompile("File already exists: `([^`]+)`")

// rejectExisting fails when the manifest is already on disk. Stack create never overwrites or merges, so it
// reports that directly instead of letting the shared renderer suggest flags this command does not have.
func rejectExisting(root, file string) error {
	if _, err := os.Lstat(filepath.Join(root, file)); err == nil {
		return existingFileError(file)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %w", errUtils.ErrReadFile, err)
	}
	return nil
}

func existingFileError(file string) error {
	return errUtils.Build(fmt.Errorf("%w: %s already exists; stack create never overwrites files", errUtils.ErrFileExists, filepath.ToSlash(file))).
		WithHint("Edit the existing manifest with `atmos stack config set`").
		WithHint("Or delete the file first and run the command again").
		Err()
}

// translateRenderError replaces the renderer's overwrite guidance (--force, --update) for any other output
// the scaffold would write over an existing file.
func translateRenderError(err error) error {
	if !errors.Is(err, errUtils.ErrFileExists) {
		return err
	}
	for _, detail := range cockroach.GetAllDetails(err) {
		if match := existingFileDetail.FindStringSubmatch(detail); match != nil && !strings.Contains(match[1], "{{") {
			return existingFileError(match[1])
		}
	}
	return errUtils.Build(fmt.Errorf("%w: a file the scaffold writes already exists; stack create never overwrites files", errUtils.ErrFileExists)).
		WithHint("Delete or move the existing file, or choose a scaffold that writes elsewhere").
		Err()
}

// staleRecordPath returns the generation record's path when a record is already on disk. The manifest was
// verified absent before rendering, so an existing record belongs to a stack that has since been deleted.
func staleRecordPath(root string, record *engine.File) (string, error) {
	if _, err := os.Lstat(filepath.Join(root, record.Path)); err == nil {
		return record.Path, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%w: %w", errUtils.ErrReadFile, err)
	}
	return "", nil
}

// withoutPath returns files minus the entry at path (a no-op for an empty path).
func withoutPath(files []engine.File, path string) []engine.File {
	if path == "" {
		return files
	}
	kept := make([]engine.File, 0, len(files))
	for _, file := range files {
		if filepath.Clean(file.Path) != filepath.Clean(path) {
			kept = append(kept, file)
		}
	}
	return kept
}

// stageStaleRecord keeps the superseded record until creation commits. Restoration
// uses a hard link so a concurrently created destination is never overwritten.
func (p *Plan) stageStaleRecord() (func(bool) error, error) {
	noop := func(bool) error { return nil }
	if p.staleRecord == "" {
		return noop, nil
	}
	path := filepath.Join(p.Root, p.staleRecord)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return noop, nil
	}
	if err != nil {
		return noop, fmt.Errorf("%w: %s: %w", errUtils.ErrFileWrite, filepath.ToSlash(path), err)
	}
	if !info.Mode().IsRegular() {
		return noop, fmt.Errorf("%w: %s is not a regular generation record", errUtils.ErrFileWrite, filepath.ToSlash(path))
	}
	backup, err := os.CreateTemp(filepath.Dir(path), ".stack-record-*")
	if err != nil {
		return noop, fmt.Errorf("%w: %s: %w", errUtils.ErrFileWrite, filepath.ToSlash(path), err)
	}
	if err := backup.Close(); err != nil {
		return noop, errors.Join(err, os.Remove(backup.Name()))
	}
	if err := os.Rename(path, backup.Name()); err != nil {
		return noop, errors.Join(fmt.Errorf("%w: %s: %w", errUtils.ErrFileWrite, filepath.ToSlash(path), err), os.Remove(backup.Name()))
	}
	return func(committed bool) error { return finishStaleRecord(path, backup.Name(), committed) }, nil
}

func finishStaleRecord(path, backup string, committed bool) error {
	if !committed {
		if err := os.Link(backup, path); err != nil {
			return fmt.Errorf("%w: restore %s from retained backup %s: %w", errUtils.ErrFileWrite, filepath.ToSlash(path), filepath.ToSlash(backup), err)
		}
	}
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("%w: remove generation record backup %s: %w", errUtils.ErrFileWrite, backup, err)
	}
	return nil
}
