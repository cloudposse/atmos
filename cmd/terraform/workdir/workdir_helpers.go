package workdir

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// terraformSubdir is the subdirectory name for terraform workdirs.
const terraformSubdir = "terraform"

// WorkdirInfo contains information about a workdir for display purposes.
type WorkdirInfo struct {
	// Name is the workdir directory name (e.g., "dev-vpc").
	Name string `json:"name" yaml:"name"`

	// Component is the component name.
	Component string `json:"component" yaml:"component"`

	// Stack is the stack name.
	Stack string `json:"stack" yaml:"stack"`

	// Source is the source path (component folder).
	Source string `json:"source" yaml:"source"`

	// Path is the workdir path relative to project root.
	Path string `json:"path" yaml:"path"`

	// ContentHash is a hash of the source content.
	ContentHash string `json:"content_hash,omitempty" yaml:"content_hash,omitempty"`

	// CreatedAt is when the workdir was created.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// UpdatedAt is when the workdir was last updated.
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`
}

// WorkdirManager defines the interface for workdir operations.
// This interface enables dependency injection for testing.
type WorkdirManager interface {
	// ListWorkdirs returns all workdirs in the project.
	ListWorkdirs(atmosConfig *schema.AtmosConfiguration) ([]WorkdirInfo, error)

	// GetWorkdirInfo returns information about a specific workdir.
	GetWorkdirInfo(atmosConfig *schema.AtmosConfiguration, component, stack string) (*WorkdirInfo, error)

	// DescribeWorkdir returns a valid stack manifest snippet for the workdir.
	DescribeWorkdir(atmosConfig *schema.AtmosConfiguration, component, stack string) (string, error)

	// CleanWorkdir removes a specific workdir.
	CleanWorkdir(atmosConfig *schema.AtmosConfiguration, component, stack string) error

	// CleanAllWorkdirs removes all workdirs.
	CleanAllWorkdirs(atmosConfig *schema.AtmosConfiguration) error
}

// DefaultWorkdirManager is the default implementation of WorkdirManager.
type DefaultWorkdirManager struct{}

// NewDefaultWorkdirManager creates a new DefaultWorkdirManager.
func NewDefaultWorkdirManager() *DefaultWorkdirManager {
	return &DefaultWorkdirManager{}
}

// ListWorkdirs returns all workdirs in the project.
func (m *DefaultWorkdirManager) ListWorkdirs(atmosConfig *schema.AtmosConfiguration) ([]WorkdirInfo, error) {
	defer perf.Track(atmosConfig, "workdir.ListWorkdirs")()

	workdirBase := filepath.Join(atmosConfig.BasePath, provWorkdir.WorkdirPath, terraformSubdir)

	// Check if workdir directory exists.
	if _, err := os.Stat(workdirBase); os.IsNotExist(err) {
		return []WorkdirInfo{}, nil
	}

	entries, err := os.ReadDir(workdirBase)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("Failed to read workdir directory").
			WithContext("path", workdirBase).
			Err()
	}

	var workdirs []WorkdirInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		metadataPath := filepath.Join(workdirBase, entry.Name(), provWorkdir.WorkdirMetadataFile)
		metadata, err := readWorkdirMetadata(metadataPath)
		if err != nil {
			// Skip directories without valid metadata.
			continue
		}

		workdirs = append(workdirs, WorkdirInfo{
			Name:        entry.Name(),
			Component:   metadata.Component,
			Stack:       metadata.Stack,
			Source:      metadata.LocalPath,
			Path:        filepath.Join(provWorkdir.WorkdirPath, terraformSubdir, entry.Name()),
			ContentHash: metadata.ContentHash,
			CreatedAt:   metadata.CreatedAt,
			UpdatedAt:   metadata.UpdatedAt,
		})
	}

	return workdirs, nil
}

// GetWorkdirInfo returns information about a specific workdir.
func (m *DefaultWorkdirManager) GetWorkdirInfo(atmosConfig *schema.AtmosConfiguration, component, stack string) (*WorkdirInfo, error) {
	defer perf.Track(atmosConfig, "workdir.GetWorkdirInfo")()

	workdirName := fmt.Sprintf("%s-%s", stack, component)
	workdirPath := filepath.Join(atmosConfig.BasePath, provWorkdir.WorkdirPath, terraformSubdir, workdirName)
	metadataPath := filepath.Join(workdirPath, provWorkdir.WorkdirMetadataFile)

	metadata, err := readWorkdirMetadata(metadataPath)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation(fmt.Sprintf("Workdir not found for component '%s' in stack '%s'", component, stack)).
			WithHint("Run 'atmos terraform init' to create the workdir").
			WithContext("component", component).
			WithContext("stack", stack).
			Err()
	}

	return &WorkdirInfo{
		Name:        workdirName,
		Component:   metadata.Component,
		Stack:       metadata.Stack,
		Source:      metadata.LocalPath,
		Path:        filepath.Join(provWorkdir.WorkdirPath, terraformSubdir, workdirName),
		ContentHash: metadata.ContentHash,
		CreatedAt:   metadata.CreatedAt,
		UpdatedAt:   metadata.UpdatedAt,
	}, nil
}

// DescribeWorkdir returns a valid stack manifest snippet for the workdir.
func (m *DefaultWorkdirManager) DescribeWorkdir(atmosConfig *schema.AtmosConfiguration, component, stack string) (string, error) {
	defer perf.Track(atmosConfig, "workdir.DescribeWorkdir")()

	info, err := m.GetWorkdirInfo(atmosConfig, component, stack)
	if err != nil {
		return "", err
	}

	// Build the manifest structure.
	manifest := map[string]any{
		"components": map[string]any{
			terraformSubdir: map[string]any{
				component: map[string]any{
					"metadata": map[string]any{
						"workdir": map[string]any{
							"name":         info.Name,
							"source":       info.Source,
							"path":         info.Path,
							"content_hash": info.ContentHash,
							"created_at":   info.CreatedAt.Format(time.RFC3339),
							"updated_at":   info.UpdatedAt.Format(time.RFC3339),
						},
					},
				},
			},
		},
	}

	yamlBytes, err := yaml.Marshal(manifest)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("Failed to marshal workdir manifest").
			Err()
	}

	return string(yamlBytes), nil
}

// CleanWorkdir removes a specific workdir.
func (m *DefaultWorkdirManager) CleanWorkdir(atmosConfig *schema.AtmosConfiguration, component, stack string) error {
	defer perf.Track(atmosConfig, "workdir.CleanWorkdir")()

	workdirName := fmt.Sprintf("%s-%s", stack, component)
	workdirPath := filepath.Join(atmosConfig.BasePath, provWorkdir.WorkdirPath, terraformSubdir, workdirName)

	// Check if workdir exists.
	if _, err := os.Stat(workdirPath); os.IsNotExist(err) {
		return errUtils.Build(errUtils.ErrWorkdirClean).
			WithExplanation(fmt.Sprintf("Workdir not found for component '%s' in stack '%s'", component, stack)).
			WithContext("component", component).
			WithContext("stack", stack).
			Err()
	}

	if err := os.RemoveAll(workdirPath); err != nil {
		return errUtils.Build(errUtils.ErrWorkdirClean).
			WithCause(err).
			WithExplanation("Failed to remove workdir").
			WithContext("path", workdirPath).
			Err()
	}

	return nil
}

// CleanAllWorkdirs removes all workdirs.
func (m *DefaultWorkdirManager) CleanAllWorkdirs(atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "workdir.CleanAllWorkdirs")()

	workdirBase := filepath.Join(atmosConfig.BasePath, provWorkdir.WorkdirPath, terraformSubdir)

	// Check if workdir directory exists.
	if _, err := os.Stat(workdirBase); os.IsNotExist(err) {
		return nil // Nothing to clean.
	}

	if err := os.RemoveAll(workdirBase); err != nil {
		return errUtils.Build(errUtils.ErrWorkdirClean).
			WithCause(err).
			WithExplanation("Failed to remove all workdirs").
			WithContext("path", workdirBase).
			Err()
	}

	return nil
}

// readWorkdirMetadata reads and parses the workdir metadata file.
func readWorkdirMetadata(path string) (*provWorkdir.WorkdirMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var metadata provWorkdir.WorkdirMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}

	return &metadata, nil
}

// workdirManager is the default manager used by commands.
// It can be overridden for testing.
var workdirManager WorkdirManager = NewDefaultWorkdirManager()

// SetWorkdirManager sets the workdir manager (for testing).
func SetWorkdirManager(manager WorkdirManager) {
	workdirManager = manager
}

// GetWorkdirManager returns the current workdir manager.
func GetWorkdirManager() WorkdirManager {
	return workdirManager
}
