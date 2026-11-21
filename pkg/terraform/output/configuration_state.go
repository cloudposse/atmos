package output

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	componentpkg "github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	localBackendType    = "local"
	backendStateFile    = "terraform.tfstate"
	backendWorkspaceDir = "workspace_dir"
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
	if !schema.ConfigurationOwned(settings) {
		return nil, nil
	}
	// Discovery does not provide the native unit's state identity. Reading the
	// placeholder backend could return a different component's local state.
	runtime, ok := config.ComponentConfigurations.(schema.ComponentConfigurationStateRuntime)
	if !ok {
		return nil, fmt.Errorf("%w: the configuration runtime does not support state lookup for component `%s` in stack `%s`", errUtils.ErrPluginUnsupportedExtension, component, stack)
	}
	env, _ := sections["env"].(map[string]any)
	environment, err := setupOutputEnvironment(&ComponentConfig{Env: env, BaseEnvironment: configurationEnvironment(request.AuthManagers)}, request.AuthContext, false)
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
	if state.BackendType == localBackendType && state.Workspace != "" && state.Workspace != "default" {
		// A local backend may declare no settings at all; maps.Clone keeps that nil.
		if backend == nil {
			backend = make(map[string]any)
		}
		root, _ := backend[backendWorkspaceDir].(string)
		if root == "" {
			root = filepath.Join(state.WorkingDirectory, "terraform.tfstate.d")
		}
		backend["path"] = filepath.Join(root, state.Workspace, backendStateFile)
	}
	result["backend_type"], result["backend"], result["workspace"] = state.BackendType, backend, state.Workspace
	// A state location does not require provisioning the source or execution workdir.
	delete(result, "source")
	delete(result, "provision")
	delete(result, "remote_state_backend")
	delete(result, "remote_state_backend_type")
	if remoteType, remote := explicitRemoteStateBackend(sections, state.BackendType, backend); remoteType != "" {
		result["remote_state_backend_type"], result["remote_state_backend"] = remoteType, remote
	}
	return result
}

// explicitRemoteStateBackend keeps the remote-state read settings the stack configured, applied to
// the plugin's backend. Atmos stores remote_state_backend as the component's backend with those
// settings layered on top. For a plugin-owned component that backend is a discovery placeholder,
// so only what differs from it was configured explicitly. A static remote state is kept as is.
// The plugin's backend stays the fallback for everything the stack did not override.
func explicitRemoteStateBackend(sections map[string]any, pluginType string, pluginBackend map[string]any) (string, map[string]any) {
	remoteType, _ := sections["remote_state_backend_type"].(string)
	remote, _ := sections["remote_state_backend"].(map[string]any)
	if remoteType == "static" {
		return remoteType, remote
	}
	placeholderType, _ := sections["backend_type"].(string)
	overrides := remoteStateOverrides(remote, remoteType, placeholderType, sections)
	explicitType := remoteType != "" && remoteType != placeholderType
	if !explicitType && len(overrides) == 0 {
		return "", nil
	}
	if !explicitType {
		remoteType = pluginType
	}
	if remoteType != pluginType {
		// A different backend type is a separate read location; it is not layered on the plugin's.
		return remoteType, overrides
	}
	merged := maps.Clone(pluginBackend)
	if merged == nil {
		merged = make(map[string]any, len(overrides))
	}
	maps.Copy(merged, overrides)
	return remoteType, merged
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
	config.BackendType, config.Backend, config.Workspace = state.BackendType, absoluteBackendFiles(state.BackendType, state.Backend, state.WorkingDirectory), state.Workspace
	if state.BackendType == localBackendType && state.WorkingDirectory != "" {
		config.Backend = maps.Clone(config.Backend)
		if config.Backend == nil {
			config.Backend = make(map[string]any)
		}
		if path, _ := config.Backend["path"].(string); path == "" {
			config.Backend["path"] = filepath.Join(state.WorkingDirectory, backendStateFile)
		}
		if dir, _ := config.Backend[backendWorkspaceDir].(string); dir == "" {
			config.Backend[backendWorkspaceDir] = filepath.Join(state.WorkingDirectory, "terraform.tfstate.d")
		}
	}
	config.AutoGenerateBackend = true
	config.AutoProvisionWorkdirForOutputs = false
	config.Providers, config.Vars = nil, nil
	config.PassVars = false
	return func() { _ = os.RemoveAll(root) }, nil
}

