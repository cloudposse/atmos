package artifacts

import (
	"errors"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

// hintRule attaches hints to one family of artifact failures.
type hintRule struct {
	// sentinel is the error the rule matches.
	sentinel error

	// mention, when set, narrows the match to errors whose text contains it.
	mention string

	// hints returns the actionable hints, given the configured repository aliases.
	hints func(configured []string) []string
}

// hintRules are checked in order; the first match wins, so narrower rules come first.
var hintRules = []hintRule{
	{errUtils.ErrArtifactConfigInvalid, "artifacts.verification", func([]string) []string {
		return []string{"Set `artifacts.verification.trusted_identities` (or `trusted_keys`) in `atmos.yaml` for the signers you trust."}
	}},
	{errUtils.ErrArtifactConfigInvalid, "", func(configured []string) []string {
		return []string{
			"Check `artifacts.repositories` in `atmos.yaml` and pass `--repository=<alias>` with a configured alias.",
			configuredHint(configured),
		}
	}},
	{errUtils.ErrArtifactRefInvalid, "", func([]string) []string {
		return []string{"Use `name:version` or `name@sha256:<digest>`. There is no implicit `latest`."}
	}},
	{errUtils.ErrArtifactNotFound, "", func([]string) []string {
		return []string{"Run `atmos artifacts list --repository=<alias>` to see the published artifacts and versions."}
	}},
	{errUtils.ErrArtifactConflict, "", func([]string) []string {
		return []string{"Re-run with `--force` to replace the existing reference. Force never bypasses verification or immutable release formats."}
	}},
	{errUtils.ErrArtifactVerificationFailed, "", func([]string) []string {
		return []string{"Configure `artifacts.verification` with the signer identities or keys you trust, then run `atmos artifacts verify`."}
	}},
	{errUtils.ErrArtifactUnsupportedCapability, "", func([]string) []string {
		return []string{"This repository kind does not provide the requested capability. See the repository configuration reference for what each backend supports."}
	}},
}

// decorate adds a precise, actionable hint to the artifact failures users hit most, keeping the
// original error chain intact for errors.Is. Errors no rule matches pass through unchanged.
func decorate(err error, configured []string) error {
	if err == nil {
		return nil
	}
	for _, rule := range hintRules {
		if !errors.Is(err, rule.sentinel) || !strings.Contains(err.Error(), rule.mention) {
			continue
		}
		builder := errUtils.Build(err)
		for _, hint := range rule.hints(configured) {
			builder = builder.WithHint(hint)
		}
		return builder.Err()
	}
	return err
}

// configuredHint lists the configured aliases, or says that none are configured.
func configuredHint(configured []string) string {
	if len(configured) == 0 {
		return "No repositories are configured. Add one under `artifacts.repositories` in `atmos.yaml`."
	}
	return "Configured repositories: " + strings.Join(configured, ", ") + "."
}
