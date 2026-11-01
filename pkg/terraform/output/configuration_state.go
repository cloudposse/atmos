package output

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ConfigurationStateRequest identifies the component whose plugin-owned state is resolved.
type ConfigurationStateRequest struct {
	// Sections are the component's described sections.
	Sections map[string]any
	// Component is the component name as the caller addresses it.
	Component string
	// Stack is the stack that contains the component.
	Stack string
	// AuthContext is the target's resolved authentication context.
	AuthContext *schema.AuthContext
	// AuthManagers optionally supply the sanitized base environment of the target's identity.
	AuthManagers []any
}

// ResolveConfigurationState locates plugin-owned state without evaluating inputs or preparing files.
// The caller supplies the target's resolved authentication context, just as for ordinary state reads.
func ResolveConfigurationState(ctx context.Context, config *schema.AtmosConfiguration, request *ConfigurationStateRequest) (*schema.ComponentConfigurationState, error) {
	defer perf.Track(config, "output.ResolveConfigurationState")()

	sections, component, stack := request.Sections, request.Component, request.Stack
	settings, _ := sections["settings"].(map[string]any)
	if _, owned := settings["configuration"]; !owned {
		return nil, nil
	}
	runtime, ok := config.ComponentConfigurations.(schema.ComponentConfigurationStateRuntime)
	if !ok {
		return nil, errUtils.ErrPluginUnsupportedExtension
	}
	env, _ := sections["env"].(map[string]any)
	environment, err := (&defaultEnvironmentSetup{}).SetupEnvironment(&ComponentConfig{Env: env, BaseEnvironment: configurationEnvironment(request.AuthManagers)}, request.AuthContext)
	if err != nil {
		return nil, err
	}
	info := &schema.ConfigAndStacksInfo{Stack: stack, ComponentFromArg: component, SubCommand: "output", ComponentSettingsSection: settings, SanitizedEnv: make([]string, 0, len(environment))}
	info.Command, _ = sections["command"].(string)
	info.TerraformWorkspace, _ = sections["workspace"].(string)
	for key, value := range environment {
		info.SanitizedEnv = append(info.SanitizedEnv, key+"="+value)
	}
	state, err := runtime.ResolveState(ctx, info)
	if err == nil && state == nil {
		return nil, errUtils.ErrPluginProtocol
	}
	return state, err
}

// ConfigurationStateSections adapts the upstream state location for existing backend readers.
// Local readers currently give an explicit path precedence over workspace, so select its exact file.
func ConfigurationStateSections(sections map[string]any, state *schema.ComponentConfigurationState) map[string]any {
	defer perf.Track(nil, "output.ConfigurationStateSections")()

	if state == nil {
		return sections
	}
	result := maps.Clone(sections)
	backend := maps.Clone(state.Backend)
	if state.BackendType == "local" && state.Workspace != "" && state.Workspace != "default" {
		root, _ := backend["workspace_dir"].(string)
		if root == "" {
			root = filepath.Join(state.WorkingDirectory, "terraform.tfstate.d")
		}
		backend["path"] = filepath.Join(root, state.Workspace, "terraform.tfstate")
	}
	result["backend_type"], result["backend"], result["workspace"] = state.BackendType, backend, state.Workspace
	// A state location does not require provisioning the source or execution workdir.
	delete(result, "source")
	delete(result, "provision")
	delete(result, "remote_state_backend")
	delete(result, "remote_state_backend_type")
	return result
}

// configurationOutputConfig uses a disposable backend-only directory. Output reads must not
// regenerate a unit, fetch its module, invoke hooks, or overwrite its existing execution directory.
func configurationOutputConfig(config *ComponentConfig, state *schema.ComponentConfigurationState) (func(), error) {
	root, err := os.MkdirTemp("", "atmos-plugin-output-")
	if err != nil {
		return nil, fmt.Errorf("%w: create state reader directory: %w", errUtils.ErrTerraformOutputFailed, err)
	}
	config.ComponentPath = root
	config.ReadOnlyState = true
	config.Env = maps.Clone(config.Env)
	if config.Env == nil {
		config.Env = make(map[string]any)
	}
	config.Env["TF_DATA_DIR"] = filepath.Join(root, ".terraform")
	config.BackendType, config.Backend, config.Workspace = state.BackendType, state.Backend, state.Workspace
	config.AutoGenerateBackend = true
	config.AutoProvisionWorkdirForOutputs = false
	config.Providers, config.Vars = nil, nil
	config.PassVars = false
	return func() { _ = os.RemoveAll(root) }, nil
}

// configurationEnvironment retains an explicitly empty sanitized environment.
func configurationEnvironment(managers []any) []string {
	for _, candidate := range managers {
		if manager, ok := candidate.(auth.AuthManager); ok && manager != nil {
			if info := manager.GetStackInfo(); info != nil && info.SanitizedEnv != nil {
				return slices.Clone(info.SanitizedEnv)
			}
		}
	}
	return nil
}
