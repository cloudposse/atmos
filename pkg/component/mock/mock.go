package mock

import (
	"context"
	"fmt"
	"sort"

	"github.com/mitchellh/mapstructure"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// MockComponentProvider is a proof-of-concept component type for testing
// the component registry pattern. It demonstrates the interface implementation
// without requiring external tools or cloud providers.
//
// This component type is NOT documented for end users and is intended only
// for development and testing purposes.
type MockComponentProvider struct{}

func init() {
	defer perf.Track(nil, "mock.init")()

	// Self-register with the component registry.
	if err := component.Register(&MockComponentProvider{}); err != nil {
		panic(fmt.Sprintf("failed to register mock component provider: %v", err))
	}
}

// GetType returns the component type identifier.
func (m *MockComponentProvider) GetType() string {
	defer perf.Track(nil, "mock.GetType")()

	return "mock"
}

// GetGroup returns the component group for categorization.
func (m *MockComponentProvider) GetGroup() string {
	defer perf.Track(nil, "mock.GetGroup")()

	return "Testing"
}

// GetBasePath returns the base directory path for this component type.
func (m *MockComponentProvider) GetBasePath(atmosConfig *schema.AtmosConfiguration) string {
	defer perf.Track(atmosConfig, "mock.GetBasePath")()

	if atmosConfig == nil {
		return DefaultConfig().BasePath
	}

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
	defer perf.Track(nil, "mock.ListComponents")()

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

	sort.Strings(componentNames)
	return componentNames, nil
}

// ValidateComponent validates component configuration.
func (m *MockComponentProvider) ValidateComponent(config map[string]any) error {
	defer perf.Track(nil, "mock.ValidateComponent")()

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
func (m *MockComponentProvider) Execute(ctx *component.ExecutionContext) error {
	defer perf.Track(ctx.AtmosConfig, "mock.Execute")()

	// Mock execution - simulates command execution without external dependencies.
	u.PrintfMessageToTUI("Mock component execution:\n")
	u.PrintfMessageToTUI("  Type: %s\n", ctx.ComponentType)
	u.PrintfMessageToTUI("  Component: %s\n", ctx.Component)
	u.PrintfMessageToTUI("  Stack: %s\n", ctx.Stack)
	u.PrintfMessageToTUI("  Command: %s\n", ctx.Command)

	if ctx.SubCommand != "" {
		u.PrintfMessageToTUI("  SubCommand: %s\n", ctx.SubCommand)
	}

	return nil
}

// GenerateArtifacts creates necessary files for component execution.
func (m *MockComponentProvider) GenerateArtifacts(ctx *component.ExecutionContext) error {
	defer perf.Track(ctx.AtmosConfig, "mock.GenerateArtifacts")()

	// Mock artifact generation - no actual files created.
	u.PrintfMessageToTUI("Mock artifact generation for %s in stack %s\n", ctx.Component, ctx.Stack)
	return nil
}

// GetAvailableCommands returns list of commands this component type supports.
func (m *MockComponentProvider) GetAvailableCommands() []string {
	defer perf.Track(nil, "mock.GetAvailableCommands")()

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
