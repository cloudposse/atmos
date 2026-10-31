// Package component adapts native configuration services without replacing component engines.
package component

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// Registry is an immutable invocation-local collection of configuration providers.
// It never mutates the global component execution registry.
type Registry struct{ providers map[string]*Provider }

// Provider owns the verified definition of one configuration provider.
type Provider struct {
	name, componentType, owner, directory, executable, schemaPath string
	expected                                                      *pluginv1.DescribeResponse
	timeout                                                       time.Duration
	capabilities                                                  map[string]bool
	schema                                                        *jsonschema.Schema
}

// NewRegistry validates the entire batch before making any provider available.
func NewRegistry(activations []*extension.Activation) (*Registry, error) {
	defer perf.Track(nil, "componentadapter.NewRegistry")()

	result := &Registry{providers: make(map[string]*Provider)}
	for _, activation := range activations {
		if err := result.add(activation); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *Registry) add(activation *extension.Activation) error {
	if err := validateActivation(activation); err != nil {
		return err
	}
	componentType := activation.Metadata["component_type"].(string)
	schemaPath := activation.Metadata["schema"].(string)
	if existing := r.providers[activation.Name]; existing != nil {
		if !existing.sameOwner(activation, componentType, schemaPath) || existing.capabilities[activation.Capability] {
			return fmt.Errorf("%w: configuration provider %s", errUtils.ErrDuplicatePluginExtension, activation.Name)
		}
		existing.capabilities[activation.Capability] = true
		return nil
	}
	compiled, err := extension.CompileSchema(activation.Directory, schemaPath)
	if err != nil {
		return err
	}
	r.providers[activation.Name] = &Provider{
		name: activation.Name, componentType: componentType, owner: activation.Package, directory: activation.Directory,
		executable: activation.Executable, schemaPath: schemaPath, timeout: activation.Timeout,
		expected: proto.Clone(activation.Description).(*pluginv1.DescribeResponse), schema: compiled,
		capabilities: map[string]bool{activation.Capability: true},
	}
	return nil
}

func validateActivation(activation *extension.Activation) error {
	if activation == nil || !extension.IsComponent(activation.Capability) || activation.Name == "" || activation.Package == "" || activation.Executable == "" || activation.Description == nil {
		return errUtils.ErrPluginInvalidConfig
	}
	if err := extension.ValidateComponent(activation.Binding); err != nil {
		return err
	}
	if !declaresCapability(activation) {
		return errUtils.ErrPluginProtocol
	}
	return nil
}

func declaresCapability(activation *extension.Activation) bool {
	if activation.Description.PluginName != activation.Package {
		return false
	}
	exported, versioned := false, false
	for _, item := range activation.Description.Exports {
		if item.GetService() == activation.Capability && item.GetKind() == activation.Name {
			exported = true
		}
	}
	for _, service := range activation.Description.Services {
		if service.GetService() == activation.Capability && service.GetMin() == 1 && service.GetMax() == 1 {
			versioned = true
		}
	}
	return exported && versioned
}

func (p *Provider) sameOwner(a *extension.Activation, componentType, schemaPath string) bool {
	return p.owner == a.Package && p.directory == a.Directory && p.executable == a.Executable &&
		p.componentType == componentType && p.schemaPath == schemaPath && p.timeout == a.Timeout && proto.Equal(p.expected, a.Description)
}

// Get looks up a configuration provider by export name.
//
//nolint:lintroller // Pure lookup or validation; exempt from performance tracking.
func (r *Registry) Get(name string) (*Provider, bool) {
	if r == nil {
		return nil, false
	}
	provider, ok := r.providers[name]
	return provider, ok
}

// Names returns a deterministic copy of the registered provider names.
//
//nolint:lintroller // Pure lookup or validation; exempt from performance tracking.
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	return slices.Sorted(maps.Keys(r.providers))
}

// Supports reports an independently negotiated configuration capability.
//
//nolint:lintroller // Pure lookup or validation; exempt from performance tracking.
func (p *Provider) Supports(capability string) bool { return p.capabilities[capability] }

// ComponentType identifies the component family this provider supplies.
//
//nolint:lintroller // Pure lookup or validation; exempt from performance tracking.
func (p *Provider) ComponentType() string { return p.componentType }

// ValidateSettings checks provider settings without starting its executable.
//
//nolint:lintroller // Pure lookup or validation; exempt from performance tracking.
func (p *Provider) ValidateSettings(settings map[string]any) error {
	if err := p.schema.Validate(settings); err != nil {
		// Schema diagnostics can contain secret field values; report only the keys involved.
		return p.settingsError(err)
	}
	return nil
}
