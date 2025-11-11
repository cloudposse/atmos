package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// GenerationMetadata tracks what was generated from a template.
// This is stored in .atmos/init/metadata.yaml (for init) or
// .atmos/scaffold/metadata.yaml (for scaffold) to enable updates.
//
// This is different from scaffold.yaml which defines the template itself.
// This metadata tracks what was actually generated from that template.
type GenerationMetadata struct {
	Version     int               `yaml:"version"`            // Metadata format version
	Command     string            `yaml:"command"`            // "atmos init" or "atmos scaffold generate"
	Template    TemplateInfo      `yaml:"template"`           // Which template was used
	BaseRef     string            `yaml:"base_ref,omitempty"` // Git ref used as base (if git-based)
	GeneratedAt time.Time         `yaml:"generated_at"`       // When generated
	Variables   map[string]string `yaml:"variables"`          // Template variables used
	Files       []GeneratedFile   `yaml:"files"`              // Files that were generated
	StorageType string            `yaml:"storage_type"`       // "git" or "file"
}

// TemplateInfo describes which template was used.
type TemplateInfo struct {
	Name    string `yaml:"name"`              // Template name (e.g., "simple", "atmos")
	Version string `yaml:"version,omitempty"` // Template version (from scaffold.yaml)
	Source  string `yaml:"source,omitempty"`  // "embedded", "atmos.yaml", or git URL
}

// GeneratedFile tracks a single file that was generated.
type GeneratedFile struct {
	Path         string `yaml:"path"`                    // Relative path from project root
	TemplatePath string `yaml:"template_path,omitempty"` // Path within template
	Checksum     string `yaml:"checksum"`                // SHA256 of generated content
}

// MetadataStorage handles reading and writing generation metadata.
type MetadataStorage struct {
	metadataPath string // Path to metadata.yaml file
}

// NewMetadataStorage creates a new metadata storage for the given metadata file path.
// For init: .atmos/init/metadata.yaml
// For scaffold: .atmos/scaffold/metadata.yaml
func NewMetadataStorage(metadataPath string) *MetadataStorage {
	return &MetadataStorage{
		metadataPath: metadataPath,
	}
}

// Load reads the generation metadata from disk.
// Returns nil if the file doesn't exist (first generation).
func (s *MetadataStorage) Load() (*GenerationMetadata, error) {
	// Check if file exists
	if _, err := os.Stat(s.metadataPath); os.IsNotExist(err) {
		return nil, nil // No metadata yet - this is OK
	}

	data, err := os.ReadFile(s.metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file %s: %w", s.metadataPath, err)
	}

	var metadata GenerationMetadata
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata file %s: %w", s.metadataPath, err)
	}

	return &metadata, nil
}

// Save writes the generation metadata to disk.
// Creates parent directories if they don't exist.
func (s *MetadataStorage) Save(metadata *GenerationMetadata) error {
	// Ensure parent directory exists
	dir := filepath.Dir(s.metadataPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create metadata directory %s: %w", dir, err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Write to file
	if err := os.WriteFile(s.metadataPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write metadata file %s: %w", s.metadataPath, err)
	}

	return nil
}

// Exists checks if the metadata file exists.
func (s *MetadataStorage) Exists() bool {
	_, err := os.Stat(s.metadataPath)
	return err == nil
}

// GetMetadataPath returns the path to the metadata file.
func (s *MetadataStorage) GetMetadataPath() string {
	return s.metadataPath
}

// NewInitMetadata creates generation metadata for an init command.
func NewInitMetadata(templateName, templateVersion, baseRef string, variables map[string]string) *GenerationMetadata {
	return &GenerationMetadata{
		Version: 1,
		Command: "atmos init",
		Template: TemplateInfo{
			Name:    templateName,
			Version: templateVersion,
			Source:  "embedded", // Could be enhanced to track source
		},
		BaseRef:     baseRef,
		GeneratedAt: time.Now(),
		Variables:   variables,
		Files:       []GeneratedFile{},
		StorageType: determineStorageType(baseRef),
	}
}

// NewScaffoldMetadata creates generation metadata for a scaffold command.
func NewScaffoldMetadata(templateName, templateVersion, baseRef string, variables map[string]string) *GenerationMetadata {
	return &GenerationMetadata{
		Version: 1,
		Command: "atmos scaffold generate",
		Template: TemplateInfo{
			Name:    templateName,
			Version: templateVersion,
			Source:  "embedded",
		},
		BaseRef:     baseRef,
		GeneratedAt: time.Now(),
		Variables:   variables,
		Files:       []GeneratedFile{},
		StorageType: determineStorageType(baseRef),
	}
}

// AddFile adds a generated file to the metadata.
func (m *GenerationMetadata) AddFile(path, templatePath, checksum string) {
	m.Files = append(m.Files, GeneratedFile{
		Path:         path,
		TemplatePath: templatePath,
		Checksum:     checksum,
	})
}

// GetFile retrieves metadata for a specific file by path.
func (m *GenerationMetadata) GetFile(path string) *GeneratedFile {
	for i := range m.Files {
		if m.Files[i].Path == path {
			return &m.Files[i]
		}
	}
	return nil
}

// IsFileGenerated checks if a file was generated from the template.
func (m *GenerationMetadata) IsFileGenerated(path string) bool {
	return m.GetFile(path) != nil
}

// determineStorageType infers the storage type from the base ref.
func determineStorageType(baseRef string) string {
	if baseRef == "" {
		return "file" // No git ref specified
	}
	return "git"
}
