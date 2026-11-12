package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/signals"
)

// loginAuthManager scopes GitLab identity authentication without changing identity
// selection, provider authentication, or the storage used by other providers.
type loginAuthManager struct {
	auth.AuthManager
	config *schema.AtmosConfiguration
}

// Authenticate removes GitLab-derived AWS files before login returns, even when
// authentication fails partway through a role chain. It preserves shared profiles
// and any credential directory inherited from an enclosing auth exec command.
func (m *loginAuthManager) Authenticate(ctx context.Context, identityName string) (info *types.WhoamiInfo, resultErr error) {
	defer perf.Track(nil, "auth.login.Authenticate")()

	kind, err := m.GetProviderKindForIdentity(strings.ToLower(identityName))
	if err != nil || kind != types.ProviderKindGitLabOIDC {
		// Let the underlying manager handle unknown identities and profile fallback.
		return m.AuthManager.Authenticate(ctx, identityName)
	}

	ctx, releaseSignals := signals.WithShutdownContext(ctx)
	defer releaseSignals()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	basePath, err := filepath.Abs(os.TempDir())
	if err != nil {
		return nil, fmt.Errorf("failed to resolve GitLab OIDC login credential directory: %w", err)
	}
	directory, err := os.MkdirTemp(basePath, "atmos-auth-login-")
	if err != nil {
		return nil, fmt.Errorf("failed to create GitLab OIDC login credential directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(directory); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("failed to clean up GitLab OIDC login credential directory: %w", err))
		}
		if sig := signals.CancellationSignal(ctx); sig != nil {
			resultErr = errUtils.WithExitCode(resultErr, signals.ExitCodeForSignal(sig))
		}
	}()

	scoped, err := auth.NewDefaultManagerWithOptions(&m.config.Auth, m.config.CliConfigPath,
		auth.WithAWSFilesBasePath(directory))
	if err != nil {
		return nil, err
	}
	return scoped.Authenticate(ctx, identityName)
}
