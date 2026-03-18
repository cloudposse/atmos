package adapter

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/ci/artifact"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
)

func TestStore_Name(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockStore(ctrl)
	store := NewStore(mockBackend)

	mockBackend.EXPECT().Name().Return("local")

	assert.Equal(t, "local", store.Name())
}

func TestStore_Upload(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockStore(ctrl)
	store := NewStore(mockBackend)

	files := []planfile.FileEntry{
		{Name: planfile.PlanFilename, Data: bytes.NewReader([]byte("plan data")), Size: -1},
		{Name: planfile.LockFilename, Data: bytes.NewReader([]byte("lock data")), Size: -1},
	}

	meta := &planfile.Metadata{
		HasChanges:       true,
		Additions:        3,
		Changes:          1,
		Destructions:     0,
		TerraformVersion: "1.5.0",
		TerraformTool:    "terraform",
	}
	meta.Stack = "dev"
	meta.Component = "vpc"
	meta.SHA = "abc123"

	mockBackend.EXPECT().Upload(
		gomock.Any(),
		"dev/vpc/abc123.tfplan",
		gomock.Len(2),
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ string, f []artifact.FileEntry, artMeta *artifact.Metadata) error {
		// Verify the file entries are passed through.
		assert.Equal(t, planfile.PlanFilename, f[0].Name)
		assert.Equal(t, planfile.LockFilename, f[1].Name)

		// Verify metadata conversion.
		assert.Equal(t, "dev", artMeta.Stack)
		assert.Equal(t, "vpc", artMeta.Component)
		assert.Equal(t, "abc123", artMeta.SHA)
		assert.Equal(t, "true", artMeta.Custom[customKeyHasChanges])
		assert.Equal(t, "3", artMeta.Custom[customKeyAdditions])
		assert.Equal(t, "1", artMeta.Custom[customKeyChanges])
		assert.Equal(t, "0", artMeta.Custom[customKeyDestructions])
		assert.Equal(t, "1.5.0", artMeta.Custom[customKeyTerraformVersion])
		assert.Equal(t, "terraform", artMeta.Custom[customKeyTerraformTool])

		return nil
	})

	err := store.Upload(context.Background(), "dev/vpc/abc123.tfplan", files, meta)
	require.NoError(t, err)
}

func TestStore_Upload_NilMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockStore(ctrl)
	store := NewStore(mockBackend)

	files := []planfile.FileEntry{
		{Name: planfile.PlanFilename, Data: bytes.NewReader([]byte("plan data")), Size: -1},
	}

	mockBackend.EXPECT().Upload(
		gomock.Any(),
		"key",
		gomock.Len(1),
		nil,
	).Return(nil)

	err := store.Upload(context.Background(), "key", files, nil)
	require.NoError(t, err)
}

func TestStore_Download(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockStore(ctrl)
	store := NewStore(mockBackend)

	planData := io.NopCloser(bytes.NewReader([]byte("plan content")))
	lockData := io.NopCloser(bytes.NewReader([]byte("lock content")))

	artMeta := &artifact.Metadata{
		Stack:     "dev",
		Component: "vpc",
		SHA:       "abc123",
		Custom: map[string]string{
			customKeyHasChanges:       "true",
			customKeyAdditions:        "5",
			customKeyChanges:          "2",
			customKeyDestructions:     "1",
			customKeyTerraformVersion: "1.5.0",
		},
	}

	mockBackend.EXPECT().Download(gomock.Any(), "dev/vpc/abc123.tfplan").Return(
		[]artifact.FileResult{
			{Name: planfile.LockFilename, Data: lockData, Size: 12},
			{Name: planfile.PlanFilename, Data: planData, Size: 12},
		},
		artMeta,
		nil,
	)

	results, meta, err := store.Download(context.Background(), "dev/vpc/abc123.tfplan")
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Close all results when done.
	defer func() {
		for _, r := range results {
			r.Data.Close()
		}
	}()

	// Verify metadata conversion.
	assert.Equal(t, "dev", meta.Stack)
	assert.Equal(t, "vpc", meta.Component)
	assert.Equal(t, "abc123", meta.SHA)
	assert.True(t, meta.HasChanges)
	assert.Equal(t, 5, meta.Additions)
	assert.Equal(t, 2, meta.Changes)
	assert.Equal(t, 1, meta.Destructions)
	assert.Equal(t, "1.5.0", meta.TerraformVersion)
}

func TestStore_Download_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockStore(ctrl)
	store := NewStore(mockBackend)

	backendErr := errors.New("not found")
	mockBackend.EXPECT().Download(gomock.Any(), "key").Return(nil, nil, backendErr)

	results, _, err := store.Download(context.Background(), "key")
	assert.Nil(t, results)
	assert.Error(t, err)
	assert.ErrorIs(t, err, backendErr)
}

func TestStore_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockStore(ctrl)
	store := NewStore(mockBackend)

	mockBackend.EXPECT().Delete(gomock.Any(), "dev/vpc/abc123.tfplan").Return(nil)

	err := store.Delete(context.Background(), "dev/vpc/abc123.tfplan")
	require.NoError(t, err)
}

