// Package gitlab provides GitLab CI/CD OIDC authentication.
package gitlab

import (
	"context"
	"fmt"
	"maps"
	"os"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const defaultTokenEnvironmentVariable = "ATMOS_OIDC_TOKEN"

// oidcProvider reads a job ID token issued through GitLab's id_tokens configuration.
// Downstream identities exchange the token for cloud credentials.
type oidcProvider struct {
	name   string
	config *schema.Provider
}

var (
	_ types.Provider        = (*oidcProvider)(nil)
	_ types.AmbientProvider = (*oidcProvider)(nil)
)

// NewOIDCProvider creates a GitLab OIDC provider without reading its runtime token.
func NewOIDCProvider(name string, config *schema.Provider) (types.Provider, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: provider config is required", errUtils.ErrInvalidProviderConfig)
	}
	if name == "" {
		return nil, fmt.Errorf("%w: provider name is required", errUtils.ErrInvalidProviderConfig)
	}
	return &oidcProvider{name: name, config: config}, nil
}

// Name returns the configured provider name.
func (p *oidcProvider) Name() string {
	return p.name
}

// Kind returns the provider kind.
func (p *oidcProvider) Kind() string {
	return "gitlab/oidc"
}

// SetRealm satisfies types.Provider. This provider owns no realm-scoped storage.
func (p *oidcProvider) SetRealm(_ string) {}

// PreAuthenticate requires no preparation for an already-issued job token.
func (p *oidcProvider) PreAuthenticate(_ types.AuthManager) error {
	return nil
}

// IsAmbient prevents the manager from persisting the token or credentials derived
// from it in the keyring. The process-level cache still applies to the final credentials.
func (p *oidcProvider) IsAmbient() bool {
	return true
}

// Authenticate reads the current job token for use by downstream cloud identities.
func (p *oidcProvider) Authenticate(_ context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "gitlab.oidcProvider.Authenticate")()

	envVar, err := p.tokenEnvironmentVariable()
	if err != nil {
		return nil, err
	}
	// The source is a runtime credential, not an Atmos configuration setting. Read
	// it directly on every call without binding secrets into global Viper state.
	token, _ := os.LookupEnv(envVar)
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errUtils.Build(errUtils.ErrAuthenticationFailed).
			WithExplanationf("GitLab OIDC token environment variable %q is missing or empty", envVar).
			WithHintf("Configure id_tokens.%s.aud in .gitlab-ci.yml to match the AWS IAM OIDC provider audience", envVar).
			Err()
	}
	return &types.OIDCCredentials{Token: token, Provider: "gitlab"}, nil
}

// Validate checks configuration without requiring a running GitLab job.
func (p *oidcProvider) Validate() error {
	_, err := p.tokenEnvironmentVariable()
	return err
}

func (p *oidcProvider) tokenEnvironmentVariable() (string, error) {
	value, exists := p.config.Spec["token_environment_variable"]
	if !exists {
		return defaultTokenEnvironmentVariable, nil
	}
	envVar, ok := value.(string)
	if !ok || strings.TrimSpace(envVar) == "" {
		return "", fmt.Errorf("%w: spec.token_environment_variable must be a non-empty string", errUtils.ErrInvalidProviderConfig)
	}
	if strings.ContainsAny(envVar, "=\x00") {
		return "", fmt.Errorf("%w: spec.token_environment_variable must not contain '=' or NUL", errUtils.ErrInvalidProviderConfig)
	}
	return strings.TrimSpace(envVar), nil
}

// Environment returns no additional variables; the token travels through credentials.
func (p *oidcProvider) Environment() (map[string]string, error) {
	return map[string]string{}, nil
}

// Paths returns no paths because the provider reads its token from the environment.
func (p *oidcProvider) Paths() ([]types.Path, error) {
	return []types.Path{}, nil
}

// PrepareEnvironment preserves the environment for the downstream cloud identity.
func (p *oidcProvider) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	return maps.Clone(environ), nil
}

// Logout reports that a GitLab-issued job token has no provider-owned storage to remove.
func (p *oidcProvider) Logout(_ context.Context) error {
	return errUtils.ErrLogoutNotSupported
}

// GetFilesDisplayPath returns no path because the provider owns no credential files.
func (p *oidcProvider) GetFilesDisplayPath() string {
	return ""
}
