package workdir

import (
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
)

// ComponentOptions selects the local component source and workdir namespace.
type ComponentOptions struct {
	Type       string
	SourcePath string
	// AllowMissingSource supports components whose files are entirely generated.
	AllowMissingSource bool
}

// ServiceOption configures a workdir service.
type ServiceOption func(*Service)

// WithComponent configures local component isolation without changing Terraform defaults.
func WithComponent(options ComponentOptions) ServiceOption {
	defer perf.Track(nil, "workdir.WithComponent")()
	return func(s *Service) { s.componentOptions = options }
}

// componentType preserves the Terraform namespace for callers without an explicit component type.
func (s *Service) componentType() string {
	if s.componentOptions.Type != "" {
		return s.componentOptions.Type
	}
	return config.TerraformComponentType
}

// migrateComponentWorkdir limits legacy directory migration to Terraform components.
func (s *Service) migrateComponentWorkdir(basePath, component, stack, path string) error {
	if s.componentType() != config.TerraformComponentType {
		return nil
	}
	return s.migrateLegacyWorkdir(basePath, component, stack, path)
}

// restoreTerraformLock restores instance lockfiles only for Terraform components.
func (s *Service) restoreTerraformLock(source, path string, section map[string]any) error {
	if s.componentType() != config.TerraformComponentType {
		return nil
	}
	return provisioner.RestorePerInstanceLock(source, path, section)
}

// syncComponentFiles permits a missing source only for components that generate all their files.
func (s *Service) syncComponentFiles(source, path string) (bool, error) {
	if s.componentOptions.AllowMissingSource && !s.fs.Exists(source) {
		return false, nil
	}
	return s.fs.SyncDir(source, path, s.hasher)
}
