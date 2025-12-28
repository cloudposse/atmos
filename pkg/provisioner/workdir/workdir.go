package workdir

import (
	"context"
	"encoding/json"
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

	_ = ui.Info(fmt.Sprintf("Provisioning workdir for component '%s'", component))

	// 1. Create .workdir/terraform/<stack>-<component>/ directory.
	workdirPath, err := s.createWorkdirDirectory(atmosConfig, stack, component)
	if err != nil {
		return err
	}

	// 2. Copy local component files to workdir.
	metadata, err := s.copyLocalToWorkdir(atmosConfig, componentConfig, workdirPath, component, stack)
	if err != nil {
		return err
	}

	// 3. Write workdir metadata.
	if err := s.writeMetadata(workdirPath, metadata); err != nil {
		return err
	}

	// 4. Store workdir path for terraform execution.
	componentConfig[WorkdirPathKey] = workdirPath

	_ = ui.Success(fmt.Sprintf("Workdir provisioned: %s", workdirPath))
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

// copyLocalToWorkdir copies local component files to workdir.
func (s *Service) copyLocalToWorkdir(
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	workdirPath, component, stack string,
) (*WorkdirMetadata, error) {
	defer perf.Track(atmosConfig, "workdir.Service.copyLocalToWorkdir")()

	// Get component path.
	componentPath := extractComponentPath(atmosConfig, componentConfig, component)
	if componentPath == "" {
		return nil, errUtils.Build(errUtils.ErrWorkdirProvision).
			WithExplanation("cannot determine local component path").
			WithContext("component", component).
			Err()
	}

	// Verify source exists.
	if !s.fs.Exists(componentPath) {
		return nil, errUtils.Build(errUtils.ErrWorkdirProvision).
			WithExplanation("local component path does not exist").
			WithContext("path", componentPath).
			WithHint("Check that the component exists in components/terraform/").
			Err()
	}

	_ = ui.Info(fmt.Sprintf("Copying local component: %s", componentPath))

	// Copy to workdir.
	if err := s.fs.CopyDir(componentPath, workdirPath); err != nil {
		return nil, errUtils.Build(errUtils.ErrWorkdirSync).
			WithCause(err).
			WithExplanation("failed to copy local component to workdir").
			WithContext("source", componentPath).
			WithContext("dest", workdirPath).
			Err()
	}

	// Compute content hash.
	contentHash, err := s.hasher.HashDir(workdirPath)
	if err != nil {
		_ = ui.Warning(fmt.Sprintf("Failed to compute content hash: %s", err))
	}

	now := time.Now()
	return &WorkdirMetadata{
		Component:   component,
		Stack:       stack,
		SourceType:  SourceTypeLocal,
		Source:      componentPath,
		CreatedAt:   now,
		UpdatedAt:   now,
		ContentHash: contentHash,
	}, nil
}

// writeMetadata writes workdir metadata to the metadata file.
func (s *Service) writeMetadata(workdirPath string, metadata *WorkdirMetadata) error {
	defer perf.Track(nil, "workdir.Service.writeMetadata")()

	metadataPath := filepath.Join(workdirPath, WorkdirMetadataFile)
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("failed to marshal workdir metadata").
			Err()
	}

	if err := s.fs.WriteFile(metadataPath, data, FilePermissionsStandard); err != nil {
		return errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("failed to write workdir metadata").
			WithContext("path", metadataPath).
			Err()
	}

	return nil
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