// configurationInitializedOutputConfig reads outputs from the unit's own working directory when
// the caller skips init. Nothing is generated or initialized there: the directory is used exactly
// as the unit's last execution left it, so a directory that was never initialized fails clearly.
func configurationInitializedOutputConfig(config *ComponentConfig, state *schema.ComponentConfigurationState, component, stack string) error {
	directory := config.ComponentPath
	if directory == "" {
		directory = state.WorkingDirectory
	}
	if directory == "" {
		return errUtils.Build(errUtils.ErrTerraformOutputFailed).
			WithExplanationf("Component '%s' in stack '%s' reported no working directory to read outputs from without init.", component, stack).
			WithHint("Allow initialization with init.mode: auto and without --skip-init so Atmos can initialize a temporary state reader").
			Err()
	}
	dataDir := configurationDataDir(config)
	if !filepath.IsAbs(dataDir) {
		dataDir = filepath.Join(directory, dataDir)
	}
	if info, err := os.Stat(dataDir); err != nil || !info.IsDir() {
		return errUtils.Build(errUtils.ErrTerraformOutputFailed).
			WithExplanationf("Component '%s' in stack '%s' has not been initialized in %s.", component, stack, directory).
			WithHint("Allow initialization with init.mode: auto and without --skip-init so Atmos can initialize a temporary state reader").
			Err()
	}
	if err := verifyInitializedBackend(dataDir, directory, state); err != nil {
		return err
	}
	if err := verifyInitializedWorkspace(dataDir, state.Workspace); err != nil {
		return err
	}
	config.ComponentPath = directory
	config.ReadOnlyState = true
	config.Workspace = state.Workspace
	config.AutoGenerateBackend = false
	config.AutoProvisionWorkdirForOutputs = false
	config.Providers, config.Vars = nil, nil
	config.PassVars = false
	return nil
}

// configurationDataDir resolves TF_DATA_DIR from the environment the output subprocess receives,
// with SetupEnvironment's precedence: the authoritative base environment (or the inherited one
// when there is none), then the component env section. Terraform's default applies otherwise.
func configurationDataDir(config *ComponentConfig) string {
	environment, err := (&defaultEnvironmentSetup{}).SetupEnvironment(&ComponentConfig{Env: config.Env, BaseEnvironment: config.BaseEnvironment}, nil)
	if err == nil && environment["TF_DATA_DIR"] != "" {
		return environment["TF_DATA_DIR"]
	}
	return ".terraform"
}

// remoteStateOverrides returns the remote-state settings that differ from the placeholder backend.
// A remote state of another type shares nothing with the placeholder, so all of it is explicit.
func remoteStateOverrides(remote map[string]any, remoteType, placeholderType string, sections map[string]any) map[string]any {
	placeholder, _ := sections["backend"].(map[string]any)
	overrides := make(map[string]any)
	for key, value := range remote {
		existing, ok := placeholder[key]
		if remoteType != placeholderType || !ok || !reflect.DeepEqual(existing, value) {
			overrides[key] = value
		}
	}
	return overrides
}

