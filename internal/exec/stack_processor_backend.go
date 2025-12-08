package exec

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// AzurermBackendName is the Azure RM backend type identifier.
	azurermBackendName = "azurerm"
	// BackendKeyName is the key field name in backend configuration.
	backendKeyName = "key"
)

// terraformBackendConfig holds configuration for processing Terraform backend.
type terraformBackendConfig struct {
	atmosConfig                 *schema.AtmosConfiguration
	component                   string
	baseComponentName           string
	componentMetadata           map[string]any
	globalBackendType           string
	globalBackendSection        map[string]any
	baseComponentBackendType    string
	baseComponentBackendSection map[string]any
	componentBackendType        string
	componentBackendSection     map[string]any
}

// processTerraformBackend processes Terraform backend configuration including S3, GCS, and Azure backends.
func processTerraformBackend(cfg *terraformBackendConfig) (string, map[string]any, error) {
	defer perf.Track(cfg.atmosConfig, "exec.processTerraformBackend")()

	// Determine final backend type.
	finalComponentBackendType := cfg.globalBackendType
	if len(cfg.baseComponentBackendType) > 0 {
		finalComponentBackendType = cfg.baseComponentBackendType
	}
	if len(cfg.componentBackendType) > 0 {
		finalComponentBackendType = cfg.componentBackendType
	}

	// Merge backend sections.
	finalComponentBackendSection, err := m.Merge(
		cfg.atmosConfig,
		[]map[string]any{
			cfg.globalBackendSection,
			cfg.baseComponentBackendSection,
			cfg.componentBackendSection,
		})
	if err != nil {
		return "", nil, err
	}

	// Extract backend configuration for the specific backend type.
	finalComponentBackend := map[string]any{}
	if i, ok := finalComponentBackendSection[finalComponentBackendType]; ok {
		finalComponentBackend, ok = i.(map[string]any)
		if !ok {
			return "", nil, fmt.Errorf("%w: for the component '%s'", errUtils.ErrInvalidTerraformBackend, cfg.component)
		}
	}

	// Set backend-specific defaults.
	switch finalComponentBackendType {
	case "s3":
		setS3BackendDefaults(finalComponentBackend, cfg.component, cfg.baseComponentName, cfg.componentMetadata)
	case "gcs":
		setGCSBackendDefaults(finalComponentBackend, cfg.component, cfg.baseComponentName, cfg.componentMetadata)
	case azurermBackendName:
		err := setAzureBackendKey(finalComponentBackend, cfg.component, cfg.baseComponentName, cfg.componentMetadata, cfg.componentBackendSection, cfg.globalBackendSection)
		if err != nil {
			return "", nil, err
		}
	}

	return finalComponentBackendType, finalComponentBackend, nil
}

// setS3BackendDefaults sets AWS S3 backend defaults.
// Priority for workspace_key_prefix: explicit config > metadata.name > metadata.component > Atmos component name.
func setS3BackendDefaults(backend map[string]any, component string, baseComponentName string, metadata map[string]any) {
	if p, ok := backend["workspace_key_prefix"].(string); !ok || p == "" {
		workspaceKeyPrefix := component
		// Priority: metadata.name > metadata.component (baseComponentName) > Atmos component name.
		if metadataName, ok := metadata["name"].(string); ok && metadataName != "" {
			workspaceKeyPrefix = metadataName
		} else if baseComponentName != "" {
			workspaceKeyPrefix = baseComponentName
		}
		backend["workspace_key_prefix"] = strings.ReplaceAll(workspaceKeyPrefix, "/", "-")
	}
}

// setGCSBackendDefaults sets Google GCS backend defaults.
// Priority for prefix: explicit config > metadata.name > metadata.component > Atmos component name.
func setGCSBackendDefaults(backend map[string]any, component string, baseComponentName string, metadata map[string]any) {
	if p, ok := backend["prefix"].(string); !ok || p == "" {
		prefix := component
		// Priority: metadata.name > metadata.component (baseComponentName) > Atmos component name.
		if metadataName, ok := metadata["name"].(string); ok && metadataName != "" {
			prefix = metadataName
		} else if baseComponentName != "" {
			prefix = baseComponentName
		}
		backend["prefix"] = strings.ReplaceAll(prefix, "/", "-")
	}
}

