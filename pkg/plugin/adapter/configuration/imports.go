package configuration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// ImportComponents resolves an explicit provider import into Terraform components of the
// importing stack. The provider evaluates only the imported roots; automatic discovery
// scope (included_paths and excluded_paths) never filters an explicit import. The units
// keep their provider names but drop the provider's derived stack, so the same unit can be
// imported by several stacks, or through a mixin, and each stack gets its own component.
func (s *Session) ImportComponents(ctx context.Context, uri string) (map[string]any, bool, error) {
	defer perf.Track(nil, "configuration.Session.ImportComponents")()

	name, path, found := strings.Cut(uri, "://")
	if !found {
		return nil, false, nil
	}
	template := s.importProviders[name]
	if template == nil {
		return nil, false, nil
	}
	key, err := s.importKey(ctx, name, path)
	if err != nil {
		return nil, true, err
	}

	// Imports are resolved one at a time; the lock also orders them before discovery starts.
	s.importMu.Lock()
	defer s.importMu.Unlock()
	instances, resolved := s.importResults[key]
	if !resolved {
		// Stack manifests may be read again after discovery; only a new import is too late.
		if s.started {
			return nil, true, fmt.Errorf("%w: cannot add import %q after configuration discovery", errUtils.ErrPluginInvalidConfig, key)
		}
		if instances, err = s.resolveImport(ctx, template, key); err != nil {
			return nil, true, err
		}
		s.importResults[key] = instances
		for _, instance := range instances {
			s.imported[instance.Source] = instance
		}
	}
	components := make(map[string]any, len(instances))
	for _, instance := range instances {
		config, err := instanceConfig(instance, true)
		if err != nil {
			return nil, true, err
		}
		components[instance.Address.Component] = config
	}
	return components, true, nil
}

// importKey validates an import path and returns its canonical URI. The path must be a
// project-relative file or directory that exists.
func (s *Session) importKey(ctx context.Context, scheme, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := s.ctx.Err(); err != nil {
		return "", err
	}
	if !filepath.IsLocal(path) || strings.ContainsAny(path, "\\?#*") {
		return "", fmt.Errorf("%w: provider import %q must name a project-relative file or directory", errUtils.ErrPluginInvalidConfig, scheme+"://"+path)
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	key := scheme + "://" + clean
	if _, err := os.Stat(filepath.Join(s.root, filepath.FromSlash(clean))); err != nil {
		return "", errUtils.Build(fmt.Errorf("%w: import %q: %s does not exist in the project", errUtils.ErrPluginInvalidConfig, key, clean)).
			WithHint("Provider imports name a file or directory relative to the Atmos base path (`base_path` in `atmos.yaml`), not to the importing stack file").
			Err()
	}
	return key, nil
}

// resolveImport asks the provider for the units under one import. The caller holds importMu.
func (s *Session) resolveImport(ctx context.Context, template *pluginv1.DiscoverComponentsRequest, key string) ([]*pluginv1.DiscoveredComponent, error) {
	registry, err := s.providers(ctx)
	if err != nil {
		return nil, err
	}
	provider, exists := registry[template.Provider]
	if !exists || provider.ComponentType() != terraformType {
		return nil, errUtils.ErrPluginUnsupportedExtension
	}
	request := proto.Clone(template).(*pluginv1.DiscoverComponentsRequest)
	request.IncludePaths, request.ExcludePaths, request.Imports = nil, nil, []string{key}
	discovered, err := provider.Discover(ctx, request)
	if err != nil {
		return nil, err
	}
	// A broken unit that is imported by name fails the import, naming the unit: the user asked for it.
	for _, diagnostic := range discovered.Diagnostics {
		if diagnosticErr := adapter.DiagnosticError(diagnostic); diagnosticErr != nil {
			return nil, diagnosticErr
		}
		adapter.WarnDiagnostic(diagnostic)
	}
	if len(discovered.Components) == 0 {
		return nil, errUtils.Build(errUtils.ErrConfigurationImportEmpty).
			WithContext("import", key).
			WithHint("Point the import at a directory below which a `terragrunt.hcl` with a `terraform { source = ... }` block or a `terragrunt.stack.hcl` exists, or at one of those files").
			Err()
	}
	names := make(map[string]*pluginv1.DiscoveredComponent, len(discovered.Components))
	for _, instance := range discovered.Components {
		if previous := names[instance.Address.Component]; previous != nil {
			return nil, importConflict(instance.Address.Component, previous.Source, instance.Source, key)
		}
		names[instance.Address.Component] = instance
	}
	return discovered.Components, nil
}

// importConflict reports two units that would become one component of the importing stack.
func importConflict(component, first, second, where string) error {
	return errUtils.Build(fmt.Errorf("%w: units %s and %s both become component %q (%s)", errUtils.ErrConfigurationImportConflict, first, second, component, where)).
		WithContext("component", component).
		WithContext("unit", first).
		WithContext("other_unit", second).
		WithHint("Import the units from different stack files, or import a narrower directory or a single `terragrunt.hcl`").
		Err()
}
