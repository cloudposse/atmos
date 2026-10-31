package produce

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/snapshot"
)

// Names of the reserved distribution paths.
const (
	// The hidden directory that holds the reserved distribution directory.
	reservedParent = artifact.DistributionParent

	// The source directory reserved for README and CHANGELOG distribution documents.
	reservedDir = artifact.DistributionDir

	// The generated manifest, written only inside the packaged representation.
	manifestName = "manifest.json"
)

var reservedDocumentNames = map[artifact.DocumentRole]string{
	artifact.DocumentReadme:    "README.md",
	artifact.DocumentChangelog: "CHANGELOG.md",
}

// documentRoles lists the roles in canonical (sorted) order.
var documentRoles = []artifact.DocumentRole{artifact.DocumentChangelog, artifact.DocumentReadme}

// stagedDocument is a document descriptor plus its bytes (at most [artifact.MaxDocumentSize]).
type stagedDocument struct {
	descriptor artifact.Document
	data       []byte
}

// checkReserved rejects reserved-directory contents other than the two distribution documents,
// including a conflicting source manifest.json, subdirectories, and links.
func checkReserved(entries []artifact.Entry) error {
	for index := range entries {
		entry := &entries[index]
		switch {
		case entry.Path == reservedDir:
			if entry.Type != artifact.EntryDirectory {
				return reservedError(entry.Path, "must be a directory")
			}
		case strings.HasPrefix(entry.Path, reservedDir+"/"):
			if !isReservedDocument(entry) {
				return reservedError(entry.Path, "is not README.md or CHANGELOG.md; the generated "+manifestName+" is written only inside the packaged representation")
			}
		}
	}
	return nil
}

func isReservedDocument(entry *artifact.Entry) bool {
	if entry.Type != artifact.EntryFile {
		return false
	}
	name := strings.TrimPrefix(entry.Path, reservedDir+"/")
	for _, reserved := range reservedDocumentNames {
		if name == reserved {
			return true
		}
	}
	return false
}

func reservedError(entryPath, reason string) error {
	return fmt.Errorf("%w: %q in the reserved %s directory %s", errUtils.ErrArtifactPackagingInvalid, entryPath, reservedDir, reason)
}

// payloadInventory removes the reserved distribution subtree from the snapshot inventory and drops
// the .atmos parent when nothing else lives under it.
func payloadInventory(entries []artifact.Entry) []artifact.Entry {
	hasOtherAtmos := false
	for index := range entries {
		p := entries[index].Path
		if strings.HasPrefix(p, reservedParent+"/") && p != reservedDir && !strings.HasPrefix(p, reservedDir+"/") {
			hasOtherAtmos = true
			break
		}
	}
	inventory := make([]artifact.Entry, 0, len(entries))
	for index := range entries {
		p := entries[index].Path
		if p == reservedDir || strings.HasPrefix(p, reservedDir+"/") {
			continue
		}
		if p == reservedParent && !hasOtherAtmos {
			continue
		}
		inventory = append(inventory, entries[index])
	}
	return inventory
}

// documentOverrides carries the explicit --readme and --changelog paths.
type documentOverrides map[artifact.DocumentRole]string

// collectDocuments resolves each document role: an explicit override wins over the reserved
// distribution file. Overrides resolve relative to the source directory or, for a single-file
// source, the file's parent.
func collectDocuments(snap *snapshot.Snapshot, source string, overrides documentOverrides) ([]stagedDocument, error) {
	var docs []stagedDocument
	for _, role := range documentRoles {
		doc, found, err := collectDocument(snap, source, role, overrides[role])
		if err != nil {
			return nil, err
		}
		if found {
			docs = append(docs, doc)
		}
	}
	return docs, nil
}

func collectDocument(snap *snapshot.Snapshot, source string, role artifact.DocumentRole, override string) (stagedDocument, bool, error) {
	if override != "" {
		rel, err := cleanDocumentPath(role, override)
		if err != nil {
			return stagedDocument{}, false, err
		}
		doc, err := loadOverride(snap, source, role, rel)
		return doc, err == nil, err
	}
	rel := reservedDir + "/" + reservedDocumentNames[role]
	if snap.SourceIsFile {
		return stagedDocument{}, false, nil
	}
	entry, ok := snap.Lookup(rel)
	if !ok {
		return stagedDocument{}, false, nil
	}
	data, err := readStagedDocument(snap, &entry)
	if err != nil {
		return stagedDocument{}, false, err
	}
	return newStagedDocument(role, rel, data), true, nil
}

