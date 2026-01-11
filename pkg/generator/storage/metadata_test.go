package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetadataStorage(t *testing.T) {
	path := "/tmp/test/metadata.yaml"
	storage := NewMetadataStorage(path)

	assert.NotNil(t, storage)
	assert.Equal(t, path, storage.metadataPath)
}

func TestMetadataStorage_GetMetadataPath(t *testing.T) {
	path := "/tmp/test/metadata.yaml"
	storage := NewMetadataStorage(path)

	assert.Equal(t, path, storage.GetMetadataPath())
}

func TestMetadataStorage_Exists(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string
		expected bool
	}{
		{
			name: "file exists",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				metadataPath := filepath.Join(tmpDir, "metadata.yaml")
				err := os.WriteFile(metadataPath, []byte("test: data"), 0o644)
				require.NoError(t, err)
				return metadataPath
			},
			expected: true,
		},
		{
			name: "file does not exist",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "nonexistent.yaml")
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadataPath := tt.setup(t)
			storage := NewMetadataStorage(metadataPath)

			assert.Equal(t, tt.expected, storage.Exists())
		})
	}
}

func TestMetadataStorage_Load(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		expectError bool
		expectNil   bool
		validate    func(t *testing.T, metadata *GenerationMetadata)
	}{
		{
			name: "load valid metadata",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				metadataPath := filepath.Join(tmpDir, "metadata.yaml")
				content := `version: 1
command: atmos init
template:
  name: simple
  version: 1.0.0
  source: embedded
base_ref: main
generated_at: 2024-01-15T10:00:00Z
variables:
  project_name: test-project
  author: test-author
files:
  - path: atmos.yaml
    template_path: templates/atmos.yaml
    checksum: abc123
  - path: stacks/orgs/acme/prod.yaml
    template_path: templates/stack.yaml
    checksum: def456
storage_type: git
`
				err := os.WriteFile(metadataPath, []byte(content), 0o644)
				require.NoError(t, err)
				return metadataPath
			},
			expectError: false,
			expectNil:   false,
			validate: func(t *testing.T, metadata *GenerationMetadata) {
				assert.Equal(t, 1, metadata.Version)
				assert.Equal(t, "atmos init", metadata.Command)
				assert.Equal(t, "simple", metadata.Template.Name)
				assert.Equal(t, "1.0.0", metadata.Template.Version)
				assert.Equal(t, "embedded", metadata.Template.Source)
				assert.Equal(t, "main", metadata.BaseRef)
				assert.Equal(t, "test-project", metadata.Variables["project_name"])
				assert.Equal(t, "test-author", metadata.Variables["author"])
				assert.Len(t, metadata.Files, 2)
				assert.Equal(t, "atmos.yaml", metadata.Files[0].Path)
				assert.Equal(t, "abc123", metadata.Files[0].Checksum)
				assert.Equal(t, "git", metadata.StorageType)
			},
		},
		{
			name: "file does not exist returns nil",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "nonexistent.yaml")
			},
			expectError: false,
			expectNil:   true,
		},
		{
			name: "invalid yaml returns error",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				metadataPath := filepath.Join(tmpDir, "metadata.yaml")
				content := `invalid: yaml: content: [unclosed`
				err := os.WriteFile(metadataPath, []byte(content), 0o644)
				require.NoError(t, err)
				return metadataPath
			},
			expectError: true,
			expectNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadataPath := tt.setup(t)
			storage := NewMetadataStorage(metadataPath)

			metadata, err := storage.Load()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectNil {
				assert.Nil(t, metadata)
			} else {
				assert.NotNil(t, metadata)
				if tt.validate != nil {
					tt.validate(t, metadata)
				}
			}
		})
	}
}

