package registryauth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

func (r *Resolver) registryCredentials(ctx context.Context, selected *schema.AuthConfig, whoami *types.WhoamiInfo, identity, host string) (authn.Authenticator, error) {
	if whoami == nil || whoami.Credentials == nil {
		return nil, errUtils.ErrIdentityCredentialsNone
	}
	integration, err := matchingIntegration(selected, host)
	if err != nil {
		return nil, err
	}
	if integration == "" {
		identityConfig := selected.Identities[identity]
		return directCredentials(whoami, &identityConfig, host)
	}
	configured := selected.Integrations[integration]
	return r.exchangedCredentials(ctx, &integrations.IntegrationConfig{Name: integration, Config: &configured, Realm: whoami.Realm}, whoami.Credentials, host)
}

func (r *Resolver) exchangedCredentials(ctx context.Context, config *integrations.IntegrationConfig, credentials types.ICredentials, host string) (authn.Authenticator, error) {
	raw, err := json.Marshal([]any{config, credentials})
	if err != nil {
		return nil, fmt.Errorf("%w: cannot identify registry authentication session", errUtils.ErrIdentityAuthFailed)
	}
	session := sha256.Sum256(raw)
	if cached, ok := r.cached[host]; ok && cached.session == session && time.Now().Before(cached.expires) {
		return cached.authenticator, nil
	}
	minted, err := r.mintCredentials(ctx, config, credentials, host)
	if err != nil {
		return nil, err
	}
	minted.session = session
	r.cached[host] = minted
	return minted.authenticator, nil
}

func (r *Resolver) mintCredentials(ctx context.Context, config *integrations.IntegrationConfig, credentials types.ICredentials, host string) (cachedAuth, error) {
	if config.Realm == "" {
		info, err := realm.GetRealm(r.config.Auth.Realm, r.config.CliConfigPath)
		if err != nil {
			return cachedAuth{}, err
		}
		config.Realm = info.Value
	}
	minted, err := r.exchange(ctx, config, credentials)
	if err != nil {
		return cachedAuth{}, err
	}
	if minted == nil {
		return cachedAuth{}, errUtils.ErrIdentityCredentialsNone
	}
	result, err := fromCredentials(minted, host)
	if err != nil {
		return cachedAuth{}, err
	}
	result, err = withExpiration(result, minted)
	if err != nil {
		return cachedAuth{}, err
	}
	expiry := time.Now().Add(time.Minute)
	if minted.Expiration != nil && minted.Expiration.Before(expiry) {
		expiry = *minted.Expiration
	}
	return cachedAuth{authenticator: result, expires: expiry}, nil
}

func directCredentials(whoami *types.WhoamiInfo, identity *schema.Identity, host string) (authn.Authenticator, error) {
	credentials, ok := whoami.Credentials.(RegistryCredentials)
	if !ok {
		return nil, fmt.Errorf("%w: identity needs an OCI credential contract or a registry integration for %s", errUtils.ErrIdentityAuthFailed, host)
	}
	if err := validateCredentialSelection(identity, credentials); err != nil {
		return nil, err
	}
	result, err := fromCredentials(credentials, host)
	if err != nil {
		return nil, err
	}
	return withExpiration(result, whoami.Credentials)
}
