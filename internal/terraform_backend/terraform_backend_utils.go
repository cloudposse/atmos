package terraform_backend

import (
	"encoding/json"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// GetTerraformWorkspace returns the `workspace` section for a component in a stack.
func GetTerraformWorkspace(sections *map[string]any) string {
	defer perf.Track(nil, "terraform_backend.GetTerraformWorkspace")()

	if workspace, ok := (*sections)[cfg.WorkspaceSectionName].(string); ok {
		return workspace
	}
	return ""
}

// GetTerraformComponent returns the `component` section for a component in a stack.
func GetTerraformComponent(sections *map[string]any) string {
	defer perf.Track(nil, "terraform_backend.GetTerraformComponent")()

	if workspace, ok := (*sections)[cfg.ComponentSectionName].(string); ok {
		return workspace
	}
	return ""
}

// GetComponentBackend returns the `backend` section for a component in a stack.
func GetComponentBackend(sections *map[string]any) map[string]any {
	defer perf.Track(nil, "terraform_backend.GetComponentBackend")()

	if remoteStateBackend, ok := (*sections)[cfg.BackendSectionName].(map[string]any); ok {
		return remoteStateBackend
	}
	return nil
}

// GetComponentBackendType returns the `backend_type` section for a component in a stack.
func GetComponentBackendType(sections *map[string]any) string {
	defer perf.Track(nil, "terraform_backend.GetComponentBackendType")()

	if backendType, ok := (*sections)[cfg.BackendTypeSectionName].(string); ok {
		return backendType
	}
	return ""
}

// GetBackendAttribute returns an attribute from a section in the backend.
func GetBackendAttribute(section *map[string]any, attribute string) string {
	defer perf.Track(nil, "terraform_backend.GetBackendAttribute")()

	if i, ok := (*section)[attribute].(string); ok {
		return i
	}
	return ""
}

// GetTerraformBackendVariable returns the output from the configured backend.
func GetTerraformBackendVariable(
	atmosConfig *schema.AtmosConfiguration,
	values map[string]any,
	variable string,
) (any, error) {
	defer perf.Track(atmosConfig, "terraform_backend.GetTerraformBackendVariable")()

	val := variable
	if !strings.HasPrefix(variable, ".") {
		val = "." + val
	}

	res, err := u.EvaluateYqExpression(atmosConfig, values, val)
	if err != nil {
		return nil, err
	}

	return res, nil
}

// RawTerraformState represents a raw Terraform state file.
type RawTerraformState struct {
	Version          int    `json:"version"`           // Internal format version
	TerraformVersion string `json:"terraform_version"` // CLI version used
	Outputs          map[string]struct {
		Value any `json:"value"` // Can be any JSON type
		Type  any `json:"type"`  // HCL type representation
	} `json:"outputs"`
	Resources interface{} `json:"resources,omitempty"`
}

// ProcessTerraformStateFile processes a Terraform state file.
func ProcessTerraformStateFile(data []byte) (map[string]any, error) {
	defer perf.Track(nil, "terraform_backend.ProcessTerraformStateFile")()

	if len(data) == 0 {
		return nil, nil
	}

	var rawState RawTerraformState
	if err := json.Unmarshal(data, &rawState); err != nil {
		return nil, err
	}

	rawOutputs := rawState.Outputs
	result := make(map[string]any, len(rawOutputs))

	// Process each output value through JSON round-trip to match terraform.output behavior.
	// This ensures consistent type handling (e.g., numbers, maps, arrays) between
	// terraform.output (which gets json.RawMessage from tfexec) and terraform.state
	// (which unmarshals from state file).
	for key, output := range rawOutputs {
		// Marshal the value back to JSON
		valueJSON, err := json.Marshal(output.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal output %s: %w", key, err)
		}

		// Unmarshal it back to ensure consistent type representation
		var convertedValue any
		if err := json.Unmarshal(valueJSON, &convertedValue); err != nil {
			return nil, fmt.Errorf("failed to unmarshal output %s: %w", key, err)
		}

		result[key] = convertedValue
	}

	return result, nil
}

// GetTerraformBackend reads and processes the Terraform state file from the configured backend.
func GetTerraformBackend(
	atmosConfig *schema.AtmosConfiguration,
	componentSections *map[string]any,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "terraform_backend.GetTerraformBackend")()

	RegisterTerraformBackends()

	backendType := GetComponentBackendType(componentSections)
	if backendType == "" {
		backendType = cfg.BackendTypeLocal
	}

	readBackendStateFunc := GetTerraformBackendReadFunc(backendType)
	if readBackendStateFunc == nil {
		return nil, fmt.Errorf("%w: `%s`\nsupported backends: `local`, `s3`, `azurerm`", errUtils.ErrUnsupportedBackendType, backendType)
	}

	content, err := readBackendStateFunc(atmosConfig, componentSections)
	if err != nil {
		return nil, err
	}

	data, err := ProcessTerraformStateFile(content)
	if err != nil {
		return nil, fmt.Errorf("%w\n%v", errUtils.ErrProcessTerraformStateFile, err)
	}

	return data, nil
}