// configurationStateReader points config at the directory a plugin-owned state read runs in and
// returns its cleanup. Skipping init reads the unit's already-initialized directory as it is;
// otherwise a disposable backend-only scratch directory is initialized.
func configurationStateReader(config *ComponentConfig, state *schema.ComponentConfigurationState, skipInit bool, component, stack string) (func(), error) {
	if skipInit {
		return func() {}, configurationInitializedOutputConfig(config, state, component, stack)
	}
	return configurationOutputConfig(config, state)
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

// backendFileKeys name backend settings that hold file paths. Terraform resolves a relative
// path against its working directory, which for output reads is the scratch directory.
var backendFileKeys = []string{
	"shared_credentials_file", "shared_credentials_files", "shared_config_files",
	"client_certificate_path", "oidc_token_file_path",
}

// localBackendFileKeys hold file paths only for the local backend. Other backends use the
// same names for remote keys (Consul's "path" is a KV key), which must not be anchored.
var localBackendFileKeys = []string{"path", backendWorkspaceDir}

// absoluteBackendFiles anchors relative backend file paths at the state working directory,
// so a scratch reader finds the same files as the unit's own execution. The input is unchanged.
func absoluteBackendFiles(backendType string, backend map[string]any, directory string) map[string]any {
	if directory == "" || len(backend) == 0 {
		return backend
	}
	result := maps.Clone(backend)
	keys := backendFileKeys
	if backendType == localBackendType {
		keys = append(slices.Clone(localBackendFileKeys), backendFileKeys...)
	}
	for _, key := range keys {
		if value, ok := result[key]; ok {
			result[key] = anchorBackendFile(value, directory)
		}
	}
	return result
}

// anchorBackendFile anchors one path, or each path of a list, at directory. Absolute and
// home-relative paths, and values that are not paths, are returned unchanged.
func anchorBackendFile(value any, directory string) any {
	switch typed := value.(type) {
	case string:
		if typed != "" && !filepath.IsAbs(typed) && !strings.HasPrefix(typed, "~") {
			return filepath.Join(directory, typed)
		}
		return typed
	case []any:
		paths := make([]any, len(typed))
		for i := range typed {
			paths[i] = anchorBackendFile(typed[i], directory)
		}
		return paths
	case []string:
		paths := make([]any, len(typed))
		for i := range typed {
			paths[i] = anchorBackendFile(typed[i], directory)
		}
		return paths
	}
	return value
}

// retainedConfigurationOutputPath locates the existing Atmos workdir without provisioning.
// The provider state directory remains untouched for relative backend path resolution.
func retainedConfigurationOutputPath(atmosConfig *schema.AtmosConfiguration, sections map[string]any, state *schema.ComponentConfigurationState, name, stack string) (string, error) {
	info := &schema.ConfigAndStacksInfo{Stack: stack, ComponentFromArg: name, Component: name, FinalComponent: name, ComponentSection: maps.Clone(sections)}
	if info.ComponentSection["atmos_stack"] == nil {
		info.ComponentSection["atmos_stack"] = stack
	}
	if info.ComponentSection["atmos_component"] == nil {
		info.ComponentSection["atmos_component"] = name
	}
	if base, ok := sections["component"].(string); ok && base != "" {
		info.FinalComponent = base
	}
	return componentpkg.ResolveRetainedWorkdir(atmosConfig, info, "terraform", state.WorkingDirectory)
}

// verifyInitializedWorkspace never selects or creates a workspace during a skip-init read.
func verifyInitializedWorkspace(dataDir, expected string) error {
	selected := "default"
	data, err := os.ReadFile(filepath.Join(dataDir, "environment"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: cannot verify selected workspace: %w", errUtils.ErrTerraformOutputFailed, err)
	}
	if err == nil {
		selected = strings.TrimSpace(string(data))
	}
	if expected == "" {
		expected = "default"
	}
	if selected != expected {
		return fmt.Errorf("%w: initialized workspace %q differs from resolved workspace %q; select the resolved workspace or allow initialization", errUtils.ErrTerraformOutputFailed, selected, expected)
	}
	return nil
}

// verifyInitializedBackend compares persisted init metadata, never regenerated backend files.
func verifyInitializedBackend(dataDir, directory string, state *schema.ComponentConfigurationState) error {
	data, err := os.ReadFile(filepath.Join(dataDir, backendStateFile))
	if err != nil {
		return fmt.Errorf("%w: cannot verify initialized backend; allow initialization: %w", errUtils.ErrTerraformOutputFailed, err)
	}
	var initialized struct {
		Backend struct {
			Type   string         `json:"type"`
			Config map[string]any `json:"config"`
		} `json:"backend"`
	}
	if err := json.Unmarshal(data, &initialized); err != nil {
		return fmt.Errorf("%w: invalid initialized backend metadata: %w", errUtils.ErrTerraformOutputFailed, err)
	}
	expected := normalizedBackendIdentity(state.BackendType, state.Backend, state.WorkingDirectory)
	actual := normalizedBackendIdentity(initialized.Backend.Type, initialized.Backend.Config, directory)
	discardUnsetBackendDefaults(actual, expected)
	expectedJSON, expectedErr := json.Marshal(expected)
	actualJSON, actualErr := json.Marshal(actual)
	if expectedErr != nil || actualErr != nil || initialized.Backend.Type != state.BackendType || string(expectedJSON) != string(actualJSON) {
		return fmt.Errorf("%w: initialized backend identity differs from the resolved backend; allow initialization before reading outputs", errUtils.ErrTerraformOutputFailed)
	}
	return nil
}

func normalizedBackendIdentity(kind string, backend map[string]any, directory string) map[string]any {
	result := maps.Clone(backend)
	if result == nil {
		result = map[string]any{}
	}
	if kind == localBackendType {
		for key, fallback := range map[string]string{"path": backendStateFile, backendWorkspaceDir: "terraform.tfstate.d"} {
			if value, _ := result[key].(string); value == "" {
				result[key] = fallback
			}
		}
	}
	return absoluteBackendFiles(kind, result, directory)
}

// discardUnsetBackendDefaults ignores only schema defaults absent from the resolved settings.
func discardUnsetBackendDefaults(actual, expected map[string]any) {
	maps.DeleteFunc(actual, func(key string, value any) bool {
		_, exists := expected[key]
		return !exists && (value == nil || value == "" || value == false || value == float64(0))
	})
}

type outputConfiguration struct {
	config          *ComponentConfig
	owned, skipInit bool
	cleanup         func()
}

// prepareOutputConfiguration selects a state reader without provisioning plugin workdirs.
func prepareOutputConfiguration(ctx context.Context, atmosConfig *schema.AtmosConfiguration, request *ConfigurationStateRequest, skipInit bool) (*outputConfiguration, error) {
	state, err := ResolveConfigurationState(ctx, atmosConfig, request)
	if err != nil {
		return nil, err
	}
	result := &outputConfiguration{owned: state != nil, skipInit: skipInit, cleanup: func() {}}
	if state == nil {
		result.config, err = ExtractComponentConfig(atmosConfig, request.Sections, request.Component, request.Stack)
		return result, err
	}
	result.config, err = extractConfigurationStateConfig(atmosConfig, request.Sections, request.Component, request.Stack)
	if err != nil {
		return nil, err
	}
	result.skipInit = skipInit || result.config.InitMode == schema.TerraformInitModeNever
	result.config.BaseEnvironment = configurationEnvironment(request.AuthManagers)
	if result.skipInit {
		result.config.ComponentPath, err = retainedConfigurationOutputPath(atmosConfig, request.Sections, state, request.Component, request.Stack)
		if err != nil {
			return nil, fmt.Errorf("%w: locate retained workdir: %w", errUtils.ErrTerraformOutputFailed, err)
		}
	}
	result.cleanup, err = configurationStateReader(result.config, state, result.skipInit, request.Component, request.Stack)
	return result, err
}
