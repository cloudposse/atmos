package workdir

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// HookEventBeforeTerraformInit is the hook event for before terraform init.
const HookEventBeforeTerraformInit = provisioner.HookEvent("before.terraform.init")

func init() {
	// Register workdir provisioner to run before terraform init.
	_ = provisioner.RegisterProvisioner(provisioner.Provisioner{
		Type:      "workdir",
		HookEvent: HookEventBeforeTerraformInit,
		Func:      ProvisionWorkdir,
	})
}

// Service coordinates workdir provisioning operations.
// The workdir provisioner copies local component files to an isolated working directory.
type Service struct {
	fs     FileSystem
	hasher Hasher
}

// NewService creates a new workdir service with default implementations.
func NewService() *Service {
	defer perf.Track(nil, "workdir.NewService")()

	return &Service{
		fs:     NewDefaultFileSystem(),
		hasher: NewDefaultHasher(),
	}
}

// NewServiceWithDeps creates a new workdir service with injected dependencies.
func NewServiceWithDeps(fs FileSystem, hasher Hasher) *Service {
	defer perf.Track(nil, "workdir.NewServiceWithDeps")()

	return &Service{
		fs:     fs,
		hasher: hasher,
	}
}

// ProvisionWorkdir creates an isolated working directory and populates it with component files.
// This is the main provisioner function registered with the provisioner registry.
//
// Activation rules:
// - Runs if provision.workdir.enabled: true (explicit opt-in for local components)
// - Does nothing otherwise (terraform runs in original component directory).
func ProvisionWorkdir(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	authContext *schema.AuthContext,
) error {
	defer perf.Track(atmosConfig, "workdir.ProvisionWorkdir")()

	service := NewService()
	return service.Provision(ctx, atmosConfig, componentConfig)
}

// Provision creates an isolated working directory and populates it with component files.
func (s *Service) Provision(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
) error {
	defer perf.Track(atmosConfig, "workdir.Service.Provision")()

	// Check activation condition.
	if !isWorkdirEnabled(componentConfig) {
		// No workdir needed - terraform runs in original directory.
		return nil
	}

	// Check if source provisioner already handled the workdir.
	// When source + workdir are both enabled, source downloads directly to workdir,
	// so we skip the workdir copy step.
	if _, ok := componentConfig[WorkdirPathKey].(string); ok {
		// Source provisioner already set the workdir path - skip workdir provisioning.
		return nil
	}

	// Get component name.
	component, ok := componentConfig[ComponentKey].(string)
	if !ok {
		component = extractComponentName(componentConfig)
	}
	if component == "" {
		return errUtils.Build(errUtils.ErrWorkdirProvision).
			WithExplanation("component name not found in configuration").
			Err()
	}

	// Get stack name for stack-specific workdir path.
	stack, _ := componentConfig["atmos_stack"].(string)
	if stack == "" {
		return errUtils.Build(errUtils.ErrWorkdirProvision).
			WithExplanation("stack name not found in configuration").
			WithHint("The 'atmos_stack' field is required for workdir provisioning").
			Err()
	}

	ui.Info(fmt.Sprintf("Provisioning workdir for component '%s'", component))

	// 1. Create .workdir/terraform/<stack>-<component>/ directory.
	workdirPath, err := s.createWorkdirDirectory(atmosConfig, stack, component)
	if err != nil {
		return err
	}

	// 2. Sync local component files to workdir (incremental, per-file checksum).
	metadata, changed, err := s.syncLocalToWorkdir(atmosConfig, componentConfig, workdirPath, component, stack)
	if err != nil {
		return err
	}

	// 3. Write workdir metadata (uses atomic write to .atmos/metadata.json).
	if err := WriteMetadata(workdirPath, metadata); err != nil {
		return err
	}

	// 4. Store workdir path for terraform execution.
	componentConfig[WorkdirPathKey] = workdirPath

	if changed {
		ui.Success(fmt.Sprintf("Workdir provisioned: %s", workdirPath))
	} else {
		ui.Success(fmt.Sprintf("Workdir ready (no changes): %s", workdirPath))
	}
	return nil
}

// createWorkdirDirectory creates the workdir directory structure.
// Uses stack-component naming (e.g., "dev-vpc") for isolation between stacks.
func (s *Service) createWorkdirDirectory(atmosConfig *schema.AtmosConfiguration, stack, component string) (string, error) {
	defer perf.Track(atmosConfig, "workdir.Service.createWorkdirDirectory")()

	basePath := atmosConfig.BasePath
	if basePath == "" {
		basePath = "."
	}

	// Use stack-component naming for proper isolation between stacks.
	workdirName := fmt.Sprintf("%s-%s", stack, component)
	workdirPath := filepath.Join(basePath, WorkdirPath, "terraform", workdirName)

	if err := s.fs.MkdirAll(workdirPath, DirPermissions); err != nil {
		return "", errUtils.Build(errUtils.ErrWorkdirCreation).
			WithCause(err).
			WithExplanation("failed to create workdir directory").
			WithContext("path", workdirPath).
			Err()
	}

	return workdirPath, nil
}

