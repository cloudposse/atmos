package local

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
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
	preserve  bool
}

// Put implements [blob.Store]. The body is streamed to a temporary file and renamed into place,
// so a failed or canceled put never leaves a partial object.
func (s *Store) Put(ctx context.Context, key string, body io.Reader, opts blob.PutOptions) (*blob.ObjectInfo, error) {
	defer perf.Track(nil, "blob.local.Put")()

	if body == nil {
		return nil, blob.NewError(blob.KindInvalid, blob.OpPut, key, fmt.Errorf("%w: nil body", errUtils.ErrBlobInvalid))
	}
	if err := s.validatePut(ctx, key, opts); err != nil {
		return nil, err
	}
	root, err := s.openRoot(true)
	if err != nil {
		return nil, wrapIO(blob.OpPut, key, err)
	}
	defer root.Close()
	target := filepath.FromSlash(key)
	pending, err := s.stage(ctx, root, key, body, opts)
	if err != nil {
		return nil, err
	}
	defer pending.discard(root)

	release, err := s.guard(ctx, blob.OpPut, key, opts.Condition.IsConditional())
	if err != nil {
		return nil, err
	}
	defer release()

	if err := s.checkCondition(ctx, root, key, target, opts.Condition); err != nil {
		return nil, err
	}
	if err := s.commit(root, key, target, pending, fromPut(opts).empty()); err != nil {
		return nil, err
	}
	return s.statCommitted(root, key, target, pending)
}

func (s *Store) validatePut(ctx context.Context, key string, opts blob.PutOptions) error {
	if err := s.validate(blob.OpPut, key); err != nil {
		return err
	}
	if err := s.Capabilities().CheckPut(storeName, key, opts.Condition); err != nil {
		return err
	}
	if opts.Size < blob.UnknownSize || opts.Size == math.MaxInt64 {
		return blob.NewError(blob.KindInvalid, blob.OpPut, key, fmt.Errorf("%w: invalid or unverifiable size %d", errUtils.ErrBlobInvalid, opts.Size))
	}
	if err := blob.ValidateTags(opts.Tags); err != nil {
		return err
	}
	if opts.Condition.Kind == blob.ConditionIfMatch && opts.Condition.ETag == "" {
		return blob.NewError(blob.KindInvalid, blob.OpPut, key, fmt.Errorf("%w: if-match requires an ETag", errUtils.ErrBlobInvalid))
	}
	if s.plainFiles && !fromPut(opts).empty() {
		return blob.NewError(blob.KindInvalid, blob.OpPut, key, fmt.Errorf("%w: plain-file stores cannot store content type, metadata, or tags", errUtils.ErrBlobInvalid))
	}
	return checkContext(ctx, blob.OpPut, key)
}

// stage writes the body (and attribute file, when needed) to temporary files next to target.
func (s *Store) stage(ctx context.Context, root *os.Root, key string, body io.Reader, opts blob.PutOptions) (*staged, error) {
	target := filepath.FromSlash(key)
	file, name, err := rootCreateTemp(root, filepath.Dir(target), s.dirPerm, s.filePerm)
	if err != nil {
		return nil, s.classifyCreate(root, key, target, err)
	}
	pending := &staged{tempPath: name}
	if err := keepDestinationMode(root, file, target); err != nil {
		_ = file.Close()
		pending.discard(root)
		return nil, wrapIO(blob.OpPut, key, err)
	}
	hash := sha256.New()
	written, copyErr := copyBody(ctx, io.MultiWriter(file, hash), body, opts.Size)
	closeErr := file.Close()
	if err := firstError(copyErr, closeErr); err != nil {
		pending.discard(root)
		return nil, classifyCopy(key, err)
	}
	pending.size = written
	pending.etag = contentETag(hash.Sum(nil))
	if err := s.stageAttrs(root, pending, target, fromPut(opts)); err != nil {
		pending.discard(root)
		return nil, wrapIO(blob.OpPut, key, err)
	}
	return pending, nil
}

// keepDestinationMode gives the staged file the permission bits of the regular file it will
// replace, so an overwrite does not widen (or narrow) a mode that was set by hand. It runs before
// any content is written, so restricted data is never exposed under the default mode. A missing
// or non-regular destination keeps the store's default mode.
func keepDestinationMode(root *os.Root, file *os.File, target string) error {
	want, ok := destinationMode(root, target)
	if !ok {
		return nil
	}
	created, err := file.Stat()
	if err != nil {
		return err
	}
	if created.Mode().Perm() == want {
		return nil
	}
	if err := file.Chmod(want); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		return err
	}
	return nil
}

