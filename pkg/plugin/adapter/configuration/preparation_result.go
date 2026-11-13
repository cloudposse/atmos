package configuration

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

func preparedConfiguration(result *pluginv1.PrepareComponentResponse) (*schema.PreparedComponentConfiguration, error) {
	inputs, err := adapter.DecodeInputs(result.Inputs)
	if err != nil {
		return nil, err
	}
	backend, err := adapter.DecodeObject(result.State.Backend)
	if err != nil {
		return nil, err
	}
	if result.State.BackendType == "local" {
		resolveLocalBackend(backend, result.State.WorkingDirectory)
	}
	prepared := &schema.PreparedComponentConfiguration{Engine: result.Engine, Source: result.ModuleSource, Inputs: inputs, BackendType: result.State.BackendType, Backend: backend, Workspace: result.State.Workspace, BackendGenerated: result.BackendGenerated}
	prepared.SensitiveInputKeys = sensitiveInputKeys(result.Inputs, inputs)
	if prepared.SourceAuthorization, err = preparedSourceAuthorization(result); err != nil {
		return nil, err
	}
	execution, err := preparedExecution(result.Execution)
	if err != nil {
		return nil, err
	}
	prepared.Execution = execution
	if prepared.IAM, err = preparedIAM(result.Iam); err != nil {
		return nil, err
	}
	if err := applyParity(prepared, result); err != nil {
		return nil, err
	}
	for _, file := range result.Files {
		prepared.Files = append(prepared.Files, schema.ComponentConfigurationFile{Path: file.Path, Content: file.Content, IfExists: file.IfExists, Sensitive: file.Sensitive, UnitFileCopy: file.UnitFileCopy})
	}
	for _, source := range result.Sources {
		prepared.Sources = append(prepared.Sources, schema.ComponentConfigurationSource{Path: source.Path, Directory: source.Kind == pluginv1.ComponentSource_DIRECTORY})
	}

	return prepared, nil
}

// sensitiveInputKeys preserves declared sensitivity independently of the decoded value.
// A sensitive empty string cannot be found later by value-based secret detection.
func sensitiveInputKeys(value *pluginv1.ComponentValue, inputs map[string]any) map[string]bool {
	if value == nil || len(value.SensitivePaths) == 0 {
		return nil
	}
	keys := make(map[string]bool)
	for _, path := range value.SensitivePaths {
		if path == nil || len(path.Steps) == 0 || path.Steps[0].GetAttribute() == "" {
			for key := range inputs {
				keys[key] = true
			}
			continue
		}
		keys[path.Steps[0].GetAttribute()] = true
	}
	return keys
}

func preparedSourceAuthorization(result *pluginv1.PrepareComponentResponse) (*schema.SourceAuthorization, error) {
	encoded := result.GetModuleSourceAuthorization()
	origin := result.GetModuleSourceAuthOrigin()
	if encoded == nil && origin == "" {
		return nil, nil
	}
	if !validSensitiveAuthorization(encoded, origin) {
		return nil, errUtils.ErrPluginProtocol
	}
	authorization, err := adapter.DecodeString(encoded)
	if err != nil || authorization == "" {
		return nil, errUtils.ErrPluginProtocol
	}
	if !sameSourceOrigin(result.GetModuleSource(), origin) {
		return nil, errUtils.ErrPluginProtocol
	}
	return &schema.SourceAuthorization{Origin: origin, Header: authorization}, nil
}

func validSensitiveAuthorization(encoded *pluginv1.ComponentValue, origin string) bool {
	return encoded != nil && origin != "" && len(encoded.GetSensitivePaths()) == 1 && len(encoded.GetSensitivePaths()[0].GetSteps()) == 0
}

func sameSourceOrigin(source, origin string) bool {
	archive, archiveErr := url.Parse(source)
	registry, registryErr := url.Parse(origin)
	if archiveErr != nil || registryErr != nil || !validRegistryOrigin(registry) {
		return false
	}
	return archive.Scheme == registry.Scheme && strings.EqualFold(archive.Host, registry.Host)
}

func validRegistryOrigin(registry *url.URL) bool {
	return registry.Host != "" && registry.User == nil && registry.Path == "" && registry.RawQuery == "" && registry.Fragment == "" && (registry.Scheme == "https" || registry.Scheme == "http")
}

func resolveLocalBackend(backend map[string]any, root string) {
	for key, fallback := range map[string]string{"path": "terraform.tfstate", "workspace_dir": "terraform.tfstate.d"} {
		path, _ := backend[key].(string)
		if path == "" {
			path = fallback
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		backend[key] = path
	}
}

// The sanitized base is authoritative, including an explicitly empty slice.
// Never reintroduce ambient credentials removed by the selected identity.
func preparationEnvironment(info *schema.ConfigAndStacksInfo) map[string]string {
	base := info.SanitizedEnv
	if base == nil {
		base = os.Environ()
	}
	env := make(map[string]string)
	for _, entries := range [][]string{base, info.ComponentEnvList} {
		for _, entry := range entries {
			key, value, ok := strings.Cut(entry, "=")
			if ok {
				env[key] = value
			}
		}
	}
	return env
}