// syncLocalToWorkdir syncs local component files to workdir using incremental per-file checksums.
// Returns the metadata and a boolean indicating if any changes were made.
func (s *Service) syncLocalToWorkdir(
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	workdirPath, component, stack string,
) (*WorkdirMetadata, bool, error) {
	defer perf.Track(atmosConfig, "workdir.Service.syncLocalToWorkdir")()

	componentPath, err := s.validateComponentPath(atmosConfig, componentConfig, component)
	if err != nil {
		return nil, false, err
	}

	existingMetadata, _ := ReadMetadata(workdirPath)

	changed, err := s.fs.SyncDir(componentPath, workdirPath, s.hasher)
	if err != nil {
		return nil, false, errUtils.Build(errUtils.ErrWorkdirSync).
			WithCause(err).
			WithExplanation("failed to sync local component to workdir").
			WithContext("source", componentPath).
			WithContext("dest", workdirPath).
			Err()
	}

	if changed {
		ui.Info(fmt.Sprintf("Local component files synced: %s", componentPath))
	}

	contentHash := s.computeContentHash(workdirPath)
	metadata := buildLocalMetadata(&localMetadataParams{
		component:        component,
		stack:            stack,
		componentPath:    componentPath,
		contentHash:      contentHash,
		existingMetadata: existingMetadata,
		changed:          changed,
	})

	return metadata, changed, nil
}

// validateComponentPath extracts and validates the component path.
func (s *Service) validateComponentPath(
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	component string,
) (string, error) {
	componentPath := extractComponentPath(atmosConfig, componentConfig, component)
	if componentPath == "" {
		return "", errUtils.Build(errUtils.ErrWorkdirProvision).
			WithExplanation("cannot determine local component path").
			WithContext("component", component).
			Err()
	}

	if !s.fs.Exists(componentPath) {
		return "", errUtils.Build(errUtils.ErrWorkdirProvision).
			WithExplanation("local component path does not exist").
			WithContext("path", componentPath).
			WithHint(fmt.Sprintf("Check that the component exists at %s", componentPath)).
			Err()
	}

	return componentPath, nil
}

// computeContentHash computes the content hash, logging a warning on failure.
func (s *Service) computeContentHash(workdirPath string) string {
	contentHash, err := s.hasher.HashDir(workdirPath)
	if err != nil {
		ui.Warning(fmt.Sprintf("Failed to compute content hash: %s", err))
		return ""
	}
	return contentHash
}

// localMetadataParams holds parameters for building local workdir metadata.
type localMetadataParams struct {
	component        string
	stack            string
	componentPath    string
	contentHash      string
	existingMetadata *WorkdirMetadata
	changed          bool
}

// buildLocalMetadata creates metadata for a local workdir, preserving timestamps from existing metadata.
func buildLocalMetadata(params *localMetadataParams) *WorkdirMetadata {
	now := time.Now()
	metadata := &WorkdirMetadata{
		Component:    params.component,
		Stack:        params.stack,
		SourceType:   SourceTypeLocal,
		Source:       params.componentPath,
		CreatedAt:    now,
		UpdatedAt:    now,
		LastAccessed: now,
		ContentHash:  params.contentHash,
	}

	if params.existingMetadata != nil {
		metadata.CreatedAt = params.existingMetadata.CreatedAt
		if !params.changed {
			metadata.UpdatedAt = params.existingMetadata.UpdatedAt
		}
	}

	return metadata
}

// isWorkdirEnabled checks if provision.workdir.enabled is set to true.
func isWorkdirEnabled(componentConfig map[string]any) bool {
	defer perf.Track(nil, "workdir.isWorkdirEnabled")()

	provisionConfig, ok := componentConfig["provision"].(map[string]any)
	if !ok {
		return false
	}

	workdirConfig, ok := provisionConfig["workdir"].(map[string]any)
	if !ok {
		return false
	}

	enabled, ok := workdirConfig["enabled"].(bool)
	return ok && enabled
}

// extractComponentName extracts the component name from config.
// Priority: 1) top-level "component" key, 2) metadata.component, 3) vars.component.
func extractComponentName(componentConfig map[string]any) string {
	defer perf.Track(nil, "workdir.extractComponentName")()

	// Try component field.
	if component, ok := componentConfig[ComponentKey].(string); ok && component != "" {
		return component
	}

	// Try metadata.component.
	if metadata, ok := componentConfig["metadata"].(map[string]any); ok {
		if component, ok := metadata[ComponentKey].(string); ok && component != "" {
			return component
		}
	}

	// Try vars.component as fallback.
	if vars, ok := componentConfig["vars"].(map[string]any); ok {
		if component, ok := vars[ComponentKey].(string); ok && component != "" {
			return component
		}
	}

	return ""
}

// extractComponentPath extracts the local component path.
func extractComponentPath(atmosConfig *schema.AtmosConfiguration, componentConfig map[string]any, component string) string {
	defer perf.Track(atmosConfig, "workdir.extractComponentPath")()

	basePath := atmosConfig.BasePath
	if basePath == "" {
		basePath = "."
	}

	// Check for component_path in config.
	if componentPath, ok := componentConfig["component_path"].(string); ok && componentPath != "" {
		return componentPath
	}

	// Build default path.
	componentsBasePath := atmosConfig.Components.Terraform.BasePath
	if componentsBasePath == "" {
		componentsBasePath = "components/terraform"
	}

	return filepath.Join(basePath, componentsBasePath, component)
}
