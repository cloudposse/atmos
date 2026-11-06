package artifact

import (
	"strings"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestEntryValidate(t *testing.T) {
	fileDigest := digest.FromString("x")
	tests := []struct {
		name  string
		entry Entry
		want  error
	}{
		{"file", Entry{Path: "a/b.txt", Type: EntryFile, Size: 1, Mode: 0o644, Digest: fileDigest}, nil},
		{"executable file", Entry{Path: "run.sh", Type: EntryFile, Mode: 0o755, Digest: fileDigest}, nil},
		{"directory", Entry{Path: "a", Type: EntryDirectory, Mode: 0o755}, nil},
		{"contained symlink", Entry{Path: "a/link", Type: EntrySymlink, Target: "../b.txt"}, nil},
		{"symlink to own directory", Entry{Path: "a/link", Type: EntrySymlink, Target: "."}, nil},
		{"symlink empty segment", Entry{Path: "a/link", Type: EntrySymlink, Target: "b//c"}, errUtils.ErrArtifactUnsafePath},
		{"symlink escaping the package", Entry{Path: "a/link", Type: EntrySymlink, Target: "../../b"}, errUtils.ErrArtifactUnsafePath},
		{"symlink at root escaping", Entry{Path: "link", Type: EntrySymlink, Target: "../x"}, errUtils.ErrArtifactUnsafePath},
		{"absolute symlink target", Entry{Path: "link", Type: EntrySymlink, Target: "/etc/passwd"}, errUtils.ErrArtifactUnsafePath},
		{"empty symlink target", Entry{Path: "link", Type: EntrySymlink}, errUtils.ErrArtifactUnsafePath},
		{"symlink with mode", Entry{Path: "link", Type: EntrySymlink, Target: "a", Mode: 0o777}, errUtils.ErrArtifactRecordInvalid},
		{"setuid bit", Entry{Path: "su", Type: EntryFile, Mode: 0o4755, Digest: fileDigest}, errUtils.ErrArtifactRecordInvalid},
		{"file without digest", Entry{Path: "a", Type: EntryFile}, errUtils.ErrArtifactRecordInvalid},
		{"file with bad digest", Entry{Path: "a", Type: EntryFile, Digest: "sha256:zz"}, errUtils.ErrArtifactRecordInvalid},
		{"file with negative size", Entry{Path: "a", Type: EntryFile, Size: -1, Digest: fileDigest}, errUtils.ErrArtifactRecordInvalid},
		{"directory with digest", Entry{Path: "a", Type: EntryDirectory, Digest: fileDigest}, errUtils.ErrArtifactRecordInvalid},
		{"device type", Entry{Path: "dev", Type: "device"}, errUtils.ErrArtifactPackagingInvalid},
		{"absolute path", Entry{Path: "/a", Type: EntryDirectory}, errUtils.ErrArtifactUnsafePath},
		{"traversal path", Entry{Path: "a/../b", Type: EntryDirectory}, errUtils.ErrArtifactUnsafePath},
		{"empty segment", Entry{Path: "a//b", Type: EntryDirectory}, errUtils.ErrArtifactUnsafePath},
		{"backslash path", Entry{Path: `a\b`, Type: EntryDirectory}, errUtils.ErrArtifactUnsafePath},
		{"control character", Entry{Path: "a\x00b", Type: EntryDirectory}, errUtils.ErrArtifactUnsafePath},
		{"invalid UTF-8", Entry{Path: "a\xffb", Type: EntryDirectory}, errUtils.ErrArtifactUnsafePath},
		{"reserved namespace", Entry{Path: ReservedNamespace + "/v1/refs/x.json", Type: EntryDirectory}, errUtils.ErrArtifactUnsafePath},
		{"reserved namespace prefix only as first segment", Entry{Path: "a/" + ReservedNamespace, Type: EntryDirectory}, nil},
		{"unicode name", Entry{Path: "ünï/名前.txt", Type: EntryFile, Digest: fileDigest}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.entry.Validate()
			if tt.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestValidateInventoryStructure(t *testing.T) {
	file := func(p string) Entry { return Entry{Path: p, Type: EntryFile, Digest: digest.FromString(p)} }
	dir := func(p string) Entry { return Entry{Path: p, Type: EntryDirectory} }

	_, err := ComputeContentDigest([]Entry{file("a"), file("a")}, nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid, "duplicate path")

	_, err = ComputeContentDigest([]Entry{file("a"), file("a/b")}, nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid, "entry nested under a file")

	_, err = ComputeContentDigest([]Entry{{Path: "l", Type: EntrySymlink, Target: "x"}, file("l/b")}, nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid, "entry nested under a symlink")

	_, err = ComputeContentDigest([]Entry{dir("a"), file("a/b")}, nil)
	require.NoError(t, err)

	// The digest is order independent for the same logical inventory.
	first, err := ComputeContentDigest([]Entry{file("b"), file("a")}, nil)
	require.NoError(t, err)
	second, err := ComputeContentDigest([]Entry{file("a"), file("b")}, nil)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestDocumentValidate(t *testing.T) {
	good := Document{Role: DocumentReadme, SourcePath: "README.md", MediaType: DocumentMediaType, Size: 10, Digest: digest.FromString("r")}
	require.NoError(t, good.Validate())

	tests := []struct {
		name   string
		mutate func(d *Document)
		want   error
	}{
		{"unknown role", func(d *Document) { d.Role = "license" }, errUtils.ErrArtifactRecordInvalid},
		{"absolute source", func(d *Document) { d.SourcePath = "/README.md" }, errUtils.ErrArtifactRecordInvalid},
		{"traversing source", func(d *Document) { d.SourcePath = "../README.md" }, errUtils.ErrArtifactRecordInvalid},
		{"no media type", func(d *Document) { d.MediaType = "" }, errUtils.ErrArtifactRecordInvalid},
		{"negative size", func(d *Document) { d.Size = -1 }, errUtils.ErrArtifactRecordInvalid},
		{"oversize document", func(d *Document) { d.Size = MaxDocumentSize + 1 }, errUtils.ErrArtifactTooLarge},
		{"bad digest", func(d *Document) { d.Digest = "nope" }, errUtils.ErrArtifactRecordInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := good
			tt.mutate(&doc)
			require.ErrorIs(t, doc.Validate(), tt.want)
		})
	}

	boundary := good
	boundary.Size = MaxDocumentSize
	require.NoError(t, boundary.Validate(), "exactly the limit is allowed")

	_, err := ComputeContentDigest(nil, []Document{good, good})
	require.ErrorIs(t, err, errUtils.ErrArtifactRecordInvalid, "duplicate role")
}

func TestRecordValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(r *Record)
		want   error
	}{
		{"valid", func(*Record) {}, nil},
		{"wrong schema version", func(r *Record) { r.SchemaVersion = 2 }, errUtils.ErrArtifactRecordInvalid},
		{"bad name", func(r *Record) { r.Name = "../x" }, errUtils.ErrArtifactUnsafePath},
		{"bad version", func(r *Record) { r.Version = "1+x" }, errUtils.ErrArtifactRefInvalid},
		{"bad kind", func(r *Record) { r.Kind = "Files" }, errUtils.ErrArtifactRecordInvalid},
		{"missing created_at", func(r *Record) { r.CreatedAt = Timestamp{} }, errUtils.ErrArtifactRecordInvalid},
		{"empty label key", func(r *Record) { r.Metadata.Labels = map[string]string{"": "v"} }, errUtils.ErrArtifactRecordInvalid},
		{"stale content digest", func(r *Record) { r.ContentDigest = digest.FromString("other") }, errUtils.ErrArtifactRecordInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := vectorRecordB(t)
			tt.mutate(&record)
			err := record.Validate()
			if tt.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.want)
			_, canonicalErr := record.Canonical()
			require.ErrorIs(t, canonicalErr, tt.want, "an invalid record is never serialized")
		})
	}
}

