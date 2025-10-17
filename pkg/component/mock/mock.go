package mock

import (
	"context"
	"fmt"

	"github.com/mitchellh/mapstructure"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

// MockComponentProvider is a proof-of-concept component type for testing
// the component registry pattern. It demonstrates the interface implementation
// without requiring external tools or cloud providers.
//
// This component type is NOT documented for end users and is intended only
// for development and testing purposes.
type MockComponentProvider struct{}

func init() {
	// Self-register with the component registry.
	if err := component.Register(&MockComponentProvider{}); err != nil {
		panic(fmt.Sprintf("failed to register mock component provider: %v", err))
	}
}

// GetType returns the component type identifier.
func (m *MockComponentProvider) GetType() string {
	return "mock"
}

// GetGroup returns the component group for categorization.
func (m *MockComponentProvider) GetGroup() string {
	return "Testing"
}

// GetBasePath returns the base directory path for this component type.
func (m *MockComponentProvider) GetBasePath(atmosConfig *schema.AtmosConfiguration) string {
	// Try to get config from Plugins map.
	rawConfig, ok := atmosConfig.Components.GetComponentConfig("mock")
	if !ok {
		return DefaultConfig().BasePath
	}

	// Parse raw config into typed struct.
	config, err := parseConfig(rawConfig)
	if err != nil {
		return DefaultConfig().BasePath
	}

	if config.BasePath != "" {
		return config.BasePath
	}

	return DefaultConfig().BasePath
}

// ListComponents discovers all components of this type in a stack.
func (m *MockComponentProvider) ListComponents(ctx context.Context, stack string, stackConfig map[string]any) ([]string, error) {
	componentsSection, ok := stackConfig["components"].(map[string]any)
	if !ok {
		return []string{}, nil
	}

	mockComponents, ok := componentsSection["mock"].(map[string]any)
	if !ok {
		return []string{}, nil
	}

	componentNames := make([]string, 0, len(mockComponents))
	for name := range mockComponents {
		componentNames = append(componentNames, name)
	}

	return componentNames, nil
}

// ValidateComponent validates component configuration.
func (m *MockComponentProvider) ValidateComponent(config map[string]any) error {
	if config == nil {
		return nil
	}

	// Mock validation - fails if explicitly marked invalid.
	if invalid, ok := config["invalid"].(bool); ok && invalid {
		return fmt.Errorf("%w: invalid flag set", errUtils.ErrComponentValidationFailed)
	}

	return nil
}

// Execute runs a command for this component type.
func (m *MockComponentProvider) Execute(ctx component.ExecutionContext) error {
	// Mock execution - simulates command execution without external dependencies.
	fmt.Printf("Mock component execution:\n")
	fmt.Printf("  Type: %s\n", ctx.ComponentType)
	fmt.Printf("  Component: %s\n", ctx.Component)
	fmt.Printf("  Stack: %s\n", ctx.Stack)
	fmt.Printf("  Command: %s\n", ctx.Command)

	if ctx.SubCommand != "" {
		fmt.Printf("  SubCommand: %s\n", ctx.SubCommand)
	}

	return nil
}

// GenerateArtifacts creates necessary files for component execution.
func (m *MockComponentProvider) GenerateArtifacts(ctx component.ExecutionContext) error {
	// Mock artifact generation - no actual files created.
	fmt.Printf("Mock artifact generation for %s in stack %s\n", ctx.Component, ctx.Stack)
	return nil
}

// GetAvailableCommands returns list of commands this component type supports.
func (m *MockComponentProvider) GetAvailableCommands() []string {
	return []string{"plan", "apply", "destroy", "validate"}
}

// parseConfig parses raw configuration map into typed Config struct.
func parseConfig(raw any) (Config, error) {
	var config Config

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &config,
		TagName: "mapstructure",
	})
	if err != nil {
		return DefaultConfig(), err
	}

	if err := decoder.Decode(raw); err != nil {
		return DefaultConfig(), err
	}

	return config, nil
}
