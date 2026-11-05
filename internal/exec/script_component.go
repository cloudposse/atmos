package exec

import (
	"context"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/custom"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ScriptComponentInfoResolver uses the execution pipeline rather than describe's
// inspection-only secret masking. Callers serialize access to config/auth caches.
func ScriptComponentInfoResolver(atmosConfig *schema.AtmosConfiguration, authManager auth.AuthManager) step.ComponentInfoResolver {
	defer perf.Track(atmosConfig, "exec.ScriptComponentInfoResolver")()

	return func(ctx context.Context, name, stack, componentType string) (*schema.ConfigAndStacksInfo, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info := schema.ConfigAndStacksInfo{ComponentFromArg: name, StackFromArg: stack, Stack: stack, ComponentType: componentType}
		config, err := cfg.InitCliConfig(info, true)
		if err != nil {
			return nil, err
		}
		if err := ensureScriptComponentProvider(&config, componentType); err != nil {
			return nil, err
		}
		var manager auth.AuthManager
		if config.CliConfigPath == atmosConfig.CliConfigPath {
			manager = authManager
		}
		resolved, err := ProcessStacks(&config, info, true, true, true, nil, manager)
		if err != nil {
			return nil, err
		}
		return &resolved, nil
	}
}

func ensureScriptComponentProvider(config *schema.AtmosConfiguration, componentType string) error {
	if _, ok := component.GetProvider(componentType); ok {
		return nil
	}
	base := filepath.Join("components", componentType)
	if plugin, ok := config.Components.Plugins[componentType].(map[string]any); ok {
		if path, ok := plugin["base_path"].(string); ok && path != "" {
			base = path
		}
	}
	return custom.EnsureRegistered(componentType, base)
}
