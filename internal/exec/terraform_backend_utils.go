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

// GetS3BackendBucket returns the `bucket` section from an S3 backend config.
func GetS3BackendBucket(backend map[string]any) string {
	if i, ok := backend["bucket"].(string); ok {
		return i
	}
	return ""
}

/*
   bucket: bd-kma-ue2-prod-tfstate
   dynamodb_table: bd-kma-ue2-prod-tfstate-lock
   role_arn: arn:aws:iam::145023098834:role/bd-kma-gbl-prod-tfstate
   encrypt: true
   key: terraform.tfstate
   acl: bucket-owner-full-control
   region: us-east-2
   workspace_key_prefix: "eks-karpenter-node-pool"
*/
