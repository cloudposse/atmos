package compose

import (
	"context"
	"fmt"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
)

// Cloud prefixes of Atmos identity kinds. The kind of an identity selects which credential type a
// backend receives.
const (
	kindPrefixAWS   = "aws/"
	kindPrefixAzure = "azure/"
	kindPrefixGCP   = "gcp/"
)

// authBridgeFactory builds the bridge from Atmos auth into cloud credentials. It is a seam so the
// resolver can be tested without a real auth manager.
type authBridgeFactory func(config *schema.AtmosConfiguration) (store.AuthContextResolver, error)

// identityResolver lazily resolves Atmos auth identities for artifact backends.
type identityResolver struct {
	config *schema.AtmosConfiguration
	build  authBridgeFactory

	once   sync.Once
	bridge store.AuthContextResolver
	err    error
}

// NewIdentityResolver returns the default lazy resolver: it does nothing until a backend asks for
// credentials, then authenticates the named Atmos identity on an isolated manager and returns the
// credential type of the identity's cloud (AWS, Azure, or GCP). Repository options never carry
// credentials, and an identity that resolves to no usable credentials fails closed.
func NewIdentityResolver(config *schema.AtmosConfiguration) artifact.IdentityResolver {
	defer perf.Track(config, "compose.NewIdentityResolver")()

	return newIdentityResolver(config, defaultAuthBridge).resolve
}

func newIdentityResolver(config *schema.AtmosConfiguration, build authBridgeFactory) *identityResolver {
	return &identityResolver{config: config, build: build}
}

func defaultAuthBridge(config *schema.AtmosConfiguration) (store.AuthContextResolver, error) {
	manager, err := auth.NewDefaultManager(&config.Auth, config.CliConfigPath)
	if err != nil {
		return nil, err
	}
	return authbridge.NewResolver(manager, &schema.ConfigAndStacksInfo{}), nil
}

func (r *identityResolver) resolve(ctx context.Context, identity string) (any, error) {
	configured, found := r.config.Auth.Identities[identity]
	if !found {
		return nil, fmt.Errorf("%w: artifact repository identity %q is not defined under auth.identities", errUtils.ErrIdentityNotFound, identity)
	}
	bridge, err := r.bridgeOnce()
	if err != nil {
		return nil, err
	}
	switch kind := configured.Kind; {
	case strings.HasPrefix(kind, kindPrefixAWS):
		return bridge.ResolveAWSAuthContext(ctx, identity)
	case strings.HasPrefix(kind, kindPrefixAzure):
		return bridge.ResolveAzureAuthContext(ctx, identity)
	case strings.HasPrefix(kind, kindPrefixGCP):
		return bridge.ResolveGCPAuthContext(ctx, identity)
	default:
		return nil, fmt.Errorf("%w: identity %q has kind %q, which provides no cloud credentials for an artifact repository", errUtils.ErrBlobIdentityUnresolved, identity, kind)
	}
}

// bridgeOnce builds the auth bridge on first use and remembers the outcome.
func (r *identityResolver) bridgeOnce() (store.AuthContextResolver, error) {
	r.once.Do(func() { r.bridge, r.err = r.build(r.config) })
	return r.bridge, r.err
}
