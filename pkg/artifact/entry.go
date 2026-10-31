package artifact

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// maxPathLength bounds one inventory path, in bytes.
const maxPathLength = 4096

// firstPrintableRune is the first rune that is not an ASCII control character.
const firstPrintableRune = 0x20

// deleteRune is the ASCII DEL control character.
const deleteRune = 0x7f

// permissionMask holds the ordinary permission bits an Entry may carry.
const permissionMask uint32 = 0o777

// ValidatePayloadPath checks a relative, slash-separated path inside a package. It rejects empty,
// absolute, backslash, control-character, "." / ".." segments, invalid UTF-8, and any path that
// enters the managed [ReservedNamespace]. Paths are never rewritten.
func ValidatePayloadPath(p string) error {
	defer perf.Track(nil, "artifact.ValidatePayloadPath")()

	if err := validateRelativePath(p, false); err != nil {
		return err
	}
	if first, _, _ := strings.Cut(p, "/"); first == ReservedNamespace {
		return fmt.Errorf("%w: %q enters the reserved %s namespace", errUtils.ErrArtifactUnsafePath, p, ReservedNamespace)
	}
	return nil
}

// validateRelativePath checks a relative slash-separated path. Link targets pass allowDots because
// "." and ".." segments are legitimate there as long as the resolved target stays inside the package.
func validateRelativePath(p string, allowDots bool) error {
	if err := validatePathShape(p); err != nil {
		return err
	}
	for _, segment := range strings.Split(p, "/") {
		if segment == "" || (!allowDots && (segment == "." || segment == "..")) {
			return fmt.Errorf("%w: path %q has an empty, '.', or '..' segment", errUtils.ErrArtifactUnsafePath, p)
		}
	}
	return nil
}

