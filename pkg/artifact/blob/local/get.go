package local

import (
	"context"
	"fmt"
	"io"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/perf"
)

// rangedBody closes the file while reading only a window of it.
type rangedBody struct {
	io.Reader
	file *os.File
}

func (r *rangedBody) Close() error {
	defer perf.Track(nil, "local.rangedBody.Close")()

	return r.file.Close()
}

// Get implements [blob.Store].
func (s *Store) Get(ctx context.Context, key string, opts blob.GetOptions) (*blob.Object, error) {
	defer perf.Track(nil, "blob.local.Get")()

	if err := s.validate(blob.OpGet, key); err != nil {
		return nil, err
	}
	if opts.Offset < 0 {
		return nil, blob.NewError(blob.KindInvalid, blob.OpGet, key, fmt.Errorf("%w: negative offset", errUtils.ErrBlobInvalid))
	}
	if err := checkContext(ctx, blob.OpGet, key); err != nil {
		return nil, err
	}
	target := s.path(key)
	file, err := os.Open(target)
	if err != nil {
		return nil, wrapIO(blob.OpGet, key, err)
	}
	info, err := s.describe(key, target, file, false)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	body, err := openBody(file, info.Size, opts)
	if err != nil {
		_ = file.Close()
		return nil, blob.NewError(blob.KindInvalid, blob.OpGet, key, err)
	}
	return &blob.Object{ObjectInfo: *info, Body: body}, nil
}

// openBody positions file for a full or ranged read.
func openBody(file *os.File, size int64, opts blob.GetOptions) (io.ReadCloser, error) {
	if !opts.IsRange() {
		return file, nil
	}
	if opts.Offset > size {
		return nil, fmt.Errorf("%w: offset %d is past the end of a %d byte object", errUtils.ErrBlobInvalid, opts.Offset, size)
	}
	if _, err := file.Seek(opts.Offset, io.SeekStart); err != nil {
		return nil, err
	}
	var reader io.Reader = file
	if opts.Length > 0 {
		reader = io.LimitReader(file, opts.Length)
	}
	return &rangedBody{Reader: reader, file: file}, nil
}

// Stat implements [blob.Store].
func (s *Store) Stat(ctx context.Context, key string, opts blob.StatOptions) (*blob.ObjectInfo, error) {
	defer perf.Track(nil, "blob.local.Stat")()

	if err := s.validate(blob.OpStat, key); err != nil {
		return nil, err
	}
	if err := checkContext(ctx, blob.OpStat, key); err != nil {
		return nil, err
	}
	target := s.path(key)
	file, err := os.Open(target)
	if err != nil {
		return nil, wrapIO(blob.OpStat, key, err)
	}
	defer file.Close()
	return s.describe(key, target, file, opts.IncludeTags)
}

// describe builds ObjectInfo for an open regular file. A directory is not an object.
func (s *Store) describe(key, target string, file *os.File, withTags bool) (*blob.ObjectInfo, error) {
	stat, err := file.Stat()
	if err != nil {
		return nil, wrapIO(blob.OpStat, key, err)
	}
	if !stat.Mode().IsRegular() {
		return nil, blob.NewError(blob.KindNotFound, blob.OpStat, key, fmt.Errorf("%w: key is a prefix, not an object", errUtils.ErrBlobNotFound))
	}
	etag, err := etagOf(target, stat)
	if err != nil {
		return nil, wrapIO(blob.OpStat, key, err)
	}
	a, err := readAttrs(target)
	if err != nil {
		return nil, blob.NewError(blob.KindOther, blob.OpStat, key, fmt.Errorf("read attributes: %w", err))
	}
	info := &blob.ObjectInfo{Key: key, Size: stat.Size(), ETag: etag, ContentType: a.ContentType, LastModified: stat.ModTime(), Metadata: a.Metadata}
	if withTags {
		info.Tags = a.Tags
	}
	return info, nil
}
