package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// MemberKind is the type of a [Member] written by [WriteDeterministic].
type MemberKind int

// Member kinds. Devices, sockets, and other special files are never representable.
const (
	// MemberFile is a regular file whose bytes come from [Member.Open].
	MemberFile MemberKind = iota

	// MemberDirectory is a directory entry.
	MemberDirectory

	// MemberSymlink is a symbolic link to [Member.Target].
	MemberSymlink
)

// deterministicPermMask holds the permission bits a deterministic member may carry. Privileged
// bits (setuid, setgid, sticky) are always cleared.
const deterministicPermMask fs.FileMode = 0o777

// Member is one explicitly described archive entry for [WriteDeterministic]. Unlike [PackOptions]
// sources, members never touch the filesystem on their own: the caller supplies the bytes, so the
// output depends only on the declared paths, modes, and content.
type Member struct {
	// Path is the relative, slash-separated path inside the archive.
	Path string

	// Kind is file, directory, or symlink.
	Kind MemberKind

	// Mode holds the ordinary permission bits. Executable bits are preserved; other bits are
	// written as given. Privileged bits are cleared.
	Mode fs.FileMode

	// Size is the exact byte length of a file's content.
	Size int64

	// Target is a symlink's target; empty for other kinds.
	Target string

	// Open returns a reader over a file's content. The writer closes it. Nil for other kinds.
	Open func() (io.ReadCloser, error)
}

// tarEpoch is the timestamp stamped into every deterministic tar entry.
var tarEpoch = time.Unix(0, 0).UTC()

// zipEpoch is the timestamp stamped into every deterministic zip entry. 1980-01-01 is the earliest
// time the zip DOS timestamp field can represent.
var zipEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// WriteDeterministic writes members to w as a zip, tar, or tgz archive whose bytes depend only on
// the members: entries are sorted by path, timestamps are pinned to a fixed epoch, owners are
// normalized to zero and empty names, gzip headers carry no name or time, and permission bits
// (including executable bits) are preserved. Identical members always produce identical bytes for
// one Go toolchain. It does not alter the behavior of [Run]; that path keeps its own defaults.
func WriteDeterministic(w io.Writer, format string, members []Member) error {
	defer perf.Track(nil, "archive.WriteDeterministic")()

	sorted, err := sortMembers(members)
	if err != nil {
		return err
	}
	switch normalizeFormat(format) {
	case FormatTar:
		return writeDeterministicTar(w, sorted)
	case FormatTGZ:
		return writeDeterministicTGZ(w, sorted)
	case FormatZip:
		return writeDeterministicZip(w, sorted)
	default:
		return errUtils.Build(errUtils.ErrArchiveFormatNotImplemented).
			WithExplanationf("deterministic writing supports zip, tar, and tgz, not %q", format).
			WithContext("format", format).
			Err()
	}
}

// sortMembers returns a path-sorted copy and rejects duplicate paths.
func sortMembers(members []Member) ([]Member, error) {
	sorted := make([]Member, len(members))
	copy(sorted, members)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	for index := range sorted {
		if sorted[index].Path == "" || strings.HasPrefix(sorted[index].Path, "/") {
			return nil, memberError(sorted[index].Path, errUtils.ErrArchiveInvalidSubpath)
		}
		if index > 0 && sorted[index].Path == sorted[index-1].Path {
			return nil, memberError(sorted[index].Path, errUtils.ErrArchiveWriteFailed)
		}
	}
	return sorted, nil
}

func memberError(path string, sentinel error) error {
	return fmt.Errorf("%w: member %q is empty, absolute, or duplicated", sentinel, path)
}

func writeDeterministicTGZ(w io.Writer, members []Member) error {
	gz := gzip.NewWriter(w)
	if err := writeDeterministicTar(gz, members); err != nil {
		_ = gz.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		return writeFailedError("gzip", err)
	}
	return nil
}

func writeDeterministicTar(w io.Writer, members []Member) error {
	tw := tar.NewWriter(w)
	for index := range members {
		if err := writeTarMember(tw, &members[index]); err != nil {
			_ = tw.Close()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return writeFailedError("tar", err)
	}
	return nil
}

func writeTarMember(tw *tar.Writer, member *Member) error {
	header := &tar.Header{
		Name:    member.Path,
		Mode:    int64(member.Mode & deterministicPermMask),
		ModTime: tarEpoch,
	}
	switch member.Kind {
	case MemberDirectory:
		header.Typeflag = tar.TypeDir
		header.Name += "/"
	case MemberSymlink:
		header.Typeflag = tar.TypeSymlink
		header.Linkname = member.Target
		header.Mode = 0
	case MemberFile:
		header.Typeflag = tar.TypeReg
		header.Size = member.Size
	default:
		return memberError(member.Path, errUtils.ErrArchiveWriteFailed)
	}
	if err := tw.WriteHeader(header); err != nil {
		return writeFailedError(member.Path, err)
	}
	if member.Kind != MemberFile {
		return nil
	}
	return copyMemberContent(tw, member)
}

func writeDeterministicZip(w io.Writer, members []Member) error {
	zw := zip.NewWriter(w)
	for index := range members {
		if err := writeZipMember(zw, &members[index]); err != nil {
			_ = zw.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return writeFailedError("zip", err)
	}
	return nil
}

func writeZipMember(zw *zip.Writer, member *Member) error {
	header := &zip.FileHeader{Name: member.Path, Method: zip.Deflate, Modified: zipEpoch}
	perm := member.Mode & deterministicPermMask
	switch member.Kind {
	case MemberDirectory:
		header.Name += "/"
		header.Method = zip.Store
		header.SetMode(fs.ModeDir | perm)
	case MemberSymlink:
		header.Method = zip.Store
		header.SetMode(fs.ModeSymlink)
	case MemberFile:
		header.SetMode(perm)
	default:
		return memberError(member.Path, errUtils.ErrArchiveWriteFailed)
	}
	out, err := zw.CreateHeader(header)
	if err != nil {
		return writeFailedError(member.Path, err)
	}
	switch member.Kind {
	case MemberSymlink:
		if _, err := io.WriteString(out, member.Target); err != nil {
			return writeFailedError(member.Path, err)
		}
	case MemberFile:
		return copyMemberContent(out, member)
	}
	return nil
}

// copyMemberContent copies exactly member.Size bytes and fails if the source is shorter or longer,
// so an archive never describes bytes other than the ones it carries.
func copyMemberContent(dst io.Writer, member *Member) error {
	if member.Open == nil {
		return memberError(member.Path, errUtils.ErrArchiveWriteFailed)
	}
	src, err := member.Open()
	if err != nil {
		return writeFailedError(member.Path, err)
	}
	defer src.Close()

	written, err := io.Copy(dst, io.LimitReader(src, member.Size))
	if err != nil {
		return writeFailedError(member.Path, err)
	}
	if written != member.Size {
		return writeFailedError(member.Path, io.ErrUnexpectedEOF)
	}
	var extra [1]byte
	if n, readErr := src.Read(extra[:]); n > 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
		return writeFailedError(member.Path, fmt.Errorf("%w: content is longer than the declared %d bytes", errUtils.ErrArchiveWriteFailed, member.Size))
	}
	return nil
}
