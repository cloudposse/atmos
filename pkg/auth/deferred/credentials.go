package deferred

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Credentials defers identity selection and authentication until resolution.
// Configuration and stack info are copied so nested consumers cannot overwrite
// their caller's selected identity or populated credentials.
func Credentials(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, identity string) deferred.Resolver[*schema.ConfigAndStacksInfo] {
	return deferred.Func[*schema.ConfigAndStacksInfo](func() (*schema.ConfigAndStacksInfo, error) {
		defer perf.Track(ac, "auth.deferred.Credentials.Resolve")()
		config := *ac
		resolved := schema.ConfigAndStacksInfo{}
		if info != nil {
			resolved = *info
		}
		resolved.AuthManager, resolved.AuthContext = nil, nil
		if !AuthDisabled(ac.AuthManager) && !resolved.AuthDisabled && identity != "" {
			merged, err := identityConfig(ac, &resolved, identity)
			if err != nil {
				return nil, err
			}
			config.Auth, resolved.ComponentSection = *merged, nil
		}
		if err := ResolveAuth(&config, &resolved); err != nil {
			return nil, err
		}
		return &resolved, nil
	})
}

// DefaultIdentity selects a backend's default without authenticating it.
func DefaultIdentity(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error) {
	merged, err := auth.MergeComponentAuthFromConfig(&ac.Auth, info.ComponentSection, ac, cfg.AuthSectionName)
	if err != nil {
		return "", err
	}
	for name := range merged.Identities {
		if merged.Identities[name].Default || len(merged.Identities) == 1 {
			return name, nil
		}
	}
	return "", nil
}

func identityConfig(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, identity string) (*schema.AuthConfig, error) {
	merged, err := auth.MergeComponentAuthFromConfig(&ac.Auth, info.ComponentSection, ac, cfg.AuthSectionName)
	if err != nil {
		return nil, err
	}
	if _, exists := merged.Identities[identity]; !exists {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrIdentityNotFound, identity)
	}
	for key := range merged.Identities {
		config := merged.Identities[key]
		config.Default = key == identity
		merged.Identities[key] = config
	}
	return merged, nil
}
