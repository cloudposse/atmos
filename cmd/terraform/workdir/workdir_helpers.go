package workdir

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
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

	// SourceType is the type of source ("local" or "remote").
	SourceType string `json:"source_type" yaml:"source_type"`

	// SourceURI is the remote source URI (only for remote sources).
	SourceURI string `json:"source_uri,omitempty" yaml:"source_uri,omitempty"`

	// SourceVersion is the remote source version (only for remote sources).
	SourceVersion string `json:"source_version,omitempty" yaml:"source_version,omitempty"`

	// Path is the workdir path relative to project root.
	Path string `json:"path" yaml:"path"`

	// ContentHash is a hash of the source content.
	ContentHash string `json:"content_hash,omitempty" yaml:"content_hash,omitempty"`

	// CreatedAt is when the workdir was created.
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// UpdatedAt is when the workdir was last updated.
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`

	// LastAccessed is when the workdir was last accessed.
	LastAccessed time.Time `json:"last_accessed" yaml:"last_accessed"`
}

// WorkdirManager defines the interface for workdir operations.
// This interface enables dependency injection for testing.
//
//go:generate go run go.uber.org/mock/mockgen -destination=mock_workdir_manager_test.go -package=workdir -source=workdir_helpers.go WorkdirManager
type WorkdirManager interface {
	// ListWorkdirs returns all workdirs in the project.
	ListWorkdirs(atmosConfig *schema.AtmosConfiguration) ([]WorkdirInfo, error)

	// GetWorkdirInfo returns information about a specific workdir. componentConfig is passed
	// through to provWorkdir.BuildPath so an "atmos_component" instance-name override (and the
	// component-name escaping BuildPath applies) resolves the same on-disk path
	// Service.Provision actually created. May be nil.
	GetWorkdirInfo(atmosConfig *schema.AtmosConfiguration, component, stack string, componentConfig map[string]any) (*WorkdirInfo, error)

	// DescribeWorkdir returns a valid stack manifest snippet for the workdir. componentConfig
	// is forwarded to GetWorkdirInfo; see its doc comment. May be nil.
	DescribeWorkdir(atmosConfig *schema.AtmosConfiguration, component, stack string, componentConfig map[string]any) (string, error)

	// CleanWorkdir removes a specific workdir. componentConfig is forwarded to
	// provWorkdir.BuildPath; see GetWorkdirInfo's doc comment. May be nil.
	CleanWorkdir(atmosConfig *schema.AtmosConfiguration, component, stack string, componentConfig map[string]any) error

	// CleanAllWorkdirs removes all workdirs. If dryRun is true, only reports what would be
	// cleaned.
	CleanAllWorkdirs(atmosConfig *schema.AtmosConfiguration, dryRun bool) error

	// CleanExpiredWorkdirs removes workdirs older than the specified TTL.
	// If dryRun is true, only reports what would be cleaned.
	CleanExpiredWorkdirs(atmosConfig *schema.AtmosConfiguration, ttl string, dryRun bool) error
}

// DefaultWorkdirManager is the default implementation of WorkdirManager.
type DefaultWorkdirManager struct{}

// NewDefaultWorkdirManager creates a new DefaultWorkdirManager.
func NewDefaultWorkdirManager() *DefaultWorkdirManager {
	return &DefaultWorkdirManager{}
}

// resolveComponentConfig best-effort resolves component's stack config, for callers that need
// to pass it to a WorkdirManager method so provWorkdir.BuildPath can honor an "atmos_component"
// instance-name override. Returns nil (not an error) when the component can no longer be
// resolved in the current stack config -- e.g. it was since removed from stack manifests --
// since workdir get/describe/clean must still work against an orphaned, no-longer-configured
// workdir; callers fall back to treating component as its own instance name in that case,
// matching the pre-override behavior.
//
// Takes the same ConfigAndStacksInfo the caller already built from CLI flags (base-path,
// config, config-path, profile) rather than an already-loaded *schema.AtmosConfiguration.
// Describing a component requires stacks to be discovered and processed
// (cfg.InitCliConfig's processStacks=true path); the caller's own AtmosConfiguration
// intentionally skips that (processStacks=false), since BuildPath and friends only need
// base_path, not the full stack list. Re-deriving a fully-processed config here, from the
// same flag overrides, is what lets a real "atmos_component" instance-name override actually
// resolve instead of ExecuteDescribeComponent always failing to find the component (and this
// function silently falling back to nil on every call). Takes configAndStacksInfo by pointer
// only to avoid copying its large struct on every call; it is never mutated -- a local copy is
// taken internally before setting the per-call ComponentFromArg/Stack fields.
func resolveComponentConfig(configAndStacksInfo *schema.ConfigAndStacksInfo, component, stack string) map[string]any {
	infoCopy := *configAndStacksInfo
	infoCopy.ComponentFromArg = component
	infoCopy.Stack = stack

	atmosConfig, err := cfg.InitCliConfig(infoCopy, true)
	if err != nil {
		log.Debug("Could not load atmos configuration for workdir path resolution; falling back to component name",
			"component", component, "stack", stack, "error", err)
		return nil
	}

	componentConfig, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		AtmosConfig:          &atmosConfig,
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          nil,
	})
	if err != nil {
		log.Debug("Could not resolve component config for workdir path resolution; falling back to component name",
			"component", component, "stack", stack, "error", err)
		return nil
	}
	return componentConfig
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

		workdirPath := filepath.Join(workdirBase, entry.Name())
		metadata, err := provWorkdir.ReadMetadata(workdirPath)
		if err != nil || metadata == nil {
			// Skip directories with invalid or missing metadata.
			// This allows the list operation to succeed even if some workdirs are corrupt.
			continue
		}

		workdirs = append(workdirs, WorkdirInfo{
			Name:          entry.Name(),
			Component:     metadata.Component,
			Stack:         metadata.Stack,
			Source:        metadata.Source,
			SourceType:    string(metadata.SourceType),
			SourceURI:     metadata.SourceURI,
			SourceVersion: metadata.SourceVersion,
			Path:          filepath.Join(provWorkdir.WorkdirPath, terraformSubdir, entry.Name()),
			ContentHash:   metadata.ContentHash,
			CreatedAt:     metadata.CreatedAt,
			UpdatedAt:     metadata.UpdatedAt,
			LastAccessed:  metadata.LastAccessed,
		})
	}

	return workdirs, nil
}

// GetWorkdirInfo returns information about a specific workdir.
func (m *DefaultWorkdirManager) GetWorkdirInfo(atmosConfig *schema.AtmosConfiguration, component, stack string, componentConfig map[string]any) (*WorkdirInfo, error) {
	defer perf.Track(atmosConfig, "workdir.GetWorkdirInfo")()

	workdirPath, err := provWorkdir.BuildPath(atmosConfig.BasePath, terraformSubdir, component, stack, componentConfig)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("Failed to resolve component workdir path").
			WithContext("component", component).
			WithContext("stack", stack).
			Err()
	}
	workdirName := filepath.Base(workdirPath)

	metadata, err := provWorkdir.ReadMetadata(workdirPath)
	if err != nil || metadata == nil {
		return nil, errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation(fmt.Sprintf("Workdir not found for component '%s' in stack '%s'", component, stack)).
			WithHint("Run 'atmos terraform init' to create the workdir").
			WithContext("component", component).
			WithContext("stack", stack).
			Err()
	}

	return &WorkdirInfo{
		Name:          workdirName,
		Component:     metadata.Component,
		Stack:         metadata.Stack,
		Source:        metadata.Source,
		SourceType:    string(metadata.SourceType),
		SourceURI:     metadata.SourceURI,
		SourceVersion: metadata.SourceVersion,
		Path:          filepath.Join(provWorkdir.WorkdirPath, terraformSubdir, workdirName),
		ContentHash:   metadata.ContentHash,
		CreatedAt:     metadata.CreatedAt,
		UpdatedAt:     metadata.UpdatedAt,
		LastAccessed:  metadata.LastAccessed,
	}, nil
}

// DescribeWorkdir returns a valid stack manifest snippet for the workdir.
func (m *DefaultWorkdirManager) DescribeWorkdir(atmosConfig *schema.AtmosConfiguration, component, stack string, componentConfig map[string]any) (string, error) {
	defer perf.Track(atmosConfig, "workdir.DescribeWorkdir")()

	info, err := m.GetWorkdirInfo(atmosConfig, component, stack, componentConfig)
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
func (m *DefaultWorkdirManager) CleanWorkdir(atmosConfig *schema.AtmosConfiguration, component, stack string, componentConfig map[string]any) error {
	defer perf.Track(atmosConfig, "workdir.CleanWorkdir")()

	workdirPath, err := provWorkdir.BuildPath(atmosConfig.BasePath, terraformSubdir, component, stack, componentConfig)
	if err != nil {
		return errUtils.Build(errUtils.ErrWorkdirClean).
			WithCause(err).
			WithExplanation("Failed to resolve component workdir path").
			WithContext("component", component).
			WithContext("stack", stack).
			Err()
	}

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

// CleanAllWorkdirs removes all workdirs. If dryRun is true, only reports what would be cleaned.
// Delegates to provWorkdir.CleanAllWorkdirs -- the single canonical implementation -- the same
// way CleanExpiredWorkdirs below already delegates to provWorkdir.CleanExpiredWorkdirs, rather
// than reimplementing removal (and dryRun handling) a second time here.
func (m *DefaultWorkdirManager) CleanAllWorkdirs(atmosConfig *schema.AtmosConfiguration, dryRun bool) error {
	defer perf.Track(atmosConfig, "workdir.CleanAllWorkdirs")()

	return provWorkdir.CleanAllWorkdirs(atmosConfig, dryRun)
}

// CleanExpiredWorkdirs removes workdirs older than the specified TTL.
// If dryRun is true, only reports what would be cleaned.
func (m *DefaultWorkdirManager) CleanExpiredWorkdirs(atmosConfig *schema.AtmosConfiguration, ttl string, dryRun bool) error {
	defer perf.Track(atmosConfig, "workdir.CleanExpiredWorkdirs")()

	// Use the provisioner workdir package's CleanExpiredWorkdirs.
	return provWorkdir.CleanExpiredWorkdirs(atmosConfig, ttl, dryRun)
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
