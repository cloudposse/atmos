package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

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
	root, err := s.openRoot(false)
	if err != nil {
		return nil, wrapIO(blob.OpGet, key, err)
	}
	defer root.Close()
	release, err := s.guard(ctx, blob.OpGet, key, false)
	if err != nil {
		return nil, err
	}
	defer release()
	target := filepath.FromSlash(key)
	file, err := rootOpenFile(root, target)
	if err != nil {
		return nil, wrapIO(blob.OpGet, key, err)
	}
	info, err := s.describe(ctx, root, describeTarget{op: blob.OpGet, key: key, file: file}, describeOptions{skipETag: opts.SkipETag})
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
	root, err := s.openRoot(false)
	if err != nil {
		return nil, wrapIO(blob.OpStat, key, err)
	}
	defer root.Close()
	release, err := s.guard(ctx, blob.OpStat, key, false)
	if err != nil {
		return nil, err
	}
	defer release()
	target := filepath.FromSlash(key)
	file, err := rootOpenFile(root, target)
	if err != nil {
		return nil, wrapIO(blob.OpStat, key, err)
	}
	defer file.Close()
	return s.describe(ctx, root, describeTarget{op: blob.OpStat, key: key, file: file}, describeOptions{withTags: opts.IncludeTags, skipETag: opts.SkipETag})
}

// describeTarget is the open file a description is built for.
type describeTarget struct {
	op   string
	key  string
	file *os.File
}

// describeOptions select the optional parts of a description.
type describeOptions struct {
	withTags bool

	// skipETag leaves the ETag empty so a metadata check does not hash the whole object.
	skipETag bool
}

// describe builds ObjectInfo for an open regular file. A directory is not an object.
func (s *Store) describe(ctx context.Context, root *os.Root, target describeTarget, opts describeOptions) (*blob.ObjectInfo, error) {
	op, key, file := target.op, target.key, target.file
	stat, err := file.Stat()
	if err != nil {
		return nil, wrapIO(op, key, err)
	}
	if !stat.Mode().IsRegular() {
		return nil, blob.NewError(blob.KindNotFound, op, key, fmt.Errorf("%w: key is a prefix, not an object", errUtils.ErrBlobNotFound))
	}
	var etag string
	if !opts.skipETag {
		if etag, err = etagOpen(ctx, file); err != nil {
			return nil, wrapIO(op, key, err)
		}
	}
	var a attrs
	if !s.plainFiles {
		if a, err = readAttrs(root, filepath.FromSlash(key)); err != nil {
			return nil, blob.NewError(blob.KindOther, op, key, fmt.Errorf("read attributes: %w", err))
		}
	}
	info := &blob.ObjectInfo{Key: key, Size: stat.Size(), ETag: etag, ContentType: a.ContentType, LastModified: stat.ModTime(), Metadata: a.Metadata}
	if opts.withTags {
		info.Tags = a.Tags
	}
	return info, nil
}
