package oci

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// An import whose destination reference disagrees with the package's own Record is rejected before
// anything is written: no mismatched tag is published, and an existing valid tag stays intact.
func TestImportLayout_RejectsAMismatchedRecordIdentityBeforeWriting(t *testing.T) {
	ctx := context.Background()
	source := newLayoutRepo(t)
	published := publish(t, source, publishSpec{content: "exported content"}).Pinned
	dir := t.TempDir()
	_, err := source.ExportLayout(ctx, published, dir)
	require.NoError(t, err)

	tests := []struct {
		name string
		ref  artifact.Ref
	}{
		{name: "another name", ref: artifact.Ref{Name: "other/app", Version: appVersion}},
		{name: "another version", ref: artifact.Ref{Name: appName, Version: "9.9.9"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			destination := newLayoutRepo(t)
			existing := publish(t, destination, publishSpec{name: tt.ref.Name, version: tt.ref.Version, content: "already published"}).Pinned
			before, err := destination.be.(*layoutBackend).references(ctx)
			require.NoError(t, err)

			_, err = destination.ImportLayout(ctx, dir, tt.ref, ImportOptions{Force: true})
			require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)

			after, err := destination.be.(*layoutBackend).references(ctx)
			require.NoError(t, err)
			assert.Equal(t, before, after, "the rejected import published no tag")
			intact, err := destination.Resolve(ctx, tt.ref)
			require.NoError(t, err)
			assert.Equal(t, existing.ResolvedDigest, intact.ResolvedDigest, "the existing reference is untouched")
		})
	}

	t.Run("matching identity imports", func(t *testing.T) {
		destination := newLayoutRepo(t)
		result, err := destination.ImportLayout(ctx, dir, artifact.Ref{Name: appName, Version: appVersion}, ImportOptions{})
		require.NoError(t, err)
		assert.Equal(t, published.ResolvedDigest, result.Pinned.ResolvedDigest)
	})
}