// destinationMode returns the permission bits of the regular file at target. It reports false
// when there is no such file; a destination that cannot be inspected fails at commit instead.
func destinationMode(root *os.Root, target string) (os.FileMode, bool) {
	info, err := root.Stat(target)
	if err != nil || !info.Mode().IsRegular() {
		return 0, false
	}
	return info.Mode().Perm(), true
}

// stageAttrs writes the attribute file to a temporary path when any attribute is set.
func (s *Store) stageAttrs(root *os.Root, pending *staged, target string, a attrs) error {
	if a.empty() {
		return nil
	}
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	file, name, err := rootCreateTemp(root, filepath.Dir(target), s.dirPerm, s.filePerm)
	if err != nil {
		return err
	}
	pending.attrsTemp = name
	_, writeErr := file.Write(data)
	return firstError(writeErr, file.Close())
}

// copyBody streams body into dst, enforcing a declared size and honoring cancellation.
func copyBody(ctx context.Context, dst io.Writer, body io.Reader, size int64) (int64, error) {
	if size == math.MaxInt64 {
		return 0, fmt.Errorf("%w: size %d is unverifiable", errUtils.ErrBlobInvalid, size)
	}
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
func (s *Store) checkCondition(ctx context.Context, root *os.Root, key, target string, cond blob.Condition) error {
	if cond.Kind == blob.ConditionNone {
		return nil
	}
	if cond.Kind != blob.ConditionIfAbsent && cond.Kind != blob.ConditionIfMatch {
		return blob.NewError(blob.KindInvalid, blob.OpPut, key, fmt.Errorf("%w: unknown condition %d", errUtils.ErrBlobInvalid, cond.Kind))
	}
	state, err := inspect(root, target)
	if err != nil {
		return wrapIO(blob.OpPut, key, err)
	}
	switch cond.Kind {
	case blob.ConditionIfAbsent:
		if state.exists {
			return preconditionFailed(blob.OpPut, key, "object already exists")
		}
	case blob.ConditionIfMatch:
		return matchETag(ctx, root, objectRef{op: blob.OpPut, key: key}, state, cond.ETag)
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
func inspect(root *os.Root, path string) (fileState, error) {
	info, err := root.Stat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fileState{}, err
	}
	return fileState{path: path, info: info, exists: err == nil && info.Mode().IsRegular()}, nil
}

// objectRef names the object an operation addresses, for error reporting.
type objectRef struct {
	op  string
	key string
}

// matchETag fails with NotFound when the object is missing and with a precondition failure when
// its ETag differs from want.
func matchETag(ctx context.Context, root *os.Root, ref objectRef, state fileState, want string) error {
	op, key := ref.op, ref.key
	if !state.exists {
		return blob.NewError(blob.KindNotFound, op, key, nil)
	}
	current, err := etagOf(ctx, root, state.path)
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
func (s *Store) commit(root *os.Root, key, target string, pending *staged, noAttrs bool) error {
	info, err := root.Stat(target)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return wrapIO(blob.OpPut, key, err)
	}
	if err == nil && info.IsDir() {
		return blob.NewError(blob.KindInvalid, blob.OpPut, key, fmt.Errorf("%w: key is also a prefix of other objects", errUtils.ErrBlobInvalid))
	}
	if !s.plainFiles {
		return s.commitManaged(root, key, pending, noAttrs, rootRenameReplace)
	}
	return commitPlain(root, key, target, pending)
}

// commitPlain installs a staged object in a plain-files store. It never touches attribute paths:
// a name such as "a.tar.atmos-blob.json" is an independent legacy object there.
func commitPlain(root *os.Root, key, target string, pending *staged) error {
	if err := rootRenameReplace(root, pending.tempPath, target); err != nil {
		return wrapIO(blob.OpPut, key, err)
	}
	pending.tempPath = ""
	return nil
}

// statCommitted describes the object that was just written.
func (s *Store) statCommitted(root *os.Root, key, target string, pending *staged) (*blob.ObjectInfo, error) {
	info, err := root.Stat(target)
	if err != nil {
		return nil, wrapIO(blob.OpPut, key, err)
	}
	return &blob.ObjectInfo{Key: key, Size: pending.size, ETag: pending.etag, LastModified: info.ModTime()}, nil
}

// discard removes any temporary files that were not committed, then prunes the staging
// directories that this put created and left empty. Non-empty directories are kept.
func (p *staged) discard(root *os.Root) {
	if p.preserve {
		return
	}
	var dirs []string
	if p.tempPath != "" {
		_ = root.Remove(p.tempPath)
		dirs = append(dirs, filepath.Dir(p.tempPath))
	}
	if p.attrsTemp != "" {
		_ = root.Remove(p.attrsTemp)
		dirs = append(dirs, filepath.Dir(p.attrsTemp))
	}
	for _, dir := range dirs {
		rootRemoveEmptyDirs(root, dir)
	}
}