func TestMetadataStorage_Save(t *testing.T) {
	tests := []struct {
		name        string
		metadata    *GenerationMetadata
		setup       func(t *testing.T) string
		expectError bool
		validate    func(t *testing.T, path string)
	}{
		{
			name: "save valid metadata",
			metadata: &GenerationMetadata{
				Version: 1,
				Command: "atmos init",
				Template: TemplateInfo{
					Name:    "simple",
					Version: "1.0.0",
					Source:  "embedded",
				},
				BaseRef:     "main",
				GeneratedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
				Variables: map[string]string{
					"project_name": "test-project",
				},
				Files: []GeneratedFile{
					{
						Path:         "atmos.yaml",
						TemplatePath: "templates/atmos.yaml",
						Checksum:     "abc123",
					},
				},
				StorageType: "git",
			},
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "metadata.yaml")
			},
			expectError: false,
			validate: func(t *testing.T, path string) {
				// Verify file was created
				assert.FileExists(t, path)

				// Verify content can be loaded back
				storage := NewMetadataStorage(path)
				metadata, err := storage.Load()
				require.NoError(t, err)
				require.NotNil(t, metadata)

				assert.Equal(t, 1, metadata.Version)
				assert.Equal(t, "atmos init", metadata.Command)
				assert.Equal(t, "simple", metadata.Template.Name)
			},
		},
		{
			name: "save creates parent directories",
			metadata: &GenerationMetadata{
				Version: 1,
				Command: "atmos scaffold generate",
				Template: TemplateInfo{
					Name: "component",
				},
				GeneratedAt: time.Now(),
				Variables:   map[string]string{},
				Files:       []GeneratedFile{},
				StorageType: "file",
			},
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "nested", "path", "metadata.yaml")
			},
			expectError: false,
			validate: func(t *testing.T, path string) {
				assert.FileExists(t, path)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadataPath := tt.setup(t)
			storage := NewMetadataStorage(metadataPath)

			err := storage.Save(tt.metadata)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, metadataPath)
				}
			}
		})
	}
}

func TestNewMetadata(t *testing.T) {
	tests := []struct {
		name            string
		templateName    string
		templateVersion string
		templateSource  string
		baseRef         string
		variables       map[string]string
		createMetadata  func(name, version, source, ref string, vars map[string]string) *GenerationMetadata
		expectedCommand string
	}{
		{
			name:            "init metadata",
			templateName:    "simple",
			templateVersion: "1.0.0",
			templateSource:  "embedded",
			baseRef:         "main",
			variables: map[string]string{
				"project_name": "test-project",
				"author":       "test-author",
			},
			createMetadata:  NewInitMetadata,
			expectedCommand: "atmos init",
		},
		{
			name:            "scaffold metadata",
			templateName:    "component",
			templateVersion: "2.0.0",
			templateSource:  "atmos.yaml",
			baseRef:         "v1.0",
			variables: map[string]string{
				"component_name": "vpc",
				"namespace":      "core",
			},
			createMetadata:  NewScaffoldMetadata,
			expectedCommand: "atmos scaffold generate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := tt.createMetadata(tt.templateName, tt.templateVersion, tt.templateSource, tt.baseRef, tt.variables)

			assert.NotNil(t, metadata)
			assert.Equal(t, 1, metadata.Version)
			assert.Equal(t, tt.expectedCommand, metadata.Command)
			assert.Equal(t, tt.templateName, metadata.Template.Name)
			assert.Equal(t, tt.templateVersion, metadata.Template.Version)
			assert.Equal(t, tt.templateSource, metadata.Template.Source)
			assert.Equal(t, tt.baseRef, metadata.BaseRef)
			assert.Equal(t, tt.variables, metadata.Variables)
			assert.Empty(t, metadata.Files)
			assert.Equal(t, "git", metadata.StorageType)
			assert.False(t, metadata.GeneratedAt.IsZero())
		})
	}
}

func TestGenerationMetadata_AddFile(t *testing.T) {
	metadata := &GenerationMetadata{
		Version:     1,
		Command:     "atmos init",
		Template:    TemplateInfo{Name: "simple"},
		GeneratedAt: time.Now(),
		Variables:   map[string]string{},
		Files:       []GeneratedFile{},
		StorageType: "file",
	}

	assert.Len(t, metadata.Files, 0)

	metadata.AddFile("atmos.yaml", "templates/atmos.yaml", "abc123")
	assert.Len(t, metadata.Files, 1)
	assert.Equal(t, "atmos.yaml", metadata.Files[0].Path)
	assert.Equal(t, "templates/atmos.yaml", metadata.Files[0].TemplatePath)
	assert.Equal(t, "abc123", metadata.Files[0].Checksum)

	metadata.AddFile("stacks/prod.yaml", "templates/stack.yaml", "def456")
	assert.Len(t, metadata.Files, 2)
	assert.Equal(t, "stacks/prod.yaml", metadata.Files[1].Path)
	assert.Equal(t, "def456", metadata.Files[1].Checksum)
}

