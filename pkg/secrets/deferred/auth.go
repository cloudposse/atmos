package deferred

import (
	"context"
	"strings"

	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	fnparser "github.com/cloudposse/atmos/pkg/function/parser"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
	storedeferred "github.com/cloudposse/atmos/pkg/store/deferred"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Local secrets (including age-encrypted files) must not authenticate a cloud identity.
func PrepareSecretAuth(ac *schema.AtmosConfiguration, input string, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(ac, "secrets.deferred.PrepareSecretAuth")()

	parsed, err := fnparser.ParseSecret(strings.TrimSpace(strings.TrimPrefix(input, u.AtmosYamlFuncSecret)))
	if err != nil {
		return err
	}
	decl, exists := secrets.LookupDeclaration(info.ComponentSection, parsed.Name)
	if !exists {
		return nil
	} // The secret resolver reports the malformed reference.
	if decl.BackendType == secrets.BackendStore {
		return storedeferred.ResolveStoreAuth(ac, info, decl.BackendName)
	}
	if authdeferred.AuthDisabled(ac.AuthManager) || info.AuthDisabled {
		return nil
	}
	identity, err := authdeferred.DefaultIdentity(ac, info)
	if err != nil {
		return err
	}
	ac.SecretsAuth = &store.SecretsAuthContext{Resolver: authbridge.NewContextResolver((&deferredSecretContext{config: ac, info: info}).resolve), DefaultIdentity: identity}
	return nil
}

type deferredSecretContext struct {
	config *schema.AtmosConfiguration
	info   *schema.ConfigAndStacksInfo
}

func (r *deferredSecretContext) resolve(_ context.Context, identity string) (*schema.AuthContext, error) {
	info, err := authdeferred.Credentials(r.config, r.info, identity).Resolve()
	if err != nil {
		return nil, err
	}
	return info.AuthContext, nil
}