// cleanDocumentPath validates an explicit document path: relative, slash-normalized, contained.
func cleanDocumentPath(role artifact.DocumentRole, override string) (string, error) {
	slashed := filepath.ToSlash(override)
	cleaned := path.Clean(slashed)
	if filepath.IsAbs(override) || strings.HasPrefix(slashed, "/") || cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned == "." {
		return "", fmt.Errorf("%w: %s path %q must be relative to the source and stay inside it", errUtils.ErrArtifactUnsafePath, role, override)
	}
	if err := artifact.ValidatePayloadPath(cleaned); err != nil {
		return "", fmt.Errorf("%s path %q: %w", role, override, err)
	}
	return cleaned, nil
}

func loadOverride(snap *snapshot.Snapshot, source string, role artifact.DocumentRole, rel string) (stagedDocument, error) {
	if snap.SourceIsFile {
		data, err := readSiblingDocument(source, role, rel)
		if err != nil {
			return stagedDocument{}, err
		}
		return newStagedDocument(role, rel, data), nil
	}
	entry, ok := snap.Lookup(rel)
	if !ok || entry.Type != artifact.EntryFile {
		return stagedDocument{}, fmt.Errorf("%w: %s file %q is not a regular file inside the source", errUtils.ErrArtifactPackagingInvalid, role, rel)
	}
	data, err := readStagedDocument(snap, &entry)
	if err != nil {
		return stagedDocument{}, err
	}
	return newStagedDocument(role, rel, data), nil
}

func readStagedDocument(snap *snapshot.Snapshot, entry *artifact.Entry) ([]byte, error) {
	if entry.Size > artifact.MaxDocumentSize {
		return nil, fmt.Errorf("%w: %s is %d bytes; the limit is %d", errUtils.ErrArtifactTooLarge, entry.Path, entry.Size, artifact.MaxDocumentSize)
	}
	reader, err := snap.Open(entry.Path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return readBounded(reader, entry.Path)
}

// readSiblingDocument reads an explicit document next to a single-file source, rejecting anything
// that is not a regular file contained in the source's parent directory.
func readSiblingDocument(source string, role artifact.DocumentRole, rel string) ([]byte, error) {
	resolvedSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return nil, fmt.Errorf("%w: source %q: %w", errUtils.ErrArtifactPackagingInvalid, source, err)
	}
	parent := filepath.Dir(resolvedSource)
	full, err := filepath.EvalSymlinks(filepath.Join(parent, filepath.FromSlash(rel)))
	if err != nil {
		return nil, fmt.Errorf("%w: %s file %q: %w", errUtils.ErrArtifactPackagingInvalid, role, rel, err)
	}
	if relative, err := filepath.Rel(parent, full); err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("%w: %s file %q resolves outside the source directory", errUtils.ErrArtifactUnsafePath, role, rel)
	}
	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s file %q is not a regular file", errUtils.ErrArtifactPackagingInvalid, role, rel)
	}
	if info.Size() > artifact.MaxDocumentSize {
		return nil, fmt.Errorf("%w: %s is %d bytes; the limit is %d", errUtils.ErrArtifactTooLarge, rel, info.Size(), artifact.MaxDocumentSize)
	}
	file, err := os.Open(full)
	if err != nil {
		return nil, fmt.Errorf("%w: %s file %q: %w", errUtils.ErrArtifactPackagingInvalid, role, rel, err)
	}
	defer file.Close()
	return readBounded(file, rel)
}

func readBounded(reader io.Reader, name string) ([]byte, error) {
	data, err := artifact.ReadBounded(reader, artifact.MaxDocumentSize)
	if err != nil {
		return nil, fmt.Errorf("document %q: %w", name, err)
	}
	return data, nil
}

func newStagedDocument(role artifact.DocumentRole, sourcePath string, data []byte) stagedDocument {
	return stagedDocument{
		descriptor: artifact.Document{
			Role:       role,
			SourcePath: sourcePath,
			MediaType:  artifact.DocumentMediaType,
			Size:       int64(len(data)),
			Digest:     digest.FromBytes(data),
		},
		data: data,
	}
}

func (d *stagedDocument) open() io.ReadCloser {
	return io.NopCloser(bytes.NewReader(d.data))
}
