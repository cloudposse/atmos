package materialize

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// Names of the distribution members generated inside a packaged representation.
const (
	// The distribution directory.
	managedDir = ".atmos/artifact"

	// The parent of the distribution directory.
	managedParent = ".atmos"

	// The generated manifest.
	managedManifest = "manifest.json"

	// The longest symlink target read from a zip member.
	maxLinkTargetBytes = 4096

	// The chunk size of context-aware copies.
	copyChunkSize = 32 * 1024
)

// fillArchive extracts the single payload stream of a tar, tgz, or zip representation.
func fillArchive(ctx context.Context, t *tree, content *artifact.Content, scratch string) error {
	var payload *artifact.ContentStream
	for index := range content.Streams {
		stream := &content.Streams[index]
		if stream.Role != artifact.BlobPayload {
			continue
		}
		if payload != nil {
			return fmt.Errorf("%w: an archive representation must have exactly one payload", errUtils.ErrArtifactIntegrityFailed)
		}
		payload = stream
	}
	if payload == nil || payload.Reader == nil {
		return fmt.Errorf("%w: the archive payload is missing", errUtils.ErrArtifactIntegrityFailed)
	}
	checker, err := newManagedChecker(content.Record)
	if err != nil {
		return err
	}
	if content.Representation.Packaging == artifact.PackagingZip {
		return fillZip(ctx, t, payload.Reader, scratch, checker)
	}
	return fillTar(ctx, t, payload.Reader, content.Representation.Packaging == artifact.PackagingTgz, checker)
}

func fillTar(ctx context.Context, t *tree, reader io.Reader, gzipped bool, checker *managedChecker) error {
	if gzipped {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return fmt.Errorf("%w: gzip: %w", errUtils.ErrArtifactIntegrityFailed, err)
		}
		defer gz.Close()
		reader = gz
	}
	tr := tar.NewReader(reader)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			// Drain so the verifying payload reader checks its full digest at end of stream.
			_, err = io.Copy(io.Discard, reader)
			return err
		}
		if err != nil {
			return fmt.Errorf("%w: tar: %w", errUtils.ErrArtifactIntegrityFailed, err)
		}
		if err := applyTarHeader(t, tr, header, checker); err != nil {
			return err
		}
	}
}

func applyTarHeader(t *tree, tr *tar.Reader, header *tar.Header, checker *managedChecker) error {
	name := strings.TrimSuffix(path.Clean(header.Name), "/")
	switch header.Typeflag {
	case tar.TypeDir:
		return applyMember(t, checker, &archiveMember{name: name, kind: memberDir, size: header.Size})
	case tar.TypeSymlink:
		return applyMember(t, checker, &archiveMember{name: name, kind: memberLink, target: header.Linkname})
	case tar.TypeReg:
		return applyMember(t, checker, &archiveMember{name: name, kind: memberFile, size: header.Size, body: tr})
	default:
		return fmt.Errorf("%w: archive member %q has unsupported type %q (hard links, devices, and special files are rejected)", errUtils.ErrArtifactPackagingInvalid, header.Name, header.Typeflag)
	}
}

func fillZip(ctx context.Context, t *tree, reader io.Reader, scratch string, checker *managedChecker) error {
	spool, err := os.CreateTemp(scratch, "archive-*.zip")
	if err != nil {
		return fmt.Errorf("%w: spool archive: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	defer os.Remove(spool.Name())
	defer spool.Close()
	size, err := copyContext(ctx, spool, reader)
	if err != nil {
		return fmt.Errorf("%w: spool archive: %w", errUtils.ErrArtifactIntegrityFailed, err)
	}
	zr, err := zip.NewReader(spool, size)
	if err != nil {
		return fmt.Errorf("%w: zip: %w", errUtils.ErrArtifactIntegrityFailed, err)
	}
	for _, file := range zr.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := applyZipFile(t, file, checker); err != nil {
			return err
		}
	}
	return nil
}

func applyZipFile(t *tree, file *zip.File, checker *managedChecker) error {
	name := strings.TrimSuffix(path.Clean(file.Name), "/")
	mode := file.Mode()
	switch {
	case mode.IsDir():
		return applyMember(t, checker, &archiveMember{name: name, kind: memberDir})
	case mode&fs.ModeSymlink != 0:
		target, err := readZipText(file)
		if err != nil {
			return err
		}
		return applyMember(t, checker, &archiveMember{name: name, kind: memberLink, target: target})
	case mode.IsRegular():
		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("%w: zip member %q: %w", errUtils.ErrArtifactIntegrityFailed, file.Name, err)
		}
		defer rc.Close()
		return applyMember(t, checker, &archiveMember{name: name, kind: memberFile, size: file.FileInfo().Size(), body: rc})
	default:
		return fmt.Errorf("%w: archive member %q has an unsupported type", errUtils.ErrArtifactPackagingInvalid, file.Name)
	}
}

func readZipText(file *zip.File) (string, error) {
	rc, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("%w: zip member %q: %w", errUtils.ErrArtifactIntegrityFailed, file.Name, err)
	}
	defer rc.Close()
	data, err := artifact.ReadBounded(rc, maxLinkTargetBytes)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type memberKind int

