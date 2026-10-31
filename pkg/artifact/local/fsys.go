package local

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

// Modes of directories and files this package creates itself.
const (
	dirMode      fs.FileMode = 0o755
	privateDir   fs.FileMode = 0o700
	privateFile  fs.FileMode = 0o600
	publicFile   fs.FileMode = 0o644
	randomBytes              = 8
	copyBufBytes             = 32 * 1024
)

// errRootMissing reports that the repository root directory does not exist yet. Read paths treat
// it as an empty repository; only writes create the root.
var errRootMissing = errors.New("repository root does not exist")

// tree is the repository root opened as an os.Root, so no operation can leave the root even
// through a symlink planted inside it. Slash-separated relative paths are converted for the host.
type tree struct {
	*os.Root

	abs string
}

// openTree opens the repository root. With create set, a missing root is created first.
func (r *Repository) openTree(create bool) (*tree, error) {
	if create {
		if err := os.MkdirAll(r.root, dirMode); err != nil {
			return nil, fmt.Errorf("%w: create repository root %q: %w", errUtils.ErrArtifactConfigInvalid, r.root, err)
		}
	}
	root, err := os.OpenRoot(r.root)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, errRootMissing
	case err != nil:
		return nil, fmt.Errorf("%w: open repository root %q: %w", errUtils.ErrArtifactConfigInvalid, r.root, err)
	}
	return &tree{Root: root, abs: r.root}, nil
}

// native converts a slash-separated relative path to the host form.
func native(rel string) string {
	return filepath.FromSlash(rel)
}

// absolute returns the absolute host path of a slash-separated relative path.
func (t *tree) absolute(rel string) string {
	return filepath.Join(t.abs, native(rel))
}

// exists reports whether a path exists without following a final symlink.
func (t *tree) exists(rel string) (bool, error) {
	_, err := t.Lstat(native(rel))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, err
	}
}

// readBounded reads a whole regular file of at most limit bytes. A missing file returns an error
// that wraps fs.ErrNotExist.
func (t *tree) readBounded(rel string, limit int64) ([]byte, error) {
	file, err := t.Open(native(rel))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return artifact.ReadBounded(file, limit)
}

// ensureParent creates the parent directories of rel.
func (t *tree) ensureParent(rel string) error {
	parent := path.Dir(rel)
	if parent == "." || parent == "" {
		return nil
	}
	return t.MkdirAll(native(parent), dirMode)
}

// writeAtomic writes data at rel through a temporary file in the managed temp directory, syncs
// it, applies the public file mode, and renames it over the destination. Readers see the old or the new file,
// never a partial one.
func (t *tree) writeAtomic(rel string, data []byte) error {
	if err := t.MkdirAll(native(layout.TempDir()), privateDir); err != nil {
		return err
	}
	if err := t.ensureParent(rel); err != nil {
		return err
	}
	suffix, err := randomSuffix()
	if err != nil {
		return err
	}
	tmp := layout.TempDir() + "/" + path.Base(rel) + "." + suffix + ".tmp"
	file, err := t.OpenFile(native(tmp), os.O_WRONLY|os.O_CREATE|os.O_EXCL, privateFile)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr == nil {
		writeErr = t.Chmod(native(tmp), publicFile)
	}
	if writeErr == nil {
		writeErr = t.Rename(native(tmp), native(rel))
	}
	if writeErr != nil {
		_ = t.Remove(native(tmp))
		return writeErr
	}
	return nil
}

func randomSuffix() (string, error) {
	buf := make([]byte, randomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// removeAll deletes a path and everything below it inside the root. Directories that lack owner
// permissions (a payload may legitimately contain mode 0555 directories) are made writable first
// so removal cannot be blocked by the artifact's own modes.
func (t *tree) removeAll(rel string) error {
	if ok, err := t.exists(rel); err != nil || !ok {
		return err
	}
	t.makeWritable(rel)
	return t.RemoveAll(native(rel))
}

// makeWritable gives every directory below rel owner access. Failures are ignored: removal reports
// the real error.
func (t *tree) makeWritable(rel string) {
	info, err := t.Lstat(native(rel))
	if err != nil || !info.IsDir() {
		return
	}
	_ = t.Chmod(native(rel), info.Mode().Perm()|privateDir)
	children, err := fs.ReadDir(t.FS(), filepath.ToSlash(native(rel)))
	if err != nil {
		return
	}
	for _, child := range children {
		if child.IsDir() {
			t.makeWritable(rel + "/" + child.Name())
		}
	}
}

// removeEmptyParents removes the now-empty parent directories of rel up to, never including, the
// root. A non-empty directory stops the walk without error.
func (t *tree) removeEmptyParents(rel string) {
	for parent := path.Dir(rel); parent != "." && parent != "" && !layout.InManagedNamespace(parent); parent = path.Dir(parent) {
		if err := t.Remove(native(parent)); err != nil {
			return
		}
	}
}

// copyVerified copies src to dst, enforcing the declared size and SHA-256 digest. It never writes
// more than size bytes plus the one extra byte needed to detect an oversize source.
func copyVerified(dst io.Writer, src io.Reader, want digest.Digest, size int64) error {
	hasher := want.Algorithm().Hash()
	written, err := io.CopyBuffer(io.MultiWriter(dst, hasher), io.LimitReader(src, size+1), make([]byte, copyBufBytes))
	switch {
	case err != nil:
		return err
	case written != size:
		return fmt.Errorf("%w: blob is %d bytes; its descriptor declares %d", errUtils.ErrArtifactIntegrityFailed, written, size)
	}
	if got := digest.NewDigest(want.Algorithm(), hasher); got != want {
		return fmt.Errorf("%w: blob digest %s does not match its descriptor %s", errUtils.ErrArtifactIntegrityFailed, got, want)
	}
	return nil
}

// stageFile creates rel exclusively in the tree and copies the verified blob into it.
func (t *tree) stageFile(rel string, src io.Reader, want digest.Digest, size int64) error {
	if err := t.ensureParent(rel); err != nil {
		return err
	}
	file, err := t.OpenFile(native(rel), os.O_WRONLY|os.O_CREATE|os.O_EXCL, privateFile)
	if err != nil {
		return err
	}
	copyErr := copyVerified(file, src, want, size)
	if copyErr == nil {
		copyErr = file.Sync()
	}
	if closeErr := file.Close(); copyErr == nil {
		copyErr = closeErr
	}
	return copyErr
}

// writeNew creates rel exclusively with data and mode.
func (t *tree) writeNew(rel string, data []byte, mode fs.FileMode) error {
	if err := t.ensureParent(rel); err != nil {
		return err
	}
	file, err := t.OpenFile(native(rel), os.O_WRONLY|os.O_CREATE|os.O_EXCL, privateFile)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr == nil {
		writeErr = t.Chmod(native(rel), mode)
	}
	return writeErr
}

// moveAside renames rel to dest when rel exists, creating the parent of dest.
func (t *tree) moveAside(rel, dest string) error {
	ok, err := t.exists(rel)
	if err != nil || !ok {
		return err
	}
	if err := t.ensureParent(dest); err != nil {
		return err
	}
	return t.Rename(native(rel), native(dest))
}
