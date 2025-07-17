package terraform_backend

import (
	"encoding/json"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// GetComponentTerraformWorkspace returns the `workspace` section for a component in a stack.
func GetComponentTerraformWorkspace(sections map[string]any) string {
	if workspace, ok := sections[cfg.WorkspaceSectionName].(string); ok {
		return workspace
	}
	return ""
}

// GetTerraformComponent returns the `component` section for a component in a stack.
func GetTerraformComponent(sections map[string]any) string {
	if workspace, ok := sections[cfg.ComponentSectionName].(string); ok {
		return workspace
	}
	return ""
}

// GetComponentBackend returns the `backend` section for a component in a stack.
func GetComponentBackend(sections map[string]any) map[string]any {
	if remoteStateBackend, ok := sections[cfg.BackendSectionName].(map[string]any); ok {
		return remoteStateBackend
	}
	return nil
}

// GetComponentBackendType returns the `backend_type` section for a component in a stack.
func GetComponentBackendType(sections map[string]any) string {
	if backendType, ok := sections[cfg.BackendTypeSectionName].(string); ok {
		return backendType
	}
	return ""
}

// GetS3BackendAssumeRole returns the `assume_role` section from the S3 backend config.
// https://developer.hashicorp.com/terraform/language/backend/s3#assume-role-configuration
func GetS3BackendAssumeRole(backend map[string]any) map[string]any {
	if i, ok := backend["assume_role"].(map[string]any); ok {
		return i
	}
	return nil
}

// GetSectionAttribute returns an attribute from a section in the config.
func GetSectionAttribute(section map[string]any, attribute string) string {
	if i, ok := section[attribute].(string); ok {
		return i
	}
	return ""
}

// TerraformS3BackendInfo contains the `s3` backend information.
type TerraformS3BackendInfo struct {
	Bucket             string
	Region             string
	Key                string
	RoleArn            string
	WorkspaceKeyPrefix string
}

// TerraformBackendInfo contains the backend information.
type TerraformBackendInfo struct {
	Type               string
	Workspace          string
	TerraformComponent string
	Backend            map[string]any
	S3                 TerraformS3BackendInfo
}

// GetTerraformBackendInfo returns the Terraform backend information from the component config.
func GetTerraformBackendInfo(sections map[string]any) TerraformBackendInfo {
	info := TerraformBackendInfo{}
	info.Workspace = GetComponentTerraformWorkspace(sections)
	info.TerraformComponent = GetTerraformComponent(sections)
	info.Backend = GetComponentBackend(sections)
	info.Type = GetComponentBackendType(sections)

	// If the backend is not configured in stack manifests, default to "local"
	if info.Type == "" {
		info.Type = cfg.BackendTypeLocal
	}

	// Process S3 backend
	if info.Type == cfg.BackendTypeS3 {
		info.S3 = TerraformS3BackendInfo{
			Bucket:             GetSectionAttribute(info.Backend, "bucket"),
			Region:             GetSectionAttribute(info.Backend, "region"),
			Key:                GetSectionAttribute(info.Backend, "key"),
			WorkspaceKeyPrefix: GetSectionAttribute(info.Backend, "workspace_key_prefix"),
		}
		// Support `assume_role.role_arn` and the deprecated `role_arn` in the backend
		var roleArn string
		assumeRoleSection := GetS3BackendAssumeRole(info.Backend)
		if assumeRoleSection != nil {
			roleArn = GetSectionAttribute(assumeRoleSection, "role_arn")
		}
		// If `assume_role.role_arn` is not set, fallback to `role_arn`
		if roleArn == "" {
			roleArn = GetSectionAttribute(info.Backend, "role_arn")
		}
		info.S3.RoleArn = roleArn
	}

	return info
}

// GetTerraformBackendVariable returns the output from the configured backend.
func GetTerraformBackendVariable(
	atmosConfig *schema.AtmosConfiguration,
	values map[string]any,
	variable string,
) (any, error) {
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
	if len(data) == 0 {
		return nil, nil
	}

	var rawState RawTerraformState
	if err := json.Unmarshal(data, &rawState); err != nil {
		return nil, err
	}

	rawOutputs := rawState.Outputs
	result := make(map[string]any, len(rawOutputs))

	for key, output := range rawOutputs {
		result[key] = output.Value
	}

	return result, nil
}

// GetTerraformBackend reads and processes the Terraform state file from the configured backend.
func GetTerraformBackend(
	atmosConfig *schema.AtmosConfiguration,
	componentSections map[string]any,
) (map[string]any, error) {
	backendInfo := GetTerraformBackendInfo(componentSections)
	var content []byte
	var err error

	switch backendInfo.Type {
	case cfg.BackendTypeLocal:
		content, err = ReadTerraformBackendLocal(atmosConfig, &backendInfo)
		if err != nil {
			return nil, err
		}
	case cfg.BackendTypeS3:
		content, err = ReadTerraformBackendS3(&backendInfo)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w %s. Supported backends: local, s3", errUtils.ErrUnsupportedBackendType, backendInfo.Type)
	}

	data, err := ProcessTerraformStateFile(content)
	if err != nil {
		return nil, fmt.Errorf("%w.\nerror: %v", errUtils.ErrProcessTerraformStateFile, err)
	}

	return data, nil
}