// setAzureBackendKey sets the Azure backend key if not present.
// Priority for key: explicit config > metadata.name > metadata.component > Atmos component name.
func setAzureBackendKey(
	finalComponentBackend map[string]any,
	component string,
	baseComponentName string,
	metadata map[string]any,
	componentBackendSection map[string]any,
	globalBackendSection map[string]any,
) error {
	defer perf.Track(nil, "exec.setAzureBackendKey")()

	componentAzurerm, componentAzurermExists := componentBackendSection[azurermBackendName].(map[string]any)
	if !componentAzurermExists {
		componentAzurerm = map[string]any{}
	}

	// Check if key already exists in component-specific azurerm section.
	if _, componentAzurermKeyExists := componentAzurerm[backendKeyName].(string); componentAzurermKeyExists {
		return nil
	}

	// Check if we should preserve an authored key from inheritance.
	if shouldPreserveAuthoredKey(finalComponentBackend, globalBackendSection) {
		return nil
	}

	// Determine the component name for the key.
	// Priority: metadata.name > metadata.component (baseComponentName) > Atmos component name.
	azureKeyPrefixComponent := component
	if metadataName, ok := metadata["name"].(string); ok && metadataName != "" {
		azureKeyPrefixComponent = metadataName
	} else if baseComponentName != "" {
		azureKeyPrefixComponent = baseComponentName
	}

	// Build the key path.
	var keyName []string
	if globalAzurerm, globalAzurermExists := globalBackendSection[azurermBackendName].(map[string]any); globalAzurermExists {
		if globalKey, globalAzurermKeyExists := globalAzurerm[backendKeyName].(string); globalAzurermKeyExists {
			keyName = append(keyName, globalKey)
		}
	}

	componentKeyName := strings.ReplaceAll(azureKeyPrefixComponent, "/", "-")
	keyName = append(keyName, fmt.Sprintf("%s.terraform.tfstate", componentKeyName))
	finalComponentBackend[backendKeyName] = strings.Join(keyName, "/")

	return nil
}

// shouldPreserveAuthoredKey checks if an authored key from base/component inheritance should be preserved.
func shouldPreserveAuthoredKey(finalComponentBackend map[string]any, globalBackendSection map[string]any) bool {
	defer perf.Track(nil, "exec.shouldPreserveAuthoredKey")()

	existingKey, ok := finalComponentBackend[backendKeyName].(string)
	if !ok || existingKey == "" {
		return false
	}

	globalAzurerm, ok := globalBackendSection[azurermBackendName].(map[string]any)
	if !ok {
		// No global azurerm section - preserve the authored key.
		return true
	}

	globalKey, ok := globalAzurerm[backendKeyName].(string)
	if !ok {
		// No global key - preserve the authored key.
		return true
	}

	// If existing key matches global key, treat global as prefix and continue.
	// Otherwise, preserve the authored key.
	return globalKey != existingKey
}

// remoteStateBackendConfig holds configuration for processing remote state backend.
type remoteStateBackendConfig struct {
	atmosConfig                            *schema.AtmosConfiguration
	component                              string
	finalComponentBackendType              string
	finalComponentBackendSection           map[string]any
	globalRemoteStateBackendType           string
	globalRemoteStateBackendSection        map[string]any
	baseComponentRemoteStateBackendType    string
	baseComponentRemoteStateBackendSection map[string]any
	componentRemoteStateBackendType        string
	componentRemoteStateBackendSection     map[string]any
}

// processTerraformRemoteStateBackend processes Terraform remote state backend configuration.
func processTerraformRemoteStateBackend(cfg *remoteStateBackendConfig) (string, map[string]any, error) {
	defer perf.Track(cfg.atmosConfig, "exec.processTerraformRemoteStateBackend")()

	// Determine final remote state backend type.
	finalComponentRemoteStateBackendType := cfg.finalComponentBackendType
	if len(cfg.globalRemoteStateBackendType) > 0 {
		finalComponentRemoteStateBackendType = cfg.globalRemoteStateBackendType
	}
	if len(cfg.baseComponentRemoteStateBackendType) > 0 {
		finalComponentRemoteStateBackendType = cfg.baseComponentRemoteStateBackendType
	}
	if len(cfg.componentRemoteStateBackendType) > 0 {
		finalComponentRemoteStateBackendType = cfg.componentRemoteStateBackendType
	}

	// Merge remote state backend sections.
	finalComponentRemoteStateBackendSection, err := m.Merge(
		cfg.atmosConfig,
		[]map[string]any{
			cfg.globalRemoteStateBackendSection,
			cfg.baseComponentRemoteStateBackendSection,
			cfg.componentRemoteStateBackendSection,
		})
	if err != nil {
		return "", nil, err
	}

	// Merge backend and remote_state_backend sections for DRY configuration.
	finalComponentRemoteStateBackendSectionMerged, err := m.Merge(
		cfg.atmosConfig,
		[]map[string]any{
			cfg.finalComponentBackendSection,
			finalComponentRemoteStateBackendSection,
		})
	if err != nil {
		return "", nil, err
	}

	// Extract remote state backend configuration for the specific backend type.
	finalComponentRemoteStateBackend := map[string]any{}
	if i, ok := finalComponentRemoteStateBackendSectionMerged[finalComponentRemoteStateBackendType]; ok {
		finalComponentRemoteStateBackend, ok = i.(map[string]any)
		if !ok {
			return "", nil, fmt.Errorf("%w: for the component '%s'", errUtils.ErrInvalidTerraformRemoteStateBackend, cfg.component)
		}
	}

	return finalComponentRemoteStateBackendType, finalComponentRemoteStateBackend, nil
}