func TestRecordOversizeRejected(t *testing.T) {
	record := vectorRecordB(t)
	record.Metadata.Description = strings.Repeat("d", int(MaxRecordSize))
	_, err := record.Canonical()
	require.ErrorIs(t, err, errUtils.ErrArtifactTooLarge)
	_, err = record.RecordDigest()
	require.ErrorIs(t, err, errUtils.ErrArtifactTooLarge)
}

func TestEmptyProvenanceNormalizesAway(t *testing.T) {
	bare := vectorRecordB(t)
	withEmpty := vectorRecordB(t)
	withEmpty.Metadata.Provenance = &Provenance{CI: &CIProvenance{}, Source: &SourceProvenance{}}
	withEmpty.Metadata.Labels = map[string]string{}

	first, err := bare.Canonical()
	require.NoError(t, err)
	second, err := withEmpty.Canonical()
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))
}

func TestSealSetsSchemaVersionAndContentDigest(t *testing.T) {
	record := Record{Name: "a", Version: "1", Kind: KindFiles, CreatedAt: NewTimestamp(time.Unix(1, 0))}
	require.NoError(t, record.Seal())
	assert.Equal(t, SchemaVersion, record.SchemaVersion)
	assert.NotEmpty(t, record.ContentDigest)
}

func TestTimestamp(t *testing.T) {
	zone := time.FixedZone("x", 5*3600)
	stamp := NewTimestamp(time.Date(2026, 10, 30, 17, 0, 0, 999, zone))
	assert.Equal(t, "2026-10-30T12:00:00Z", stamp.String())
	assert.Equal(t, time.UTC, stamp.Time().Location())

	var parsed Timestamp
	require.NoError(t, parsed.UnmarshalJSON([]byte(`"2026-10-30T12:00:00Z"`)))
	assert.Equal(t, stamp.Time(), parsed.Time())

	for _, bad := range []string{`"2026-10-30T12:00:00+00:00"`, `"2026-10-30T12:00:00.5Z"`, `"2026-10-30 12:00:00"`, `12`, `""`} {
		var ts Timestamp
		require.ErrorIs(t, ts.UnmarshalJSON([]byte(bad)), errUtils.ErrArtifactRecordInvalid, bad)
	}
	assert.True(t, Timestamp{}.IsZero())
}
