package cmd

import (
	"context"
	"errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
)

// isNoAuthConfigError reports whether err indicates a terminal state where
// the loaded configuration has no usable identities or providers — a state
// that switching to a different profile may recover from.
func isNoAuthConfigError(err error) bool {
	return errors.Is(err, errUtils.ErrNoProvidersAvailable) ||
		errors.Is(err, errUtils.ErrNoIdentitiesAvailable) ||
		errors.Is(err, errUtils.ErrNoDefaultIdentity)
}

// maybeOfferProfileFallbackOnAuthConfigError offers a profile-switch
// suggestion when err signals a "no auth config in base" terminal state.
// Called by auth commands (login, exec, shell, env, console, whoami) before
// returning their terminal error.
//
// For unrelated errors it returns err unchanged. On successful interactive
// re-exec it never returns. Otherwise it returns either the original err (no
// candidates, loop guard active, or explicit --profile/ATMOS_PROFILE set) or
// an enriched error naming the candidate profile(s).
func maybeOfferProfileFallbackOnAuthConfigError(ctx context.Context, authManager auth.AuthManager, err error) error {
	if err == nil || !isNoAuthConfigError(err) {
		return err
	}
	if fbErr := authManager.MaybeOfferAnyProfileFallback(ctx); fbErr != nil {
		return fbErr
	}
	return err
}
