package artifact

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetadataJSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	expires := now.Add(24 * time.Hour)

	original := &Metadata{
		Stack:        "plat-ue2-dev",
		Component:    "vpc",
		SHA:          "abc123def456",
		BaseSHA:      "000111222333",
		Branch:       "feature/test",
		PRNumber:     42,
		RunID:        "run-123",
		Repository:   "cloudposse/atmos",
		CreatedAt:    now,
		ExpiresAt:    &expires,
		SHA256:       "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		AtmosVersion: "1.100.0",
		Custom: map[string]string{
			"env": "production",
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Metadata
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original.Stack, decoded.Stack)
	assert.Equal(t, original.Component, decoded.Component)
	assert.Equal(t, original.SHA, decoded.SHA)
	assert.Equal(t, original.BaseSHA, decoded.BaseSHA)
	assert.Equal(t, original.Branch, decoded.Branch)
	assert.Equal(t, original.PRNumber, decoded.PRNumber)
	assert.Equal(t, original.RunID, decoded.RunID)
	assert.Equal(t, original.Repository, decoded.Repository)
	assert.Equal(t, original.SHA256, decoded.SHA256)
	assert.Equal(t, original.AtmosVersion, decoded.AtmosVersion)
	assert.Equal(t, original.Custom, decoded.Custom)
	assert.True(t, original.CreatedAt.Equal(decoded.CreatedAt))
	require.NotNil(t, decoded.ExpiresAt)
	assert.True(t, original.ExpiresAt.Equal(*decoded.ExpiresAt))
}

func TestMetadataJSONNilOptionalFields(t *testing.T) {
	original := &Metadata{
		Stack:     "dev",
		Component: "vpc",
		SHA:       "abc123",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Metadata
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Nil(t, decoded.ExpiresAt)
	assert.Empty(t, decoded.BaseSHA)
	assert.Empty(t, decoded.Custom)
}

func TestArtifactInfoJSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	original := ArtifactInfo{
		Name:         "plat-ue2-dev/vpc/abc123",
		Size:         1024,
		LastModified: now,
		Metadata: &Metadata{
			Stack:     "plat-ue2-dev",
			Component: "vpc",
			SHA:       "abc123",
			CreatedAt: now,
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded ArtifactInfo
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original.Name, decoded.Name)
	assert.Equal(t, original.Size, decoded.Size)
	require.NotNil(t, decoded.Metadata)
	assert.Equal(t, original.Metadata.Stack, decoded.Metadata.Stack)
}
