package terraform

import (
	"errors"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// configurationGapReason reports why a provider API write was skipped when the cause is a
// configuration gap rather than a runtime failure: no token, or no usable API URL. Callers
// log these at Debug level so a run without API reporting configured stays quiet on every
// provider; the provider itself logs one line when the gap disables its API features.
func configurationGapReason(err error) (string, bool) {
	switch {
	case errors.Is(err, errUtils.ErrGitHubTokenNotFound):
		return "GITHUB_TOKEN not set", true
	case errors.Is(err, errUtils.ErrGiteaTokenNotFound):
		return "ATMOS_CI_GITEA_TOKEN or GITEA_TOKEN not set", true
	case errors.Is(err, errUtils.ErrGiteaAPIUnavailable):
		return "Gitea API URL unavailable", true
	default:
		return "", false
	}
}

// logOperationStatusError logs a skipped per-operation status, quietly for a configuration gap.
func logOperationStatusError(name string, err error) {
	const msg = "CI per-operation status creation skipped"
	if reason, ok := configurationGapReason(err); ok {
		log.Debug(msg, logKeyName, name, "reason", reason)
		return
	}
	log.Warn(msg, logKeyName, name, "error", err)
}