const (
	memberFile memberKind = iota
	memberDir
	memberLink
)

// archiveMember is one decoded archive member.
type archiveMember struct {
	name   string
	kind   memberKind
	target string
	size   int64
	body   io.Reader
}

// applyMember validates one archive member against the inventory (or the managed directory) and
// writes it through the tree.
func applyMember(t *tree, checker *managedChecker, m *archiveMember) error {
	if err := validateMemberName(m.name); err != nil {
		return err
	}
	if checker.owns(m.name, t) {
		return checker.check(m)
	}
	entry, err := t.entryFor(m.name)
	if err != nil {
		return err
	}
	if !m.matches(entry) {
		return fmt.Errorf("%w: archive member %q does not match its inventory entry (%s)", errUtils.ErrArtifactIntegrityFailed, m.name, entry.Type)
	}
	switch entry.Type {
	case artifact.EntryDirectory:
		return t.mkdir(entry)
	case artifact.EntrySymlink:
		return t.symlink(entry)
	default:
		return t.writeFile(entry, m.body)
	}
}

// matches reports whether the member has the kind, link target, and size its inventory entry declares.
func (m *archiveMember) matches(entry *artifact.Entry) bool {
	switch entry.Type {
	case artifact.EntryDirectory:
		return m.kind == memberDir
	case artifact.EntrySymlink:
		return m.kind == memberLink && m.target == entry.Target
	case artifact.EntryFile:
		return m.kind == memberFile && m.size == entry.Size
	default:
		return false
	}
}

// validateMemberName rejects traversal, absolute, backslash, and empty names. Managed names pass
// through the same check, so even the reserved directory cannot be used to escape.
func validateMemberName(name string) error {
	if name == "." || name == "" || name == ".." || strings.HasPrefix(name, "../") || filepath.IsAbs(name) {
		return fmt.Errorf("%w: archive member %q escapes the destination", errUtils.ErrArtifactUnsafePath, name)
	}
	return artifact.ValidatePayloadPath(name)
}

// managedChecker verifies the generated distribution members of a packaged representation
// (manifest.json plus embedded documents) without ever writing them to the user's tree.
type managedChecker struct {
	manifest  digest.Digest
	documents map[string]artifact.Document
	seen      map[string]bool
}

func newManagedChecker(record *artifact.Record) (*managedChecker, error) {
	canonical, err := record.Canonical()
	if err != nil {
		return nil, err
	}
	checker := &managedChecker{manifest: digest.FromBytes(canonical), documents: map[string]artifact.Document{}, seen: map[string]bool{}}
	for _, doc := range record.Documents {
		checker.documents[managedDir+"/"+strings.ToUpper(string(doc.Role))+".md"] = doc
	}
	return checker, nil
}

// owns reports whether name belongs to the managed directory and the inventory does not claim it.
func (m *managedChecker) owns(name string, t *tree) bool {
	if _, claimed := t.inventory[name]; claimed {
		return false
	}
	return name == managedParent || name == managedDir || strings.HasPrefix(name, managedDir+"/")
}

// check verifies one managed member without ever writing it into the user's tree.
func (m *managedChecker) check(member *archiveMember) error {
	name := member.name
	if name == managedParent || name == managedDir {
		if member.kind != memberDir {
			return fmt.Errorf("%w: %q must be a directory", errUtils.ErrArtifactPackagingInvalid, name)
		}
		return nil
	}
	want, ok := m.expected(name)
	if member.kind != memberFile || m.seen[name] || !ok {
		return fmt.Errorf("%w: unexpected managed member %q", errUtils.ErrArtifactPackagingInvalid, name)
	}
	m.seen[name] = true
	return verifyManagedFile(member, want)
}

// verifyManagedFile reads a managed file within its size bound and compares it to the Record.
func verifyManagedFile(member *archiveMember, want digest.Digest) error {
	limit := artifact.MaxDocumentSize
	if member.name == managedDir+"/"+managedManifest {
		limit = artifact.MaxRecordSize
	}
	if member.size > limit {
		return fmt.Errorf("%w: managed member %q is %d bytes", errUtils.ErrArtifactTooLarge, member.name, member.size)
	}
	data, err := artifact.ReadBounded(member.body, limit)
	if err != nil {
		return err
	}
	if digest.FromBytes(data) != want {
		return fmt.Errorf("%w: embedded %q does not match the Record", errUtils.ErrArtifactIntegrityFailed, member.name)
	}
	return nil
}

func (m *managedChecker) expected(name string) (digest.Digest, bool) {
	if name == managedDir+"/"+managedManifest {
		return m.manifest, true
	}
	doc, ok := m.documents[name]
	return doc.Digest, ok
}

// copyContext copies src to dst in chunks and stops as soon as ctx is canceled.
func copyContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buffer := make([]byte, copyChunkSize)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, readErr := src.Read(buffer)
		if n > 0 {
			written, writeErr := dst.Write(buffer[:n])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
		}
		if readErr == io.EOF {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}
