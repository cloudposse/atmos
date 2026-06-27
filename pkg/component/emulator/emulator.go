package emulator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	_ "github.com/cloudposse/atmos/pkg/emulator/driver" // register the built-in emulator drivers.
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// EmulatorComponentProvider implements component.ComponentProvider for the
// emulator component kind: a stack-scoped, long-running cloud-API emulator whose
// lifecycle is discovered by labels derived from the canonical instance address.
type EmulatorComponentProvider struct{}

func init() {
	if err := component.Register(&EmulatorComponentProvider{}); err != nil {
		panic(fmt.Sprintf("failed to register emulator component provider: %v", err))
	}
}

// GetType returns the component type identifier.
func (p *EmulatorComponentProvider) GetType() string {
	defer perf.Track(nil, "emulator.EmulatorComponentProvider.GetType")()

	return cfg.EmulatorComponentType
}

// GetGroup returns the component group for categorization.
func (p *EmulatorComponentProvider) GetGroup() string {
	defer perf.Track(nil, "emulator.EmulatorComponentProvider.GetGroup")()

	return "Emulators"
}

// GetBasePath returns the base directory path for emulator components.
func (p *EmulatorComponentProvider) GetBasePath(atmosConfig *schema.AtmosConfiguration) string {
	defer perf.Track(atmosConfig, "emulator.GetBasePath")()

	if atmosConfig == nil {
		return DefaultConfig().BasePath
	}
	if raw, ok := atmosConfig.Components.GetComponentConfig(cfg.EmulatorComponentType); ok {
		if config, err := parseConfig(raw); err == nil && config.BasePath != "" {
			return config.BasePath
		}
	}
	return DefaultConfig().BasePath
}

// ListComponents discovers all emulator components in a stack.
func (p *EmulatorComponentProvider) ListComponents(_ context.Context, _ string, stackConfig map[string]any) ([]string, error) {
	defer perf.Track(nil, "emulator.ListComponents")()

	componentsSection, ok := stackConfig["components"].(map[string]any)
	if !ok {
		return []string{}, nil
	}
	emulatorComponents, ok := componentsSection[cfg.EmulatorComponentType].(map[string]any)
	if !ok {
		return []string{}, nil
	}
	names := make([]string, 0, len(emulatorComponents))
	for name := range emulatorComponents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// ValidateComponent validates emulator component configuration. A real
// (non-abstract) component must declare a driver.
func (p *EmulatorComponentProvider) ValidateComponent(config map[string]any) error {
	defer perf.Track(nil, "emulator.ValidateComponent")()

	if config == nil {
		return nil
	}
	if isAbstractSection(config) {
		return nil
	}
	if driver, ok := config["driver"].(string); !ok || strings.TrimSpace(driver) == "" {
		return fmt.Errorf("%w: emulator component requires a driver", errUtils.ErrComponentValidationFailed)
	}
	return nil
}

// verbExecutor runs a single emulator subcommand. It receives the full execution
// context so command-specific flags (e.g. `--ephemeral`, `--force`) carried in
// ExecutionContext.Flags reach the executor.
type verbExecutor func(ctx context.Context, execCtx *component.ExecutionContext) error

// verbExecutors maps each subcommand to its executor. Only `exec` forwards the
// pass-through command; `up` and `reset` read their flags from execCtx.Flags.
var verbExecutors = map[string]verbExecutor{
	"up": func(ctx context.Context, ec *component.ExecutionContext) error {
		return executeUp(ctx, &ec.ConfigAndStacksInfo, flagBool(ec.Flags, "ephemeral"))
	},
	"down": func(ctx context.Context, ec *component.ExecutionContext) error {
		return ExecuteDown(ctx, &ec.ConfigAndStacksInfo)
	},
	"reset": func(ctx context.Context, ec *component.ExecutionContext) error {
		return ExecuteReset(ctx, &ec.ConfigAndStacksInfo, flagBool(ec.Flags, "force"))
	},
	"ps": func(ctx context.Context, ec *component.ExecutionContext) error {
		return ExecutePs(ctx, &ec.ConfigAndStacksInfo)
	},
	"logs": func(ctx context.Context, ec *component.ExecutionContext) error {
		return ExecuteLogs(ctx, &ec.ConfigAndStacksInfo)
	},
	"exec": func(ctx context.Context, ec *component.ExecutionContext) error {
		return ExecuteExec(ctx, &ec.ConfigAndStacksInfo, ec.Args)
	},
}

// Execute dispatches an emulator subcommand to the matching executor.
func (p *EmulatorComponentProvider) Execute(execCtx *component.ExecutionContext) error {
	defer perf.Track(execCtx.AtmosConfig, "emulator.Execute")()

	exec, ok := verbExecutors[execCtx.SubCommand]
	if !ok {
		return fmt.Errorf("%w: unknown emulator subcommand %q", errUtils.ErrComponentExecutionFailed, execCtx.SubCommand)
	}
	return exec(context.Background(), execCtx)
}

// GenerateArtifacts is a no-op for emulator components (no generated files).
func (p *EmulatorComponentProvider) GenerateArtifacts(_ *component.ExecutionContext) error {
	defer perf.Track(nil, "emulator.EmulatorComponentProvider.GenerateArtifacts")()

	return nil
}

// GetAvailableCommands returns the lifecycle verbs supported by emulator components.
func (p *EmulatorComponentProvider) GetAvailableCommands() []string {
	defer perf.Track(nil, "emulator.EmulatorComponentProvider.GetAvailableCommands")()

	return []string{"up", "down", "reset", "ps", "logs", "exec"}
}

// flagBool reads a boolean flag from the execution context's Flags map, returning
// false when the key is absent or not a bool.
func flagBool(flags map[string]any, key string) bool {
	if flags == nil {
		return false
	}
	v, _ := flags[key].(bool)
	return v
}
