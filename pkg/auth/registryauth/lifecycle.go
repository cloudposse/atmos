package registryauth

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

type authenticationKey struct{}

type expiringAuthenticator struct {
	authn.Authenticator
	expires time.Time
}

func authenticationCycle(ctx context.Context, identity string) error {
	active, _ := ctx.Value(authenticationKey{}).(map[string]bool)
	if active[identity] {
		return fmt.Errorf("%w: registry authentication depends on downloading its own auth plugin for identity %s", errUtils.ErrIdentityAuthFailed, identity)
	}
	return nil
}

func guardAuthentication(ctx context.Context, identity string) (context.Context, error) {
	if err := authenticationCycle(ctx, identity); err != nil {
		return nil, err
	}
	active, _ := ctx.Value(authenticationKey{}).(map[string]bool)
	next := maps.Clone(active)
	if next == nil {
		next = make(map[string]bool)
	}
	next[identity] = true
	return context.WithValue(ctx, authenticationKey{}, next), nil
}

func withExpiration(auth authn.Authenticator, credentials types.ICredentials) (authn.Authenticator, error) {
	expiration, err := credentials.GetExpiration()
	if err != nil {
		return nil, err
	}
	if expiration == nil {
		return auth, nil
	}
	if !time.Now().Before(*expiration) {
		return nil, errUtils.ErrIdentityCredentialsNone
	}
	return expiringAuthenticator{Authenticator: auth, expires: *expiration}, nil
}
