package filesystem

import (
	"context"
	"errors"
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// WriteOptions controls file creation. Mode applies only to newly created files.
type WriteOptions struct {
	Mode      os.FileMode
	Overwrite bool
}

// WriteFile writes exact bytes without creating parent directories. By default it refuses
// to replace an existing path; overwrites preserve existing permissions and follow symlinks.
func WriteFile(ctx context.Context, path string, content []byte, options WriteOptions) error {
	defer perf.Track(nil, "filesystem.WriteFile")()

	if err := ctx.Err(); err != nil {
		return err
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if options.Overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	file, err := os.OpenFile(path, flags, options.Mode)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrWriteFile, err)
	}
	_, writeErr := file.Write(content)
	if err = errors.Join(writeErr, file.Close()); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrWriteFile, err)
	}
	return nil
}
