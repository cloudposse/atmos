package marketplace

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAgentMetadata_Valid(t *testing.T) {
	metadataPath := filepath.Join("testdata", "valid-agent", ".agent.yaml")

	metadata, err := ParseAgentMetadata(metadataPath)
	require.NoError(t, err)
	require.NotNil(t, metadata)

	// Verify all fields parsed correctly.
	assert.Equal(t, "test-agent", metadata.Name)
	assert.Equal(t, "Test Agent", metadata.DisplayName)
	assert.Equal(t, "1.0.0", metadata.Version)
	assert.Equal(t, "Test Author", metadata.Author)
	assert.Equal(t, "A test agent for unit testing", metadata.Description)
	assert.Equal(t, "general", metadata.Category)

	// Verify Atmos version constraints.
	assert.Equal(t, "1.0.0", metadata.Atmos.MinVersion)
	assert.Equal(t, "", metadata.Atmos.MaxVersion)

	// Verify prompt config.
	assert.Equal(t, "prompt.md", metadata.Prompt.File)

	// Verify tools config.
	require.NotNil(t, metadata.Tools)
	assert.Equal(t, []string{"describe_stacks", "describe_component"}, metadata.Tools.Allowed)
	assert.Equal(t, []string{"terraform_apply", "terraform_destroy"}, metadata.Tools.Restricted)

	// Verify repository.
	assert.Equal(t, "https://github.com/test/test-agent", metadata.Repository)
}

func TestParseAgentMetadata_InvalidFile(t *testing.T) {
	tests := []struct {
		name         string
		metadataPath string
		wantErrMsg   string
	}{
		{
			name:         "nonexistent file",
			metadataPath: filepath.Join("testdata", "nonexistent", ".agent.yaml"),
			wantErrMsg:   "failed to read metadata file",
		},
		{
			name:         "invalid metadata",
			metadataPath: filepath.Join("testdata", "invalid-metadata", ".agent.yaml"),
			wantErrMsg:   "display_name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := ParseAgentMetadata(tt.metadataPath)
			assert.Error(t, err)
			assert.Nil(t, metadata)
			assert.Contains(t, err.Error(), tt.wantErrMsg)
		})
	}
}

func TestValidateMetadata(t *testing.T) {
	tests := []struct {
		name       string
		metadata   *AgentMetadata
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "valid metadata",
			metadata: &AgentMetadata{
				Name:        "test-agent",
				DisplayName: "Test Agent",
				Version:     "1.0.0",
				Author:      "Test Author",
				Description: "Test description",
				Category:    "general",
				Repository:  "https://github.com/test/agent",
				Atmos: AtmosCompatibility{
					MinVersion: "1.0.0",
				},
				Prompt: PromptConfig{
					File: "prompt.md",
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			metadata: &AgentMetadata{
				DisplayName: "Test Agent",
				Version:     "1.0.0",
				Author:      "Test Author",
				Description: "Test description",
				Category:    "general",
			},
			wantErr:    true,
			wantErrMsg: "name is required",
		},
		{
			name: "missing display_name",
			metadata: &AgentMetadata{
				Name:        "test-agent",
				Version:     "1.0.0",
				Author:      "Test Author",
				Description: "Test description",
				Category:    "general",
			},
			wantErr:    true,
			wantErrMsg: "display_name is required",
		},
		{
			name: "missing version",
			metadata: &AgentMetadata{
				Name:        "test-agent",
				DisplayName: "Test Agent",
				Author:      "Test Author",
				Description: "Test description",
				Category:    "general",
			},
			wantErr:    true,
			wantErrMsg: "version is required",
		},
		{
			name: "invalid category",
			metadata: &AgentMetadata{
				Name:        "test-agent",
				DisplayName: "Test Agent",
				Version:     "1.0.0",
				Author:      "Test Author",
				Description: "Test description",
				Category:    "invalid-category",
				Atmos: AtmosCompatibility{
					MinVersion: "1.0.0",
				},
			},
			wantErr:    true,
			wantErrMsg: "invalid category",
		},
		{
			name: "missing min_version",
			metadata: &AgentMetadata{
				Name:        "test-agent",
				DisplayName: "Test Agent",
				Version:     "1.0.0",
				Author:      "Test Author",
				Description: "Test description",
				Category:    "general",
				Atmos:       AtmosCompatibility{},
			},
			wantErr:    true,
			wantErrMsg: "atmos.min_version is required",
		},
		{
			name: "missing prompt file",
			metadata: &AgentMetadata{
				Name:        "test-agent",
				DisplayName: "Test Agent",
				Version:     "1.0.0",
				Author:      "Test Author",
				Description: "Test description",
				Category:    "general",
				Atmos: AtmosCompatibility{
					MinVersion: "1.0.0",
				},
				Prompt: PromptConfig{},
			},
			wantErr:    true,
			wantErrMsg: "prompt.file is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMetadata(tt.metadata)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
