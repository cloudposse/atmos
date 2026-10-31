package produce

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/archive"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// Constants for staged payloads.
const (
	// The media type of a per-file payload blob.
	payloadMediaType = "application/octet-stream"

	// The mode of generated archive files.
	generatedFilePerm = 0o644

	// The mode of generated archive directories.
	generatedDirPerm = 0o755

	// The mode of the staged archive file itself.
	archiveFilePerm = 0o600
)

// opener opens the bytes of one inventory file.
type opener func(entryPath string) (io.ReadCloser, error)

// assembly turns a sealed Record, its documents, and a file opener into staged blobs.
type assembly struct {
	record      *artifact.Record
	recordBytes []byte
	docs        []stagedDocument
	open        opener
	stagingDir  string
}

// blobs stages the payload for packaging and then the separate document blobs.
func (a *assembly) blobs(ctx context.Context, packaging artifact.Packaging) ([]artifact.StagedBlob, error) {
	var (
		payload []artifact.StagedBlob
		err     error
	)
	switch packaging {
	case artifact.PackagingNone:
		payload = a.fileBlobs()
	case artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip:
		var blob artifact.StagedBlob
		blob, err = a.archiveBlob(ctx, packaging)
		payload = []artifact.StagedBlob{blob}
	default:
		err = fmt.Errorf("%w: cannot assemble packaging %q", errUtils.ErrArtifactPackagingInvalid, packaging)
	}
	if err != nil {
		return nil, err
	}
	return append(payload, a.documentBlobs()...), nil
}

func (a *assembly) fileBlobs() []artifact.StagedBlob {
	var blobs []artifact.StagedBlob
	for index := range a.record.Inventory {
		entry := a.record.Inventory[index]
		if entry.Type != artifact.EntryFile {
			continue
		}
		blobs = append(blobs, artifact.StagedBlob{
			Descriptor: ocispec.Descriptor{MediaType: payloadMediaType, Digest: entry.Digest, Size: entry.Size},
			Role:       artifact.BlobPayload,
			Path:       entry.Path,
			Open:       func(context.Context) (io.ReadCloser, error) { return a.open(entry.Path) },
		})
	}
	return blobs
}

func (a *assembly) documentBlobs() []artifact.StagedBlob {
	blobs := make([]artifact.StagedBlob, 0, len(a.docs))
	for index := range a.docs {
		doc := &a.docs[index]
		blobs = append(blobs, artifact.StagedBlob{
			Descriptor:   ocispec.Descriptor{MediaType: doc.descriptor.MediaType, Digest: doc.descriptor.Digest, Size: doc.descriptor.Size},
			Role:         artifact.BlobDocument,
			DocumentRole: doc.descriptor.Role,
			Open:         func(context.Context) (io.ReadCloser, error) { return doc.open(), nil },
		})
	}
	return blobs
}

// archiveBlob writes the deterministic archive into staging, hashing it as it is written. The
// context is checked before every member is opened, so cancellation stops a large assembly.
func (a *assembly) archiveBlob(ctx context.Context, packaging artifact.Packaging) (artifact.StagedBlob, error) {
	format, mediaType := archiveFormat(packaging)
	path := filepath.Join(a.stagingDir, "payload."+format)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, archiveFilePerm)
	if err != nil {
		return artifact.StagedBlob{}, fmt.Errorf("%w: create archive: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	hasher := sha256.New()
	writeErr := archive.WriteDeterministic(io.MultiWriter(file, hasher), format, withContext(ctx, a.members()))
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		return artifact.StagedBlob{}, fmt.Errorf("%w: %w", errUtils.ErrArtifactPackagingInvalid, writeErr)
	}
	info, err := os.Stat(path)
	if err != nil {
		return artifact.StagedBlob{}, fmt.Errorf("%w: stat archive: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	return artifact.StagedBlob{
		Descriptor: ocispec.Descriptor{MediaType: mediaType, Digest: digest.NewDigest(digest.SHA256, hasher), Size: info.Size()},
		Role:       artifact.BlobPayload,
		Open:       func(context.Context) (io.ReadCloser, error) { return os.Open(path) },
	}, nil
}

// withContext makes every file member fail to open once ctx is canceled.
func withContext(ctx context.Context, members []archive.Member) []archive.Member {
	for index := range members {
		open := members[index].Open
		if open == nil {
			continue
		}
		members[index].Open = func() (io.ReadCloser, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return open()
		}
	}
	return members
}

func archiveFormat(packaging artifact.Packaging) (string, string) {
	switch packaging {
	case artifact.PackagingTar:
		return archive.FormatTar, artifact.MediaTypeTar
	case artifact.PackagingZip:
		return archive.FormatZip, artifact.MediaTypeZip
	default:
		return archive.FormatTGZ, artifact.MediaTypeTarGzip
	}
}

// members lists the archive members: the inventory plus the generated reserved directory holding
// manifest.json and the embedded documents. The generated members never enter the Record.
func (a *assembly) members() []archive.Member {
	members := make([]archive.Member, 0, len(a.record.Inventory)+len(a.docs)+3)
	present := make(map[string]bool, len(a.record.Inventory))
	for index := range a.record.Inventory {
		entry := a.record.Inventory[index]
		present[entry.Path] = true
		members = append(members, a.inventoryMember(&entry))
	}
	if !present[reservedParent] {
		members = append(members, archive.Member{Path: reservedParent, Kind: archive.MemberDirectory, Mode: generatedDirPerm})
	}
	members = append(members, archive.Member{Path: reservedDir, Kind: archive.MemberDirectory, Mode: generatedDirPerm})
	members = append(members, bytesMember(reservedDir+"/"+manifestName, a.recordBytes))
	for index := range a.docs {
		members = append(members, bytesMember(reservedDir+"/"+reservedDocumentNames[a.docs[index].descriptor.Role], a.docs[index].data))
	}
	return members
}

func (a *assembly) inventoryMember(entry *artifact.Entry) archive.Member {
	member := archive.Member{Path: entry.Path, Mode: os.FileMode(entry.Mode)}
	switch entry.Type {
	case artifact.EntryDirectory:
		member.Kind = archive.MemberDirectory
	case artifact.EntrySymlink:
		member.Kind = archive.MemberSymlink
		member.Target = entry.Target
	default:
		member.Kind = archive.MemberFile
		member.Size = entry.Size
		entryPath := entry.Path
		member.Open = func() (io.ReadCloser, error) { return a.open(entryPath) }
	}
	return member
}

func bytesMember(path string, data []byte) archive.Member {
	doc := stagedDocument{data: data}
	return archive.Member{
		Path: path, Kind: archive.MemberFile, Mode: generatedFilePerm, Size: int64(len(data)),
		Open: func() (io.ReadCloser, error) { return doc.open(), nil },
	}
}
