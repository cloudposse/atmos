package artifact_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
)

func sampleRecord() artifact.Record {
	return artifact.Record{
		SchemaVersion: artifact.SchemaVersion,
		Name:          "team/app",
		Version:       "1.0.0",
		Kind:          artifact.KindFiles,
		Metadata: artifact.Metadata{
			Description: "sample",
			Labels:      map[string]string{"a": "1"},
			Provenance: &artifact.Provenance{
				CI:     &artifact.CIProvenance{Provider: "github", RunID: "1"},
				Source: &artifact.SourceProvenance{Repository: "repo", Commit: "abc"},
			},
		},
		Inventory: []artifact.Entry{{Path: "a.txt", Type: artifact.EntryFile, Size: 1}, {Path: "b.txt", Type: artifact.EntryFile, Size: 2}},
		Documents: []artifact.Document{{Role: artifact.DocumentReadme, SourcePath: "README.md"}},
		KindData:  artifact.KindData(`{"k":1}`),
	}
}

// TestRecord_CloneIsIndependentInBothDirections mutates the clone and then the original, and
// requires that neither edit shows up in the other.
func TestRecord_CloneIsIndependentInBothDirections(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(r *artifact.Record)
		read   func(r *artifact.Record) any
		want   any
	}{
		{"labels", func(r *artifact.Record) { r.Metadata.Labels["a"] = "changed" }, func(r *artifact.Record) any { return r.Metadata.Labels }, map[string]string{"a": "1"}},
		{"inventory", func(r *artifact.Record) { r.Inventory[0].Path = "changed" }, func(r *artifact.Record) any { return r.Inventory[0].Path }, "a.txt"},
		{"inventory last", func(r *artifact.Record) { r.Inventory[1].Size = 99 }, func(r *artifact.Record) any { return r.Inventory[1].Size }, int64(2)},
		{"documents", func(r *artifact.Record) { r.Documents[0].SourcePath = "changed" }, func(r *artifact.Record) any { return r.Documents[0].SourcePath }, "README.md"},
		{"kind data", func(r *artifact.Record) { r.KindData[0] = 'X' }, func(r *artifact.Record) any { return string(r.KindData) }, `{"k":1}`},
		{"CI provenance", func(r *artifact.Record) { r.Metadata.Provenance.CI.RunID = "changed" }, func(r *artifact.Record) any { return r.Metadata.Provenance.CI.RunID }, "1"},
		{"source provenance", func(r *artifact.Record) { r.Metadata.Provenance.Source.Commit = "changed" }, func(r *artifact.Record) any { return r.Metadata.Provenance.Source.Commit }, "abc"},
		{"provenance", func(r *artifact.Record) { r.Metadata.Provenance.CI = nil }, func(r *artifact.Record) any { return r.Metadata.Provenance.CI != nil }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/clone to original", func(t *testing.T) {
			original := sampleRecord()
			clone := original.Clone()
			tt.mutate(&clone)
			assert.Equal(t, tt.want, tt.read(&original))
		})
		t.Run(tt.name+"/original to clone", func(t *testing.T) {
			original := sampleRecord()
			clone := original.Clone()
			tt.mutate(&original)
			assert.Equal(t, tt.want, tt.read(&clone))
		})
	}
}

func TestRecord_CloneKeepsContent(t *testing.T) {
	original := sampleRecord()
	clone := original.Clone()
	assert.Equal(t, original, clone)

	empty := artifact.Record{}
	assert.Equal(t, empty, empty.Clone(), "nil maps and slices stay nil")
}

func TestCapabilities_CloneIsIndependentInBothDirections(t *testing.T) {
	sample := func() artifact.Capabilities {
		return artifact.Capabilities{
			Read:            true,
			Packagings:      []artifact.Packaging{artifact.PackagingTar, artifact.PackagingZip},
			SubjectKinds:    []artifact.SubjectKind{artifact.SubjectRecord},
			EvidenceFormats: []artifact.EvidenceFormat{"one", "two"},
			LockScopes:      []artifact.LockScope{artifact.LockScopeHost},
		}
	}
	original := sample()
	clone := original.Clone()
	require.Equal(t, original, clone)

	clone.Packagings[0], clone.SubjectKinds[0], clone.EvidenceFormats[1], clone.LockScopes[0] = artifact.PackagingTgz, "x", "x", artifact.LockScopeRepository
	assert.Equal(t, sample(), original, "editing the clone leaves the original alone")

	original = sample()
	fresh := sample()
	clone = fresh.Clone()
	original.Packagings[1], original.SubjectKinds[0], original.EvidenceFormats[0], original.LockScopes[0] = artifact.PackagingNone, "x", "x", artifact.LockScopeRepository
	assert.Equal(t, sample(), clone, "editing the original leaves the clone alone")
}
