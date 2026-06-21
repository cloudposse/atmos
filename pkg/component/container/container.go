package container

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

// ContainerComponentProvider implements component.ComponentProvider for the
// container component kind: a stack-scoped, Atmos-native, persistent container
// whose lifecycle is discovered by labels derived from the canonical component
// instance address.
type ContainerComponentProvider struct{}

func init() {
	if err := component.Register(&ContainerComponentProvider{}); err != nil {
		panic(fmt.Sprintf("failed to register container component provider: %v", err))
	}
}

// GetType returns the component type identifier.
func (p *ContainerComponentProvider) GetType() string {
	defer perf.Track(nil, "container.ContainerComponentProvider.GetType")()

	return cfg.ContainerComponentType
}

// GetGroup returns the component group for categorization.
func (p *ContainerComponentProvider) GetGroup() string {
	defer perf.Track(nil, "container.ContainerComponentProvider.GetGroup")()

	return "Containers"
}

// GetBasePath returns the base directory path for container components.
func (p *ContainerComponentProvider) GetBasePath(atmosConfig *schema.AtmosConfiguration) string {
	defer perf.Track(atmosConfig, "container.GetBasePath")()

	if atmosConfig == nil {
		return DefaultConfig().BasePath
	}
	if raw, ok := atmosConfig.Components.GetComponentConfig(cfg.ContainerComponentType); ok {
		if config, err := parseConfig(raw); err == nil && config.BasePath != "" {
			return config.BasePath
		}
	}
	return DefaultConfig().BasePath
}

// ListComponents discovers all container components in a stack.
func (p *ContainerComponentProvider) ListComponents(_ context.Context, _ string, stackConfig map[string]any) ([]string, error) {
	defer perf.Track(nil, "container.ListComponents")()

	componentsSection, ok := stackConfig["components"].(map[string]any)
	if !ok {
		return []string{}, nil
	}
	containerComponents, ok := componentsSection[cfg.ContainerComponentType].(map[string]any)
	if !ok {
		return []string{}, nil
	}
	names := make([]string, 0, len(containerComponents))
	for name := range containerComponents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// ValidateComponent validates container component configuration. A real
// (non-abstract) component must declare an image or a build.
func (p *ContainerComponentProvider) ValidateComponent(config map[string]any) error {
	defer perf.Track(nil, "container.ValidateComponent")()

	if config == nil {
		return nil
	}
	if metadata, ok := config["metadata"].(map[string]any); ok {
		if componentType, ok := metadata["type"].(string); ok && componentType == "abstract" {
			return nil
		}
	}

	vars, _ := config["vars"].(map[string]any)
	if vars == nil {
		return fmt.Errorf("%w: container component requires vars.image or vars.build", errUtils.ErrComponentValidationFailed)
	}
	_, hasImage := vars["image"]
	_, hasBuild := vars["build"]
	if !hasImage && !hasBuild {
		return fmt.Errorf("%w: container component requires vars.image or vars.build", errUtils.ErrComponentValidationFailed)
	}
	return nil
}

// verbExecutor runs a single container subcommand.
type verbExecutor func(ctx context.Context, info *schema.ConfigAndStacksInfo, args []string) error

// verbExecutors maps each subcommand to its executor. Most verbs ignore args;
// only `exec` forwards the pass-through command.
var verbExecutors = map[string]verbExecutor{
	"list": func(ctx context.Context, info *schema.ConfigAndStacksInfo, _ []string) error {
		return ExecuteList(ctx, info)
	},
	"build": func(ctx context.Context, info *schema.ConfigAndStacksInfo, _ []string) error {
		return ExecuteBuild(ctx, info)
	},
	"push": func(ctx context.Context, info *schema.ConfigAndStacksInfo, _ []string) error {
		return ExecutePush(ctx, info)
	},
	"pull": func(ctx context.Context, info *schema.ConfigAndStacksInfo, _ []string) error {
		return ExecutePull(ctx, info)
	},
	"run": func(ctx context.Context, info *schema.ConfigAndStacksInfo, _ []string) error {
		return ExecuteRun(ctx, info)
	},
	"up": func(ctx context.Context, info *schema.ConfigAndStacksInfo, _ []string) error {
		return ExecuteUp(ctx, info)
	},
	"ps": func(ctx context.Context, info *schema.ConfigAndStacksInfo, _ []string) error {
		return ExecutePs(ctx, info)
	},
	"logs": func(ctx context.Context, info *schema.ConfigAndStacksInfo, _ []string) error {
		return ExecuteLogs(ctx, info)
	},
	"exec": ExecuteExec,
	"restart": func(ctx context.Context, info *schema.ConfigAndStacksInfo, _ []string) error {
		return ExecuteRestart(ctx, info)
	},
	"stop": func(ctx context.Context, info *schema.ConfigAndStacksInfo, _ []string) error {
		return ExecuteStop(ctx, info)
	},
	"rm": func(ctx context.Context, info *schema.ConfigAndStacksInfo, _ []string) error {
		return ExecuteRm(ctx, info)
	},
	"down": func(ctx context.Context, info *schema.ConfigAndStacksInfo, _ []string) error {
		return ExecuteDown(ctx, info)
	},
}

// Execute dispatches a container subcommand to the matching executor.
func (p *ContainerComponentProvider) Execute(execCtx *component.ExecutionContext) error {
	defer perf.Track(execCtx.AtmosConfig, "container.Execute")()

	exec, ok := verbExecutors[execCtx.SubCommand]
	if !ok {
		return fmt.Errorf("%w: unknown container subcommand %q", errUtils.ErrComponentExecutionFailed, execCtx.SubCommand)
	}
	return exec(context.Background(), &execCtx.ConfigAndStacksInfo, execCtx.Args)
}

// GenerateArtifacts is a no-op for container components (no generated files).
func (p *ContainerComponentProvider) GenerateArtifacts(_ *component.ExecutionContext) error {
	defer perf.Track(nil, "container.ContainerComponentProvider.GenerateArtifacts")()

	return nil
}

// GetAvailableCommands returns the lifecycle verbs supported by container components.
func (p *ContainerComponentProvider) GetAvailableCommands() []string {
	defer perf.Track(nil, "container.ContainerComponentProvider.GetAvailableCommands")()

	return []string{
		"build", "push", "pull", "run", "up", "ps",
		"logs", "exec", "restart", "stop", "rm", "down",
	}
}