// validatePathShape checks length, encoding, rootedness, separators, and control characters.
func validatePathShape(p string) error {
	if p == "" || len(p) > maxPathLength {
		return fmt.Errorf("%w: path must be 1-%d bytes", errUtils.ErrArtifactUnsafePath, maxPathLength)
	}
	if !utf8.ValidString(p) {
		return fmt.Errorf("%w: path %q is not valid UTF-8", errUtils.ErrArtifactUnsafePath, p)
	}
	if strings.HasPrefix(p, "/") || strings.Contains(p, `\`) || hasControlRune(p) {
		return fmt.Errorf("%w: path %q must be relative, slash-separated, and free of control characters", errUtils.ErrArtifactUnsafePath, p)
	}
	return nil
}

func hasControlRune(p string) bool {
	return strings.IndexFunc(p, func(r rune) bool { return r < firstPrintableRune || r == deleteRune }) >= 0
}

// Validate checks one inventory entry in isolation.
func (e *Entry) Validate() error {
	defer perf.Track(nil, "artifact.Entry.Validate")()

	if err := ValidatePayloadPath(e.Path); err != nil {
		return err
	}
	if e.Mode&^permissionMask != 0 {
		return fmt.Errorf("%w: entry %q mode %#o has bits outside 0777; privileged bits are cleared", errUtils.ErrArtifactRecordInvalid, e.Path, e.Mode)
	}
	switch e.Type {
	case EntryFile:
		return e.validateFile()
	case EntryDirectory:
		return e.validateDirectory()
	case EntrySymlink:
		return e.validateSymlink()
	default:
		return fmt.Errorf("%w: entry %q has unsupported type %q", errUtils.ErrArtifactPackagingInvalid, e.Path, e.Type)
	}
}

func (e *Entry) validateFile() error {
	if e.Size < 0 || e.Target != "" {
		return fmt.Errorf("%w: file %q needs a non-negative size and no link target", errUtils.ErrArtifactRecordInvalid, e.Path)
	}
	if e.Digest == "" {
		return fmt.Errorf("%w: file %q needs a digest", errUtils.ErrArtifactRecordInvalid, e.Path)
	}
	if err := ValidateDigest(e.Digest); err != nil {
		return fmt.Errorf("%w: file %q: %w", errUtils.ErrArtifactRecordInvalid, e.Path, err)
	}
	return nil
}

func (e *Entry) validateDirectory() error {
	if e.Size != 0 || e.Digest != "" || e.Target != "" {
		return fmt.Errorf("%w: directory %q must have no size, digest, or target", errUtils.ErrArtifactRecordInvalid, e.Path)
	}
	return nil
}

func (e *Entry) validateSymlink() error {
	if e.Size != 0 || e.Mode != 0 || e.Digest != "" {
		return fmt.Errorf("%w: symlink %q must have no size, mode, or digest", errUtils.ErrArtifactRecordInvalid, e.Path)
	}
	if err := validateRelativePath(e.Target, true); err != nil {
		return fmt.Errorf("%w: symlink %q target: %w", errUtils.ErrArtifactUnsafePath, e.Path, err)
	}
	resolved := path.Clean(path.Dir(e.Path) + "/" + e.Target)
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return fmt.Errorf("%w: symlink %q target %q escapes the package", errUtils.ErrArtifactUnsafePath, e.Path, e.Target)
	}
	return nil
}

// validateInventory checks every entry, rejects duplicate paths, and rejects entries nested under
// a file or symlink entry.
func validateInventory(inventory []Entry) error {
	types := make(map[string]EntryType, len(inventory))
	for _, entry := range inventory {
		if err := entry.Validate(); err != nil {
			return err
		}
		if _, duplicate := types[entry.Path]; duplicate {
			return fmt.Errorf("%w: duplicate inventory path %q", errUtils.ErrArtifactPackagingInvalid, entry.Path)
		}
		types[entry.Path] = entry.Type
	}
	for _, entry := range inventory {
		for parent := path.Dir(entry.Path); parent != "."; parent = path.Dir(parent) {
			if parentType, found := types[parent]; found && parentType != EntryDirectory {
				return fmt.Errorf("%w: %q is nested under non-directory %q", errUtils.ErrArtifactPackagingInvalid, entry.Path, parent)
			}
		}
	}
	return nil
}

// Validate checks one document descriptor in isolation.
func (d Document) Validate() error {
	defer perf.Track(nil, "artifact.Document.Validate")()

	if d.Role != DocumentReadme && d.Role != DocumentChangelog {
		return fmt.Errorf("%w: unsupported document role %q", errUtils.ErrArtifactRecordInvalid, d.Role)
	}
	if err := validateRelativePath(d.SourcePath, false); err != nil {
		return fmt.Errorf("%w: %s source path: %w", errUtils.ErrArtifactRecordInvalid, d.Role, err)
	}
	if d.MediaType == "" {
		return fmt.Errorf("%w: %s needs a media type", errUtils.ErrArtifactRecordInvalid, d.Role)
	}
	if d.Size < 0 {
		return fmt.Errorf("%w: %s has a negative size", errUtils.ErrArtifactRecordInvalid, d.Role)
	}
	if d.Size > MaxDocumentSize {
		return fmt.Errorf("%w: %s is %d bytes; the limit is %d", errUtils.ErrArtifactTooLarge, d.Role, d.Size, MaxDocumentSize)
	}
	if err := ValidateDigest(d.Digest); err != nil {
		return fmt.Errorf("%w: %s: %w", errUtils.ErrArtifactRecordInvalid, d.Role, err)
	}
	return nil
}

func validateDocuments(documents []Document) error {
	seen := make(map[DocumentRole]struct{}, len(documents))
	for _, document := range documents {
		if err := document.Validate(); err != nil {
			return err
		}
		if _, duplicate := seen[document.Role]; duplicate {
			return fmt.Errorf("%w: duplicate %s document", errUtils.ErrArtifactRecordInvalid, document.Role)
		}
		seen[document.Role] = struct{}{}
	}
	return nil
}

// sortedInventory returns a copy of inventory sorted by path (bytewise).
func sortedInventory(inventory []Entry) []Entry {
	out := make([]Entry, len(inventory))
	copy(out, inventory)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// sortedDocuments returns a copy of documents sorted by role.
func sortedDocuments(documents []Document) []Document {
	out := make([]Document, len(documents))
	copy(out, documents)
	sort.Slice(out, func(i, j int) bool { return out[i].Role < out[j].Role })
	return out
}
