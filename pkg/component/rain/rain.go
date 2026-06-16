package rain

import (
	"context"
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ComponentProvider implements ComponentProvider for Rain CloudFormation components.
type ComponentProvider struct{}

func init() {
	defer perf.Track(nil, "rain.init")()

	if err := component.Register(&ComponentProvider{}); err != nil {
		panic(fmt.Sprintf("failed to register rain component provider: %v", err))
	}
}

func (p *ComponentProvider) GetType() string {
	defer perf.Track(nil, "rain.ComponentProvider.GetType")()

	return cfg.RainComponentType
}

func (p *ComponentProvider) GetGroup() string {
	defer perf.Track(nil, "rain.ComponentProvider.GetGroup")()

	return "Infrastructure as Code"
}

func (p *ComponentProvider) GetBasePath(atmosConfig *schema.AtmosConfiguration) string {
	defer perf.Track(atmosConfig, "rain.ComponentProvider.GetBasePath")()

	if atmosConfig == nil || atmosConfig.Components.Rain.BasePath == "" {
		return DefaultConfig().BasePath
	}
	return atmosConfig.Components.Rain.BasePath
}

func (p *ComponentProvider) ListComponents(ctx context.Context, stack string, stackConfig map[string]any) ([]string, error) {
	defer perf.Track(nil, "rain.ListComponents")()

	componentsSection, ok := stackConfig[cfg.ComponentsSectionName].(map[string]any)
	if !ok {
		return []string{}, nil
	}

	rainComponents, ok := componentsSection[cfg.RainComponentType].(map[string]any)
	if !ok {
		return []string{}, nil
	}

	names := make([]string, 0, len(rainComponents))
	for name := range rainComponents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (p *ComponentProvider) ValidateComponent(config map[string]any) error {
	defer perf.Track(nil, "rain.ValidateComponent")()

	if config == nil {
		return nil
	}

	return validateComponentFields(config)
}

func (p *ComponentProvider) Execute(ctx *component.ExecutionContext) error {
	defer perf.Track(ctx.AtmosConfig, "rain.Execute")()

	return Execute(ctx)
}

func (p *ComponentProvider) GenerateArtifacts(ctx *component.ExecutionContext) error {
	defer perf.Track(ctx.AtmosConfig, "rain.GenerateArtifacts")()
	return nil
}

func (p *ComponentProvider) GetAvailableCommands() []string {
	defer perf.Track(nil, "rain.ComponentProvider.GetAvailableCommands")()

	return []string{
		"bootstrap", "build", "cat", "cc", "console", "deploy", "diff", "fmt",
		"forecast", "info", "logs", "ls", "merge", "module", "pkg", "rm",
		"stackset", "tree", "watch",
	}
}

func validateComponentFields(config map[string]any) error {
	if err := validateStringField(config, "template"); err != nil {
		return err
	}
	if err := validateStringField(config, cfg.NameSectionName); err != nil {
		return err
	}
	if err := validateMapField(config, "params"); err != nil {
		return err
	}
	return validateMapField(config, "tags")
}

func validateStringField(config map[string]any, key string) error {
	value, ok := config[key]
	if !ok || value == nil {
		return nil
	}
	if _, isString := value.(string); !isString {
		return fmt.Errorf("%w: %s must be a string", errUtils.ErrComponentValidationFailed, key)
	}
	return nil
}

func validateMapField(config map[string]any, key string) error {
	value, ok := config[key]
	if !ok || value == nil {
		return nil
	}
	if _, isMap := value.(map[string]any); !isMap {
		return fmt.Errorf("%w: %s must be a map", errUtils.ErrComponentValidationFailed, key)
	}
	return nil
}
