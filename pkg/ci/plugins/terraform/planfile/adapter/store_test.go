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

	data := bytes.NewReader([]byte("plan data"))
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
		gomock.Len(1),
		gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ string, files []artifact.FileEntry, artMeta *artifact.Metadata) error {
		// Verify the file entry.
		assert.Equal(t, planFilename, files[0].Name)
		assert.Equal(t, int64(-1), files[0].Size)

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

	err := store.Upload(context.Background(), "dev/vpc/abc123.tfplan", data, meta)
	require.NoError(t, err)
}

func TestStore_Upload_NilMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockStore(ctrl)
	store := NewStore(mockBackend)

	data := bytes.NewReader([]byte("plan data"))

	mockBackend.EXPECT().Upload(
		gomock.Any(),
		"key",
		gomock.Len(1),
		nil,
	).Return(nil)

	err := store.Upload(context.Background(), "key", data, nil)
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
			{Name: "lock.hcl", Data: lockData, Size: 12},
			{Name: planFilename, Data: planData, Size: 12},
		},
		artMeta,
		nil,
	)

	reader, meta, err := store.Download(context.Background(), "dev/vpc/abc123.tfplan")
	require.NoError(t, err)
	require.NotNil(t, reader)
	defer reader.Close()

	content, _ := io.ReadAll(reader)
	assert.Equal(t, "plan content", string(content))

	assert.Equal(t, "dev", meta.Stack)
	assert.Equal(t, "vpc", meta.Component)
	assert.Equal(t, "abc123", meta.SHA)
	assert.True(t, meta.HasChanges)
	assert.Equal(t, 5, meta.Additions)
	assert.Equal(t, 2, meta.Changes)
	assert.Equal(t, 1, meta.Destructions)
	assert.Equal(t, "1.5.0", meta.TerraformVersion)
}

func TestStore_Download_NoPlanFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockStore(ctrl)
	store := NewStore(mockBackend)

	otherData := io.NopCloser(bytes.NewReader([]byte("other")))

	mockBackend.EXPECT().Download(gomock.Any(), "key").Return(
		[]artifact.FileResult{
			{Name: "other.txt", Data: otherData, Size: 5},
		},
		&artifact.Metadata{},
		nil,
	)

	reader, _, err := store.Download(context.Background(), "key")
	assert.Nil(t, reader)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), planFilename)
}

func TestStore_Download_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockStore(ctrl)
	store := NewStore(mockBackend)

	backendErr := errors.New("not found")
	mockBackend.EXPECT().Download(gomock.Any(), "key").Return(nil, nil, backendErr)

	reader, _, err := store.Download(context.Background(), "key")
	assert.Nil(t, reader)
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
	mockBackend.EXPECT().List(gomock.Any(), artifact.Query{
		Stacks: []string{"dev"},
	}).Return([]artifact.ArtifactInfo{
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

	infos, err := store.List(context.Background(), "dev")
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

	infos, err := store.List(context.Background(), "")
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

func TestPrefixToQuery(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		expected artifact.Query
	}{
		{
			name:     "empty prefix matches all",
			prefix:   "",
			expected: artifact.Query{All: true},
		},
		{
			name:     "stack only",
			prefix:   "stack1",
			expected: artifact.Query{Stacks: []string{"stack1"}},
		},
		{
			name:   "stack and component",
			prefix: "stack1/component1",
			expected: artifact.Query{
				Stacks:     []string{"stack1"},
				Components: []string{"component1"},
			},
		},
		{
			name:   "stack, component, and sha",
			prefix: "stack1/component1/sha123",
			expected: artifact.Query{
				Stacks:     []string{"stack1"},
				Components: []string{"component1"},
				SHAs:       []string{"sha123"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := prefixToQuery(tt.prefix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewStoreFactory(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockBackend := artifact.NewMockStore(ctrl)

	factory := NewStoreFactory(mockBackend)
	require.NotNil(t, factory)

	store, err := factory(planfile.StoreOptions{Type: "adapter"})
	require.NoError(t, err)
	require.NotNil(t, store)

	// Verify the created store delegates to the mock backend.
	mockBackend.EXPECT().Name().Return("local")
	assert.Equal(t, "local", store.Name())
}