func TestStore_List(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockStore(ctrl)
	store := NewStore(mockBackend)

	now := time.Now()
	query := artifact.Query{Stacks: []string{"dev"}}

	mockBackend.EXPECT().List(gomock.Any(), query).Return([]artifact.ArtifactInfo{
		{
			Name:         "dev/vpc/abc123.tfplan",
			Size:         1024,
			LastModified: now,
			Metadata: &artifact.Metadata{
				Stack:     "dev",
				Component: "vpc",
				SHA:       "abc123",
				Custom: map[string]string{
					customKeyHasChanges: "true",
				},
			},
		},
	}, nil)

	infos, err := store.List(context.Background(), query)
	require.NoError(t, err)
	require.Len(t, infos, 1)

	assert.Equal(t, "dev/vpc/abc123.tfplan", infos[0].Key)
	assert.Equal(t, int64(1024), infos[0].Size)
	assert.Equal(t, now, infos[0].LastModified)
	assert.Equal(t, "dev", infos[0].Metadata.Stack)
	assert.Equal(t, "vpc", infos[0].Metadata.Component)
	assert.True(t, infos[0].Metadata.HasChanges)
}

func TestStore_List_Empty(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockStore(ctrl)
	store := NewStore(mockBackend)

	mockBackend.EXPECT().List(gomock.Any(), artifact.Query{All: true}).Return([]artifact.ArtifactInfo{}, nil)

	infos, err := store.List(context.Background(), artifact.Query{All: true})
	require.NoError(t, err)
	assert.Empty(t, infos)
}

func TestStore_Exists(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockStore(ctrl)
	store := NewStore(mockBackend)

	mockBackend.EXPECT().Exists(gomock.Any(), "key").Return(true, nil)

	exists, err := store.Exists(context.Background(), "key")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestStore_GetMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockStore(ctrl)
	store := NewStore(mockBackend)

	artMeta := &artifact.Metadata{
		Stack:     "staging",
		Component: "rds",
		SHA:       "def456",
		Custom: map[string]string{
			customKeyPlanSummary:      "Plan: 2 to add",
			customKeyHasChanges:       "true",
			customKeyAdditions:        "2",
			customKeyChanges:          "0",
			customKeyDestructions:     "0",
			customKeyTerraformVersion: "1.6.0",
			customKeyTerraformTool:    "tofu",
			"extra_key":               "extra_value",
		},
	}

	mockBackend.EXPECT().GetMetadata(gomock.Any(), "staging/rds/def456.tfplan").Return(artMeta, nil)

	meta, err := store.GetMetadata(context.Background(), "staging/rds/def456.tfplan")
	require.NoError(t, err)
	assert.Equal(t, "staging", meta.Stack)
	assert.Equal(t, "rds", meta.Component)
	assert.Equal(t, "def456", meta.SHA)
	assert.Equal(t, "Plan: 2 to add", meta.PlanSummary)
	assert.True(t, meta.HasChanges)
	assert.Equal(t, 2, meta.Additions)
	assert.Equal(t, 0, meta.Changes)
	assert.Equal(t, 0, meta.Destructions)
	assert.Equal(t, "1.6.0", meta.TerraformVersion)
	assert.Equal(t, "tofu", meta.TerraformTool)
	assert.Equal(t, "extra_value", meta.Custom["extra_key"])
}

func TestStore_GetMetadata_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockStore(ctrl)
	store := NewStore(mockBackend)

	backendErr := errors.New("not found")
	mockBackend.EXPECT().GetMetadata(gomock.Any(), "key").Return(nil, backendErr)

	meta, err := store.GetMetadata(context.Background(), "key")
	assert.Nil(t, meta)
	assert.Error(t, err)
	assert.ErrorIs(t, err, backendErr)
}

func TestMetadataConversion_RoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	expires := now.Add(24 * time.Hour)

	original := &planfile.Metadata{
		PlanSummary:      "Plan: 1 to add, 2 to change",
		HasChanges:       true,
		Additions:        1,
		Changes:          2,
		Destructions:     3,
		TerraformVersion: "1.5.0",
		TerraformTool:    "terraform",
	}
	original.Stack = "prod"
	original.Component = "eks"
	original.SHA = "abc123"
	original.BaseSHA = "def456"
	original.Branch = "feature/test"
	original.PRNumber = 42
	original.RunID = "run-123"
	original.Repository = "org/repo"
	original.CreatedAt = now
	original.ExpiresAt = &expires
	original.Custom = map[string]string{
		"user_key": "user_value",
	}

	// Convert planfile → artifact → planfile.
	artMeta := planfileToArtifactMeta(original)
	roundTripped := artifactToPlanfileMeta(artMeta)

	assert.Equal(t, original.Stack, roundTripped.Stack)
	assert.Equal(t, original.Component, roundTripped.Component)
	assert.Equal(t, original.SHA, roundTripped.SHA)
	assert.Equal(t, original.BaseSHA, roundTripped.BaseSHA)
	assert.Equal(t, original.Branch, roundTripped.Branch)
	assert.Equal(t, original.PRNumber, roundTripped.PRNumber)
	assert.Equal(t, original.RunID, roundTripped.RunID)
	assert.Equal(t, original.Repository, roundTripped.Repository)
	assert.Equal(t, original.CreatedAt, roundTripped.CreatedAt)
	assert.Equal(t, original.ExpiresAt, roundTripped.ExpiresAt)
	assert.Equal(t, original.PlanSummary, roundTripped.PlanSummary)
	assert.Equal(t, original.HasChanges, roundTripped.HasChanges)
	assert.Equal(t, original.Additions, roundTripped.Additions)
	assert.Equal(t, original.Changes, roundTripped.Changes)
	assert.Equal(t, original.Destructions, roundTripped.Destructions)
	assert.Equal(t, original.TerraformVersion, roundTripped.TerraformVersion)
	assert.Equal(t, original.TerraformTool, roundTripped.TerraformTool)
	assert.Equal(t, "user_value", roundTripped.Custom["user_key"])
}

func TestMetadataConversion_NilInput(t *testing.T) {
	assert.Nil(t, planfileToArtifactMeta(nil))
	assert.Nil(t, artifactToPlanfileMeta(nil))
}
