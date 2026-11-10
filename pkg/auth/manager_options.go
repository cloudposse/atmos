package auth

import "github.com/cloudposse/atmos/pkg/auth/types"

// AuthExecAWSFilesBasePathEnv is an internal marker passed to child processes so
// nested Atmos helpers share their GitLab auth exec invocation's AWS file storage.
const AuthExecAWSFilesBasePathEnv = "_ATMOS_AUTH_EXEC_AWS_FILES_BASE_PATH"

// ManagerOption configures an authentication manager for a single invocation.
type ManagerOption func(*manager)

// WithAWSFilesBasePath scopes GitLab-backed AWS files to an invocation-owned directory.
// The caller owns cleanup. Explicit options override the scope inherited from the environment.
func WithAWSFilesBasePath(basePath string) ManagerOption {
	return func(m *manager) {
		m.awsFilesBasePath = basePath
	}
}

// awsFilesBasePathReceiver keeps runtime file configuration out of the general identity interface.
type awsFilesBasePathReceiver interface {
	SetAWSFilesBasePath(string)
}

func (m *manager) usesScopedAWSFiles(identityName string) bool {
	if m.awsFilesBasePath == "" {
		return false
	}
	provider := m.providers[m.resolveProviderForIdentity(identityName)]
	return provider != nil && provider.Kind() == types.ProviderKindGitLabOIDC
}

func applyManagerOptions(authManager types.AuthManager, options ...ManagerOption) types.AuthManager {
	// These options are only applied to managers constructed by NewAuthManager.
	m := authManager.(*manager)
	for _, option := range options {
		option(m)
	}
	m.configureScopedAWSFiles()
	return m
}

func (m *manager) configureScopedAWSFiles() {
	for name, identity := range m.identities {
		if receiver, ok := identity.(awsFilesBasePathReceiver); ok {
			basePath := ""
			if m.usesScopedAWSFiles(name) {
				basePath = m.awsFilesBasePath
			}
			receiver.SetAWSFilesBasePath(basePath)
		}
	}
}
