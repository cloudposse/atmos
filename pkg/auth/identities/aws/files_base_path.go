package aws

import (
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// filesBasePath returns spec.files.base_path of the root provider of
// identityName, or "" (the default XDG location) when it is not configured or
// the manager is not available yet.
//
// Credentials are namespaced by the root provider, and the provider reports
// and cleans up its files under this base path (Paths, Logout), so identities
// must read and write them under the same base path.
func filesBasePath(manager types.AuthManager, identityName string) string {
	if manager == nil {
		return ""
	}
	provider, ok := manager.ResolveProviderConfig(identityName)
	if !ok {
		return ""
	}
	return awsCloud.GetFilesBasePath(provider)
}
