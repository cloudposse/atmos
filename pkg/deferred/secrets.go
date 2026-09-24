package deferred

import (
	"context"
	"strings"

	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	fnparser "github.com/cloudposse/atmos/pkg/function/parser"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Local secrets (including age-encrypted files) must not authenticate a cloud identity.
func PrepareSecretAuth(ac *schema.AtmosConfiguration, input string, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(ac, "deferred.PrepareSecretAuth")()

	parsed, err := fnparser.ParseSecret(strings.TrimSpace(strings.TrimPrefix(input, u.AtmosYamlFuncSecret)))
	if err != nil {
		return err
	}
	decl, exists := secrets.LookupDeclaration(info.ComponentSection, parsed.Name)
	if !exists {
		return nil
	} // The secret resolver reports the malformed reference.
	if decl.BackendType == secrets.BackendStore {
		return ResolveStoreAuth(ac, info, decl.BackendName)
	}
	if AuthDisabled(ac) || info.AuthDisabled {
		return nil
	}
	merged, err := auth.MergeComponentAuthFromConfig(&ac.Auth, info.ComponentSection, ac, cfg.AuthSectionName)
	if err != nil {
		return err
	}
	identity := ""
	for name := range merged.Identities {
		if merged.Identities[name].Default || len(merged.Identities) == 1 {
			identity = name
		}
	}
	ac.SecretsAuth = &store.SecretsAuthContext{Resolver: authbridge.NewContextResolver((&deferredSecretContext{config: ac, info: info}).resolve), DefaultIdentity: identity}
	return nil
}

type deferredSecretContext struct {
	config *schema.AtmosConfiguration
	info   *schema.ConfigAndStacksInfo
}

func (r *deferredSecretContext) resolve(_ context.Context, identity string) (*schema.AuthContext, error) {
	config, info := *r.config, *r.info
	if identity != "" {
		merged, err := deferredStoreIdentity(&config, &info, identity)
		if err != nil {
			return nil, err
		}
		config.Auth, info.ComponentSection = *merged, nil
	}
	if err := ResolveAuth(&config, &info); err != nil {
		return nil, err
	}
	return info.AuthContext, nil
}
