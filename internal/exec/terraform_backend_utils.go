package exec

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
)

// GetComponentTerraformWorkspace returns the `workspace` section for a component in a stack.
func GetComponentTerraformWorkspace(sections map[string]any) string {
	if workspace, ok := sections[cfg.WorkspaceSectionName].(string); ok {
		return workspace
	}
	return ""
}

// GetComponentRemoteStateBackendStaticType returns the `remote_state_backend` section for a component in a stack.
// if the `remote_state_backend_type` is `static`
func GetComponentRemoteStateBackendStaticType(sections map[string]any) map[string]any {
	var remoteStateBackend map[string]any
	var remoteStateBackendType string
	var ok bool

	if remoteStateBackendType, ok = sections[cfg.RemoteStateBackendTypeSectionName].(string); !ok {
		return nil
	}
	if remoteStateBackendType != cfg.StaticSectionName {
		return nil
	}
	if remoteStateBackend, ok = sections[cfg.RemoteStateBackendSectionName].(map[string]any); ok {
		return remoteStateBackend
	}
	return nil
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

// GetBackendAssumeRole returns the `assume_role` section from the backend config.
// https://developer.hashicorp.com/terraform/language/backend/s3#assume-role-configuration
func GetBackendAssumeRole(backend map[string]any) map[string]any {
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
	Type      string
	Workspace string
	Backend   map[string]any
	S3        TerraformS3BackendInfo
}

// GetTerraformBackendInfo returns the Terraform backend information from the component config.
func GetTerraformBackendInfo(sections map[string]any) TerraformBackendInfo {
	info := TerraformBackendInfo{}
	info.Workspace = GetComponentTerraformWorkspace(sections)
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
		assumeRoleSection := GetBackendAssumeRole(info.Backend)
		if assumeRoleSection != nil {
			roleArn = GetSectionAttribute(assumeRoleSection, "role_arn")
		}
		if roleArn == "" {
			roleArn = GetSectionAttribute(info.Backend, "role_arn")
		}
		info.S3.RoleArn = roleArn
	}

	return info
}
