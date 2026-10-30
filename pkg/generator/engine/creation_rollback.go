package engine

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
)

// createdFile records ownership and the bytes written by this attempt, including
// a short write. Empty directories are retained; hooks may have added files there.
type createdFile struct {
	path string
	info os.FileInfo
	hash [sha256.Size]byte
}

func writeCreationFile(path string, file *File) (*createdFile, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, file.Permissions)
	if err != nil {
		return nil, err
	}
	n, writeErr := f.Write([]byte(file.Content))
	info, statErr := f.Stat()
	owned := &createdFile{path: path, info: info, hash: sha256.Sum256([]byte(file.Content[:n]))}
	return owned, errors.Join(writeErr, statErr, f.Close())
}

func rollbackCreation(files []*createdFile) error {
	var errs []error
	for i := len(files) - 1; i >= 0; i-- {
		if err := files[i].remove(); err != nil {
			errs = append(errs, fmt.Errorf("%w: rollback left %s: %w", errUtils.ErrFileWrite, files[i].path, err))
		}
	}
	return errors.Join(errs...)
}

// remove refuses to delete replaced or edited files. Lstat also rejects a
// replacement symlink even when its target happens to contain identical bytes.
func (f *createdFile) remove() error {
	info, err := os.Lstat(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if f.info == nil || !os.SameFile(f.info, info) || info.Mode() != f.info.Mode() || !info.ModTime().Equal(f.info.ModTime()) {
		return fmt.Errorf("%w: file changed since creation; retained for manual recovery", errUtils.ErrFileWrite)
	}
	content, err := os.ReadFile(f.path)
	if err != nil {
		return err
	}
	if sha256.Sum256(content) != f.hash {
		return fmt.Errorf("%w: file contents changed; retained for manual recovery", errUtils.ErrFileWrite)
	}
	return os.Remove(f.path)
}
