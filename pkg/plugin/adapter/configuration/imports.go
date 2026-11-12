package configuration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/configuration"
	"github.com/cloudposse/atmos/pkg/perf"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// importRejectedCharacters cannot appear in a provider import path.
const importRejectedCharacters = `\?#*[{`

// ImportComponents resolves an explicit provider import into Terraform components of the
// importing stack. The provider evaluates only the imported roots; automatic discovery
// scope (included_paths and excluded_paths) never filters an explicit import. The units
// keep their provider names but drop the provider's derived stack, so the same unit can be
// imported by several stacks, or through a mixin, and each stack gets its own component.
func (s *Session) ImportComponents(ctx context.Context, uri string) (map[string]any, bool, error) {
	defer perf.Track(nil, "configuration.Session.ImportComponents")()

	return s.importComponents(ctx, uri, false)
}

func (s *Session) importComponents(ctx context.Context, uri string, validation bool) (map[string]any, bool, error) {
	name, path, found := strings.Cut(uri, "://")
	if !found {
		return nil, false, nil
	}
	template := s.importProviders[name]
	if template == nil {
		// A configured provider with invalid settings reports them, not a remote download error.
		if err := s.settingsErrs[name]; err != nil {
			return nil, true, err
		}
		return nil, false, nil
	}
	key, err := s.importKey(ctx, name, template.Provider, path)
	if err != nil {
		return nil, true, err
	}

	instances, err := s.resolveCachedImport(ctx, template, key, validation)
	if err != nil {
		return nil, true, err
	}
	components, err := importedComponents(instances)
	return components, true, err
}

// resolveCachedImport serializes import evaluation and orders executable imports
// before discovery starts. Validation reads share results without changing inventory.
func (s *Session) resolveCachedImport(ctx context.Context, template *pluginv1.DiscoverComponentsRequest, key string, validation bool) ([]*pluginv1.DiscoveredComponent, error) {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	instances, resolved := s.importResults[key]
	if validation && !resolved {
		instances, resolved = s.validationImports[key]
	}
	if resolved {
		return instances, nil
	}
	if s.started && !validation {
		return nil, fmt.Errorf("%w: cannot add import %q after configuration discovery", errUtils.ErrPluginInvalidConfig, key)
	}
	instances, err := s.resolveImport(ctx, template, key)
	if err != nil {
		return nil, err
	}
	s.cacheImport(key, instances, validation)
	return instances, nil
}

// cacheImport stores successful results under importMu. Validation-only units must
// never become discovery exclusions or preparation sources.
func (s *Session) cacheImport(key string, instances []*pluginv1.DiscoveredComponent, validation bool) {
	if validation {
		if s.validationImports == nil {
			s.validationImports = make(map[string][]*pluginv1.DiscoveredComponent)
		}
		s.validationImports[key] = instances
		return
	}
	s.importResults[key] = instances
	for _, instance := range instances {
		s.imported[instance.Source] = instance
	}
}

// importedComponents translates resolved units without changing the cached provider results.
func importedComponents(instances []*pluginv1.DiscoveredComponent) (map[string]any, error) {
	components := make(map[string]any, len(instances))
	for _, instance := range instances {
		config, err := instanceConfig(instance, true)
		if err != nil {
			return nil, err
		}
		components[instance.Address.Component] = config
	}
	return components, nil
}

// importKey validates an import path and returns its canonical URI. The path must be a
// project-relative file or directory that exists. The scheme may be a configuration alias, so
// errors name the import as written while the key uses the provider's own name, the only one
// the provider recognizes.
func (s *Session) importKey(ctx context.Context, scheme, provider, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := s.ctx.Err(); err != nil {
		return "", err
	}
	// Providers read the path as a pattern root, so glob and escape characters are rejected
	// here, naming the import and the reason, even inside a literal directory name.
	if !filepath.IsLocal(path) || strings.ContainsAny(path, importRejectedCharacters) {
		return "", fmt.Errorf("%w: provider import %s must name a project-relative file or directory without any of the characters %s", errUtils.ErrPluginInvalidConfig, scheme+"://"+path, importRejectedCharacters)
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	key := provider + "://" + clean
	if _, err := os.Stat(filepath.Join(s.root, filepath.FromSlash(clean))); err != nil {
		return "", errUtils.Build(fmt.Errorf("%w: import %q: %s does not exist in the project", errUtils.ErrPluginInvalidConfig, scheme+"://"+clean, clean)).
			WithHint("Provider imports name a file or directory relative to the Atmos base path (`base_path` in `atmos.yaml`), not to the importing stack file").
			Err()
	}
	return key, nil
}

// resolveImport asks the provider for the units under one import. The caller holds importMu.
func (s *Session) resolveImport(ctx context.Context, template *pluginv1.DiscoverComponentsRequest, key string) ([]*pluginv1.DiscoveredComponent, error) {
	// Keep caller deadlines and values while also stopping work when the session ends.
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer cancel()
	// Cancellation may have happened while this import waited for importMu.
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
			return nil, configuration.ImportConflict(instance.Address.Component, previous.Source, instance.Source, key)
		}
		names[instance.Address.Component] = instance
	}
	return discovered.Components, nil
}

// ForValidation shares cached imports but never registers additional executable units.
func (s *Session) ForValidation() schema.ComponentConfigurationRuntime {
	defer perf.Track(nil, "configuration.Session.ForValidation")()
	return &validationSession{Session: s}
}

type validationSession struct{ *Session }

func (s *validationSession) ImportComponents(ctx context.Context, uri string) (map[string]any, bool, error) {
	defer perf.Track(nil, "configuration.validationSession.ImportComponents")()
	return s.importComponents(ctx, uri, true)
}
