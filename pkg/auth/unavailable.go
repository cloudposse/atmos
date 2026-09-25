package auth

import (
	"errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// NormalizeAuthenticationError classifies an authentication attempt without hiding
// configuration or identity-selection errors. Callers decide whether to fail or degrade.
func NormalizeAuthenticationError(err error) error {
	defer perf.Track(nil, "auth.NormalizeAuthenticationError")()

	if err == nil || errors.Is(err, errUtils.ErrAuthenticationUnavailable) {
		return err
	}
	for _, fatal := range []error{
		errUtils.ErrInvalidAuthConfig, errUtils.ErrIdentityNotFound, errUtils.ErrProviderNotFound,
		errUtils.ErrInvalidIdentityKind, errUtils.ErrInvalidIdentityConfig, errUtils.ErrCircularDependency,
		errUtils.ErrUserAborted,
	} {
		if errors.Is(err, fatal) {
			return err
		}
	}
	for _, unavailable := range []error{
		errUtils.ErrAuthenticationFailed, errUtils.ErrIdentityAuthFailed, errUtils.ErrNoCredentialsFound,
		errUtils.ErrExpiredCredentials, errUtils.ErrCredentialsInvalid, errUtils.ErrIdentityCredentialsNone,
	} {
		if errors.Is(err, unavailable) {
			return errUtils.JoinPreservingHints(errUtils.ErrAuthenticationUnavailable, err)
		}
	}
	return err
}
