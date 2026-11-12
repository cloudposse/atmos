package auth

import (
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

// prepareGitLabAuthExec isolates credential files before authentication can write them.
// It preserves the configured realm and process cache without changing global environment.
func prepareGitLabAuthExec(execCtx *authExecContext, config *schema.AtmosConfiguration) error {
	defer perf.Track(nil, "auth.exec.prepareGitLabAuthExec")()

	// Configured identity keys are normalized by the manager; flags may use mixed case.
	identityName := execCtx.identity
	identities := execCtx.authManager.GetIdentities()
	if _, exists := identities[identityName]; !exists {
		identityName = strings.ToLower(identityName)
		if _, exists := identities[identityName]; !exists {
			// Authenticate offers profile fallback for identities outside the loaded config.
			return nil
		}
	}
	kind, err := execCtx.authManager.GetProviderKindForIdentity(identityName)
	if err != nil {
		return err
	}
	if kind != types.ProviderKindGitLabOIDC {
		return nil
	}
	execCtx.identity = identityName
	execCtx.ctx, execCtx.releaseSignals = signals.WithShutdownContext(execCtx.ctx)
	if err := execCtx.ctx.Err(); err != nil {
		return err
	}

	basePath, err := filepath.Abs(os.TempDir())
	if err != nil {
		return fmt.Errorf("failed to resolve GitLab OIDC credential directory: %w", err)
	}
	execCtx.credentialDir, err = os.MkdirTemp(basePath, "atmos-auth-exec-")
	if err != nil {
		return fmt.Errorf("failed to create GitLab OIDC credential directory: %w", err)
	}
	execCtx.authManager, err = auth.NewDefaultManagerWithOptions(&config.Auth, config.CliConfigPath,
		auth.WithAWSFilesBasePath(execCtx.credentialDir))
	return err
}

// cleanupGitLabAuthExec removes only files owned by this invocation. Logging out
// the identity would also remove shared profiles and integration sessions (such as ECR).
func cleanupGitLabAuthExec(execCtx *authExecContext) error {
	defer perf.Track(nil, "auth.exec.cleanupGitLabAuthExec")()

	if execCtx == nil {
		return nil
	}
	if execCtx.releaseSignals != nil {
		defer execCtx.releaseSignals()
	}
	if execCtx.credentialDir == "" {
		return nil
	}
	if err := os.RemoveAll(execCtx.credentialDir); err != nil {
		return fmt.Errorf("failed to clean up GitLab OIDC credential directory: %w", err)
	}
	return nil
}

// cleanupFailedGitLabAuthExec handles credentials written before setup returns an error.
func cleanupFailedGitLabAuthExec(execCtx *authExecContext, resultErr *error) {
	if *resultErr != nil {
		if sig := signals.CancellationSignal(execCtx.ctx); sig != nil {
			*resultErr = errUtils.WithExitCode(*resultErr, signals.ExitCodeForSignal(sig))
		}
		if cleanupErr := cleanupGitLabAuthExec(execCtx); cleanupErr != nil {
			*resultErr = errors.Join(*resultErr, cleanupErr)
		}
	}
}
