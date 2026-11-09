package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// cleanupGitLabAuthExec removes the GitLab-backed chain's credential and config
// profiles after command execution or failed setup. Other profiles are preserved.
func cleanupGitLabAuthExec(execCtx *authExecContext) error {
	defer perf.Track(nil, "auth.exec.cleanupGitLabAuthExec")()

	if execCtx == nil || execCtx.authManager == nil || execCtx.identity == "" {
		return nil
	}
	chain := execCtx.authManager.GetChain()
	if len(chain) < 2 {
		return nil
	}
	providers := execCtx.authManager.GetProviders()
	if providers[chain[0]].Kind != types.ProviderKindGitLabOIDC {
		return nil
	}

	// Work backwards through all roles, even when a previous cleanup fails.
	// A fresh context allows cleanup to finish after command cancellation.
	var errs []error
	for i := len(chain) - 1; i > 0; i-- {
		if err := execCtx.authManager.Logout(context.Background(), chain[i], false); err != nil {
			errs = append(errs, fmt.Errorf("failed to clean up GitLab OIDC identity %q: %w", chain[i], err))
		}
	}
	return errors.Join(errs...)
}
