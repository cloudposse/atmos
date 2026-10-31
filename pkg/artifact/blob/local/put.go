package local

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/perf"
)

// staged is a fully written temporary object waiting to be renamed into place.
type staged struct {
	tempPath  string
	attrsTemp string
	size      int64
	etag      string
}

// Put implements [blob.Store]. The body is streamed to a temporary file and renamed into place,
// so a failed or canceled put never leaves a partial object.
func (s *Store) Put(ctx context.Context, key string, body io.Reader, opts blob.PutOptions) (*blob.ObjectInfo, error) {
	defer perf.Track(nil, "blob.local.Put")()

	if err := s.validatePut(ctx, key, opts); err != nil {
		return nil, err
	}
	target := s.path(key)
	pending, err := s.stage(ctx, key, target, body, opts)
	if err != nil {
		return nil, err
	}
	defer pending.discard()

	release, err := s.guard(ctx, blob.OpPut, key, opts.Condition.IsConditional())
	if err != nil {
		return nil, err
	}
	defer release()

	if err := s.checkCondition(key, target, opts.Condition); err != nil {
		return nil, err
	}
	if err := s.commit(key, target, pending, fromPut(opts).empty()); err != nil {
		return nil, err
	}
	return s.statCommitted(key, target, pending)
}

func (s *Store) validatePut(ctx context.Context, key string, opts blob.PutOptions) error {
	if err := s.validate(blob.OpPut, key); err != nil {
		return err
	}
	if err := blob.ValidateTags(opts.Tags); err != nil {
		return err
	}
	if opts.Condition.Kind == blob.ConditionIfMatch && opts.Condition.ETag == "" {
		return blob.NewError(blob.KindInvalid, blob.OpPut, key, fmt.Errorf("%w: if-match requires an ETag", errUtils.ErrBlobInvalid))
	}
	return checkContext(ctx, blob.OpPut, key)
}

// stage writes the body (and attribute file, when needed) to temporary files next to target.
func (s *Store) stage(ctx context.Context, key, target string, body io.Reader, opts blob.PutOptions) (*staged, error) {
	file, err := createTemp(filepath.Dir(target), s.dirPerm, s.filePerm)
	if err != nil {
		return nil, s.classifyCreate(key, target, err)
	}
	pending := &staged{tempPath: file.Name()}
	hash := sha256.New()
	written, copyErr := copyBody(ctx, io.MultiWriter(file, hash), body, opts.Size)
	closeErr := file.Close()
	if err := firstError(copyErr, closeErr); err != nil {
		pending.discard()
		return nil, classifyCopy(key, err)
	}
	pending.size = written
	pending.etag = contentETag(hash.Sum(nil))
	if err := s.stageAttrs(pending, target, fromPut(opts)); err != nil {
		pending.discard()
		return nil, wrapIO(blob.OpPut, key, err)
	}
	return pending, nil
}

// stageAttrs writes the attribute file to a temporary path when any attribute is set.
func (s *Store) stageAttrs(pending *staged, target string, a attrs) error {
	if a.empty() {
		return nil
	}
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	file, err := createTemp(filepath.Dir(target), s.dirPerm, s.filePerm)
	if err != nil {
		return err
	}
	pending.attrsTemp = file.Name()
	_, writeErr := file.Write(data)
	return firstError(writeErr, file.Close())
}

// copyBody streams body into dst, enforcing a declared size and honoring cancellation.
func copyBody(ctx context.Context, dst io.Writer, body io.Reader, size int64) (int64, error) {
	reader := io.Reader(&ctxReader{ctx: ctx, r: body})
	if size >= 0 {
		reader = io.LimitReader(reader, size+1)
	}
	written, err := io.Copy(dst, reader)
	if err != nil {
		return written, err
	}
	if size >= 0 && written != size {
		return written, fmt.Errorf("%w: body is %d bytes, declared %d", errUtils.ErrBlobInvalid, written, size)
	}
	return written, nil
}

// checkCondition evaluates the put condition against the current object. Callers hold the lock.
func (s *Store) checkCondition(key, target string, cond blob.Condition) error {
	if !cond.IsConditional() {
		return nil
	}
	state, err := inspect(target)
	if err != nil {
		return wrapIO(blob.OpPut, key, err)
	}
	switch cond.Kind {
	case blob.ConditionIfAbsent:
		if state.exists {
			return preconditionFailed(blob.OpPut, key, "object already exists")
		}
	case blob.ConditionIfMatch:
		return matchETag(blob.OpPut, key, state, cond.ETag)
	}
	return nil
}

// fileState is the current state of the file backing a key.
type fileState struct {
	path   string
	info   os.FileInfo
	exists bool
}

// inspect reports whether a regular file exists at path. A missing path is not an error.
func inspect(path string) (fileState, error) {
	info, err := os.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fileState{}, err
	}
	return fileState{path: path, info: info, exists: err == nil && info.Mode().IsRegular()}, nil
}

// matchETag fails with NotFound when the object is missing and with a precondition failure when
// its ETag differs from want.
func matchETag(op, key string, state fileState, want string) error {
	if !state.exists {
		return blob.NewError(blob.KindNotFound, op, key, nil)
	}
	current, err := etagOf(state.path, state.info)
	if err != nil {
		return wrapIO(op, key, err)
	}
	if current != want {
		return preconditionFailed(op, key, "ETag does not match")
	}
	return nil
}

// commit moves the staged files into place: attributes first, so an object is only ever visible
// together with its attributes or with stale ones until the data rename lands.
func (s *Store) commit(key, target string, pending *staged, noAttrs bool) error {
	switch {
	case pending.attrsTemp != "":
		if err := renameReplace(pending.attrsTemp, attrsPath(target)); err != nil {
			return wrapIO(blob.OpPut, key, err)
		}
		pending.attrsTemp = ""
	case noAttrs:
		if err := os.Remove(attrsPath(target)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return wrapIO(blob.OpPut, key, err)
		}
	}
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		return blob.NewError(blob.KindInvalid, blob.OpPut, key, fmt.Errorf("%w: key is also a prefix of other objects", errUtils.ErrBlobInvalid))
	}
	if err := renameReplace(pending.tempPath, target); err != nil {
		return wrapIO(blob.OpPut, key, err)
	}
	pending.tempPath = ""
	return nil
}

// statCommitted describes the object that was just written.
func (s *Store) statCommitted(key, target string, pending *staged) (*blob.ObjectInfo, error) {
	info, err := os.Stat(target)
	if err != nil {
		return nil, wrapIO(blob.OpPut, key, err)
	}
	etag := pending.etag
	if info.Size() > contentETagLimit {
		etag = weakETag(info)
	}
	return &blob.ObjectInfo{Key: key, Size: pending.size, ETag: etag, LastModified: info.ModTime()}, nil
}

// discard removes any temporary files that were not committed.
func (p *staged) discard() {
	if p.tempPath != "" {
		_ = os.Remove(p.tempPath)
	}
	if p.attrsTemp != "" {
		_ = os.Remove(p.attrsTemp)
	}
}
