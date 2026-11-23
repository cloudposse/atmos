package config

import (
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// MaxInteractionTimeout caps plugins.packages.<alias>.interaction_timeout. The limit only bounds
// how long a person may take to finish a sign-in prompt; identity providers expire device codes
// and authorization requests within minutes, so a day is far beyond any real sign-in. The cap
// keeps a typo such as 300h from leaving a stalled login holding the plugin session lock for days.
const MaxInteractionTimeout = 24 * time.Hour

// PackageInteractionTimeout parses the optional limit on how long an interactive call may wait
// for the user. Zero means the runtime default (15 minutes).
//
//nolint:lintroller,gocritic // Pure parsing of an immutable configuration value; preserve the public value-based API.
func PackageInteractionTimeout(entry schema.PluginPackage) (time.Duration, error) {
	if entry.InteractionTimeout == "" {
		return 0, nil
	}
	limit, err := time.ParseDuration(entry.InteractionTimeout)
	if err != nil || limit <= 0 {
		return 0, fmt.Errorf("%w: plugins.packages interaction_timeout %q must be a positive duration such as 30m", errUtils.ErrPluginInvalidConfig, entry.InteractionTimeout)
	}
	if limit > MaxInteractionTimeout {
		return 0, fmt.Errorf("%w: plugins.packages interaction_timeout %q exceeds the maximum of %s", errUtils.ErrPluginInvalidConfig, entry.InteractionTimeout, MaxInteractionTimeout)
	}
	return limit, nil
}

// validateInteractionTimeout names the package and key of an invalid interaction_timeout. The key
// only extends the timeout, so setting it without timeout (no deadline at all) is a mistake.
//
//nolint:gocritic // The declaration is an immutable value from the configuration map.
func validateInteractionTimeout(alias string, entry schema.PluginPackage) error {
	limit, err := PackageInteractionTimeout(entry)
	if err != nil {
		hint := fmt.Sprintf("Set plugins.packages.%s.interaction_timeout to a positive Go duration such as `30m`, or remove it for the 15m default.", alias)
		if parsed, parseErr := time.ParseDuration(entry.InteractionTimeout); parseErr == nil && parsed > MaxInteractionTimeout {
			hint = fmt.Sprintf("Set plugins.packages.%s.interaction_timeout to at most 24h, or remove it for the 15m default.", alias)
		}
		return errUtils.Build(fmt.Errorf("%w: plugins.packages.%s.interaction_timeout %q is not a valid sign-in limit", errUtils.ErrPluginInvalidConfig, alias, entry.InteractionTimeout)).
			WithHint(hint).
			Err()
	}
	if limit > 0 && entry.Timeout == "" {
		return errUtils.Build(fmt.Errorf("%w: plugins.packages.%s.interaction_timeout requires plugins.packages.%s.timeout", errUtils.ErrPluginInvalidConfig, alias, alias)).
			WithHintf("Without plugins.packages.%s.timeout a call has no deadline, so there is nothing to extend. Set a timeout such as `5m`, or remove interaction_timeout.", alias).
			Err()
	}
	return nil
}