func TestGenerationMetadata_GetFile(t *testing.T) {
	metadata := &GenerationMetadata{
		Version:     1,
		Command:     "atmos init",
		Template:    TemplateInfo{Name: "simple"},
		GeneratedAt: time.Now(),
		Variables:   map[string]string{},
		Files: []GeneratedFile{
			{
				Path:         "atmos.yaml",
				TemplatePath: "templates/atmos.yaml",
				Checksum:     "abc123",
			},
			{
				Path:         "stacks/prod.yaml",
				TemplatePath: "templates/stack.yaml",
				Checksum:     "def456",
			},
		},
		StorageType: "file",
	}

	tests := []struct {
		name       string
		path       string
		expectFind bool
		validate   func(t *testing.T, file GeneratedFile)
	}{
		{
			name:       "find existing file",
			path:       "atmos.yaml",
			expectFind: true,
			validate: func(t *testing.T, file GeneratedFile) {
				assert.Equal(t, "atmos.yaml", file.Path)
				assert.Equal(t, "templates/atmos.yaml", file.TemplatePath)
				assert.Equal(t, "abc123", file.Checksum)
			},
		},
		{
			name:       "find second file",
			path:       "stacks/prod.yaml",
			expectFind: true,
			validate: func(t *testing.T, file GeneratedFile) {
				assert.Equal(t, "stacks/prod.yaml", file.Path)
				assert.Equal(t, "def456", file.Checksum)
			},
		},
		{
			name:       "file not found",
			path:       "nonexistent.yaml",
			expectFind: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, found := metadata.GetFile(tt.path)

			assert.Equal(t, tt.expectFind, found)

			if tt.expectFind && tt.validate != nil {
				tt.validate(t, file)
			}
		})
	}
}

func TestGenerationMetadata_IsFileGenerated(t *testing.T) {
	metadata := &GenerationMetadata{
		Version:     1,
		Command:     "atmos init",
		Template:    TemplateInfo{Name: "simple"},
		GeneratedAt: time.Now(),
		Variables:   map[string]string{},
		Files: []GeneratedFile{
			{Path: "atmos.yaml", Checksum: "abc123"},
			{Path: "stacks/prod.yaml", Checksum: "def456"},
		},
		StorageType: "file",
	}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "file is generated",
			path:     "atmos.yaml",
			expected: true,
		},
		{
			name:     "second file is generated",
			path:     "stacks/prod.yaml",
			expected: true,
		},
		{
			name:     "file is not generated",
			path:     "custom-file.yaml",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := metadata.IsFileGenerated(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetermineStorageType(t *testing.T) {
	tests := []struct {
		name     string
		baseRef  string
		expected string
	}{
		{
			name:     "git ref provided",
			baseRef:  "main",
			expected: "git",
		},
		{
			name:     "git tag provided",
			baseRef:  "v1.0.0",
			expected: "git",
		},
		{
			name:     "empty baseRef",
			baseRef:  "",
			expected: "file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineStorageType(tt.baseRef)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMetadataStorage_RoundTrip(t *testing.T) {
	// Test complete save and load cycle
	tmpDir := t.TempDir()
	metadataPath := filepath.Join(tmpDir, ".atmos", "init", "metadata.yaml")

	storage := NewMetadataStorage(metadataPath)

	// Create metadata
	original := NewInitMetadata(
		"simple",
		"1.0.0",
		"embedded",
		"main",
		map[string]string{
			"project_name": "test-project",
			"author":       "test-author",
		},
	)

	// Add files
	original.AddFile("atmos.yaml", "templates/atmos.yaml", "abc123")
	original.AddFile("stacks/prod.yaml", "templates/stack.yaml", "def456")

	// Save
	err := storage.Save(original)
	require.NoError(t, err)

	// Verify file exists
	assert.True(t, storage.Exists())

	// Load
	loaded, err := storage.Load()
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// Verify all fields match
	assert.Equal(t, original.Version, loaded.Version)
	assert.Equal(t, original.Command, loaded.Command)
	assert.Equal(t, original.Template.Name, loaded.Template.Name)
	assert.Equal(t, original.Template.Version, loaded.Template.Version)
	assert.Equal(t, original.Template.Source, loaded.Template.Source)
	assert.Equal(t, original.BaseRef, loaded.BaseRef)
	assert.Equal(t, original.Variables, loaded.Variables)
	assert.Equal(t, original.StorageType, loaded.StorageType)
	assert.Len(t, loaded.Files, 2)

	// Verify files
	file1, found := loaded.GetFile("atmos.yaml")
	assert.True(t, found)
	assert.Equal(t, "abc123", file1.Checksum)

	file2, found := loaded.GetFile("stacks/prod.yaml")
	assert.True(t, found)
	assert.Equal(t, "def456", file2.Checksum)
}
