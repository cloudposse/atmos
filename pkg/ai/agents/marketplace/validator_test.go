package marketplace

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidator_Validate(t *testing.T) {
	tests := []struct {
		name       string
		agentPath  string
		atmosVer   string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:      "valid agent",
			agentPath: filepath.Join("testdata", "valid-agent"),
			atmosVer:  "1.50.0",
			wantErr:   false,
		},
		{
			name:       "missing metadata file",
			agentPath:  filepath.Join("testdata", "nonexistent"),
			atmosVer:   "1.50.0",
			wantErr:    true,
			wantErrMsg: ".agent.yaml not found",
		},
		{
			name:       "missing prompt file",
			agentPath:  filepath.Join("testdata", "missing-prompt"),
			atmosVer:   "1.50.0",
			wantErr:    true,
			wantErrMsg: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(tt.atmosVer)

			// Parse metadata first (if it exists).
			metadataPath := filepath.Join(tt.agentPath, ".agent.yaml")
			metadata, _ := ParseAgentMetadata(metadataPath)

			err := validator.Validate(tt.agentPath, metadata)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tt.wantErrMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateVersionCompatibility(t *testing.T) {
	tests := []struct {
		name       string
		atmosVer   string
		minVer     string
		maxVer     string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:     "compatible version",
			atmosVer: "1.50.0",
			minVer:   "1.0.0",
			maxVer:   "",
			wantErr:  false,
		},
		{
			name:       "version too old",
			atmosVer:   "1.0.0",
			minVer:     "1.50.0",
			maxVer:     "",
			wantErr:    true,
			wantErrMsg: "agent requires Atmos >= 1.50.0",
		},
		{
			name:       "version too new",
			atmosVer:   "2.0.0",
			minVer:     "1.0.0",
			maxVer:     "1.99.0",
			wantErr:    true,
			wantErrMsg: "agent requires Atmos <= 1.99.0",
		},
		{
			name:     "within range",
			atmosVer: "1.50.0",
			minVer:   "1.0.0",
			maxVer:   "2.0.0",
			wantErr:  false,
		},
		{
			name:       "invalid min version",
			atmosVer:   "1.50.0",
			minVer:     "invalid",
			maxVer:     "",
			wantErr:    true,
			wantErrMsg: "invalid min_version",
		},
		{
			name:       "invalid max version",
			atmosVer:   "1.50.0",
			minVer:     "1.0.0",
			maxVer:     "invalid",
			wantErr:    true,
			wantErrMsg: "invalid max_version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator(tt.atmosVer)

			metadata := &AgentMetadata{
				Atmos: AtmosCompatibility{
					MinVersion: tt.minVer,
					MaxVersion: tt.maxVer,
				},
			}

			err := validator.validateVersionCompatibility(metadata)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tt.wantErrMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateToolConfig(t *testing.T) {
	tests := []struct {
		name       string
		tools      *ToolConfig
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "valid tool config",
			tools: &ToolConfig{
				Allowed:    []string{"describe_stacks", "describe_component"},
				Restricted: []string{"terraform_apply", "terraform_destroy"},
			},
			wantErr: false,
		},
		{
			name:    "nil tool config",
			tools:   nil,
			wantErr: false,
		},
		{
			name: "tool in both allowed and restricted",
			tools: &ToolConfig{
				Allowed:    []string{"describe_stacks", "terraform_apply"},
				Restricted: []string{"terraform_apply", "terraform_destroy"},
			},
			wantErr:    true,
			wantErrMsg: "cannot be both allowed and restricted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator("1.50.0")

			metadata := &AgentMetadata{
				Tools: tt.tools,
			}

			err := validator.validateToolConfig(metadata)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tt.wantErrMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidatePromptStructure(t *testing.T) {
	tests := []struct {
		name       string
		promptPath string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "valid prompt",
			promptPath: filepath.Join("testdata", "valid-agent", "prompt.md"),
			wantErr:    false,
		},
		{
			name:       "nonexistent prompt",
			promptPath: filepath.Join("testdata", "nonexistent", "prompt.md"),
			wantErr:    true,
			wantErrMsg: "failed to read prompt file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator("1.50.0")

			err := validator.validatePromptStructure(tt.promptPath)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tt.wantErrMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_PromptMustStartWithHeading(t *testing.T) {
	// Create a temporary prompt file without heading.
	tmpDir := t.TempDir()
	invalidPromptPath := filepath.Join(tmpDir, "invalid-prompt.md")

	err := WriteTestFile(invalidPromptPath, "This is a prompt without a heading")
	require.NoError(t, err)

	validator := NewValidator("1.50.0")
	err = validator.validatePromptStructure(invalidPromptPath)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "should start with level-1 heading")
}
