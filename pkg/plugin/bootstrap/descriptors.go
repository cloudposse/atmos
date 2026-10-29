// Package bootstrap composes plugin descriptors, runtimes, and existing Atmos registries.
package bootstrap

import (
	"context"
	"errors"

	errUtils "github.com/cloudposse/atmos/errors"
	authregistry "github.com/cloudposse/atmos/pkg/auth/registry"
	"github.com/cloudposse/atmos/pkg/perf"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/auth"
	"github.com/cloudposse/atmos/pkg/plugin/credential"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
)

// Descriptors declares supported extension types. Lifecycle code receives this
// registry rather than switching over a closed list of capability names.
func Descriptors() (*extension.Registry, error) {
	defer perf.Track(nil, "bootstrap.Descriptors")()

	return descriptors(nil)
}

func descriptors(constructors *ConstructorSet) (*extension.Registry, error) {
	const componentsPath = "components"
	registry := extension.New()
	for _, descriptor := range []extension.Descriptor{
		{Capability: "auth.provider", Path: []string{"auth", "providers"}, Version: 1, Validate: extension.ValidateAuth, Scaffold: "plugin-oidc"},
		{Capability: "command", Path: []string{"commands"}, Version: 1, Validate: extension.ValidateCommand},
		{Capability: "auth.identity", Path: []string{"auth", "identities"}, Version: 1, Validate: extension.ValidateAuth, Scaffold: "plugin-oidc"},
		{Capability: extension.ComponentDiscovery, Path: []string{componentsPath, "discovery"}, Version: 1, Validate: extension.ValidateComponent},
		{Capability: extension.ComponentValidation, Path: []string{componentsPath, "validation"}, Version: 1, Validate: extension.ValidateComponent},
		{Capability: extension.ComponentPreparation, Path: []string{componentsPath, "preparation"}, Version: 1, Validate: extension.ValidateComponent},
		{Capability: extension.ComponentState, Path: []string{componentsPath, "state"}, Version: 1, Validate: extension.ValidateComponent},
	} {
		if constructors != nil {
			descriptor.Activate = constructors.activator(descriptor.Capability)
		}
		if err := registry.Register(descriptor); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// ConstructorSet accumulates all package exports before atomically installing them.
type ConstructorSet struct {
	Batch       authregistry.Registration
	Services    adapter.Services
	Credentials *credential.Registry
	Commands    map[string]*extension.Activation
	Components  []*extension.Activation
}

func newConstructors(services *adapter.Services) *ConstructorSet {
	result := &ConstructorSet{Batch: authregistry.Registration{Providers: make(map[string]authregistry.ProviderConfigConstructor), Identities: make(map[string]authregistry.IdentityConfigConstructor)}, Services: *services, Credentials: credential.New(), Commands: make(map[string]*extension.Activation)}
	return result
}

func (c *ConstructorSet) provider(binding extension.Binding, definition *adapter.Definition) error {
	if _, exists := c.Batch.Providers[binding.Name]; exists {
		return errUtils.ErrDuplicateAuthRegistration
	}
	c.Batch.Providers[binding.Name] = adapter.ProviderConstructor(definition)
	return nil
}

func (c *ConstructorSet) identity(binding extension.Binding, definition *adapter.Definition) error {
	if _, exists := c.Batch.Identities[binding.Name]; exists {
		return errUtils.ErrDuplicateAuthRegistration
	}
	accepts, err := stringList(binding.Metadata["accepts"])
	if err != nil {
		return err
	}
	materializers := map[string]adapter.MaterializerFactory{"aws/v1": adapter.AWSMaterializer, "environment/v1": adapter.EnvironmentMaterializer}
	if len(definition.Outputs) != 1 {
		return errUtils.ErrPluginUnsupportedExtension
	}
	materialize := materializers[definition.Outputs[0]]
	if materialize == nil {
		return errUtils.ErrPluginUnsupportedExtension
	}
	c.Batch.Identities[binding.Name] = adapter.IdentityConstructor(definition, accepts, materialize)
	return nil
}

func (c *ConstructorSet) activator(capability string) func(context.Context, *extension.Activation) error {
	if extension.IsComponent(capability) {
		return func(ctx context.Context, activation *extension.Activation) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			c.Components = append(c.Components, activation)
			return nil
		}
	}
	adapters := map[string]func(extension.Binding, *adapter.Definition) error{"auth.provider": c.provider, "auth.identity": c.identity}
	if capability == "command" {
		return c.command
	}
	compile := adapters[capability]
	if compile == nil {
		return nil
	}
	return func(ctx context.Context, activation *extension.Activation) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		definition, err := c.authDefinition(activation)
		if err != nil {
			return err
		}
		return compile(activation.Binding, definition)
	}
}

func (c *ConstructorSet) command(ctx context.Context, activation *extension.Activation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key := activation.Package + ":" + activation.Name
	if _, exists := c.Commands[key]; exists {
		return errUtils.ErrDuplicatePluginExtension
	}
	c.Commands[key] = activation
	return nil
}

func (c *ConstructorSet) authDefinition(activation *extension.Activation) (*adapter.Definition, error) {
	binding := activation.Binding
	schemaPath, ok := binding.Metadata["schema"].(string)
	if !ok {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	schema, err := extension.CompileSchema(binding.Directory, schemaPath)
	if err != nil {
		return nil, err
	}
	outputs, err := stringList(binding.Metadata["credentials"])
	if err != nil {
		return nil, err
	}
	return &adapter.Definition{
		Expected: activation.Description, Package: activation.Package, Directory: binding.Directory, Executable: activation.Executable,
		Schema: schema.Validate, Outputs: outputs, Services: c.Services, Credentials: c.Credentials, Timeout: activation.Timeout,
	}, nil
}

func stringList(value any) ([]string, error) {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	result := make([]string, len(items))
	for index, item := range items {
		result[index], ok = item.(string)
		if !ok || result[index] == "" {
			return nil, errors.Join(errUtils.ErrPluginInvalidConfig, errUtils.ErrInvalidCredentialCodec)
		}
	}
	return result, nil
}
