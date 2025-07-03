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

// GetBackendAttribute returns an attribute from the backend config.
func GetBackendAttribute(backend map[string]any, section string) string {
	if i, ok := backend[section].(string); ok {
		return i
	}
	return ""
}

type TerraformS3BackendInfo struct {
	Bucket             string
	Region             string
	Key                string
	RoleArn            string
	WorkspaceKeyPrefix string
}

type TerraformBackendInfo struct {
	Type      string
	Workspace string
	Backend   map[string]any
	S3        TerraformS3BackendInfo
}

func GetTerraformBackendInfo(sections map[string]any) TerraformBackendInfo {
	info := TerraformBackendInfo{}
	info.Workspace = GetComponentTerraformWorkspace(sections)
	info.Backend = GetComponentBackend(sections)
	info.Type = GetComponentBackendType(sections)
	if info.Type == "" {
		info.Type = cfg.BackendTypeLocal
	}

	if info.Type == cfg.BackendTypeS3 {
		info.S3 = TerraformS3BackendInfo{
			Bucket:             GetBackendAttribute(info.Backend, "bucket"),
			Region:             GetBackendAttribute(info.Backend, "region"),
			Key:                GetBackendAttribute(info.Backend, "key"),
			RoleArn:            GetBackendAttribute(info.Backend, "role_arn"),
			WorkspaceKeyPrefix: GetBackendAttribute(info.Backend, "workspace_key_prefix"),
		}
	}

	return info
}
