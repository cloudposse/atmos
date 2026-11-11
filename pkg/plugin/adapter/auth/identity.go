package auth

import (
	"context"
	"errors"

	errUtils "github.com/cloudposse/atmos/errors"
	authregistry "github.com/cloudposse/atmos/pkg/auth/registry"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Materializer applies decoded credentials to an Atmos execution environment.
// Credential contracts provide this seam; the plugin runtime never writes cloud files.
type Materializer interface {
	Apply(context.Context, *types.PostAuthenticateParams) error
	Environment() (map[string]string, error)
	Paths() ([]types.Path, error)
	Logout(context.Context) error
	SetRealm(string)
	Load(context.Context) (types.ICredentials, error)
	PrepareEnvironment(map[string]string) (map[string]string, error)
	SetRootProvider(string)
}

// MaterializerFactory creates invocation-local cloud file/environment state.
type MaterializerFactory func(string, *schema.Identity) Materializer

// IdentityConstructor registers a native identity with explicit accepted credential types.
//
//nolint:lintroller // Returns an auth constructor closure; factory functions are exempt from tracking.
func IdentityConstructor(definition *Definition, accepts []string, materialize MaterializerFactory) authregistry.IdentityConfigConstructor {
	return func(name string, config *schema.Identity) (types.Identity, error) {
		if materialize == nil {
			return nil, errUtils.ErrPluginInvalidConfig
		}
		return &identity{name: name, config: config, definition: definition, accepts: accepts, materializer: materialize(name, config)}, nil
	}
}

type identity struct {
	name         string
	rootProvider string
	config       *schema.Identity
	realm        string
	definition   *Definition
	accepts      []string
	materializer Materializer
}

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (i *identity) Kind() string { return i.config.Kind }

func (i *identity) SetRealm(realm string) {
	defer perf.Track(nil, "auth.identity.SetRealm")()

	i.realm = realm
	i.materializer.SetRealm(realm)
}

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (i *identity) GetProviderName() (string, error) {
	if i.rootProvider != "" {
		return i.rootProvider, nil
	}
	if i.config.Via != nil && i.config.Via.Provider != "" {
		return i.config.Via.Provider, nil
	}
	if i.config.Provider != "" {
		return i.config.Provider, nil
	}
	return "", errUtils.ErrInvalidIdentityConfig
}

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (i *identity) Validate() error {
	if i.config.Via == nil || (i.config.Via.Provider == "" && i.config.Via.Identity == "") {
		return errUtils.ErrInvalidIdentityConfig
	}
	if i.definition.SchemaTarget == "spec" {
		if i.Kind() == "secrets/env" {
			if _, legacy := i.config.Principal["environment"]; legacy {
				return errUtils.Build(errUtils.ErrPluginInvalidConfig).
					WithExplanation("principal.environment is no longer supported for secrets/env.").
					WithHint("Use spec.env with explicit .credentials templates.").Err()
			}
		}
		return i.definition.Schema(i.config.Spec)
	}
	return i.definition.Schema(i.config.Principal)
}

func (i *identity) Authenticate(ctx context.Context, upstream types.ICredentials) (types.ICredentials, error) {
	defer perf.Track(nil, "auth.identity.Authenticate")()

	encoded, err := i.definition.Credentials.Encode(upstream, i.accepts)
	if err != nil {
		return nil, err
	}
	principal, err := toStruct(i.config.Principal)
	if err != nil {
		return nil, err
	}
	spec, err := toStruct(i.config.Spec)
	if err != nil {
		return nil, err
	}
	var result types.ICredentials
	err = i.definition.invoke(ctx, i.name, i.realm, i.sessionSettings(), func(call *invocation) error {
		stream, err := call.handle.Identity.Authenticate(ctx, &pluginv1.IdentityAuthenticateRequest{
			Identity: &pluginv1.IdentityRequest{Invocation: call.request, Kind: i.Kind(), Name: i.name, Principal: principal, Spec: spec, Realm: i.realm}, Upstream: encoded,
		})
		if err != nil {
			return err
		}
		envelope, err := receiveCredentials(stream)
		if err != nil {
			return err
		}
		result, err = i.definition.Credentials.Decode(envelope, i.definition.Outputs)
		return err
	})
	return result, err
}

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (i *identity) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error {
	return i.materializer.Apply(ctx, params)
}

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (i *identity) Environment() (map[string]string, error) { return i.materializer.Environment() }

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (i *identity) Paths() ([]types.Path, error) { return i.materializer.Paths() }

func (i *identity) Logout(ctx context.Context) error {
	defer perf.Track(nil, "auth.identity.Logout")()

	principal, err := toStruct(i.config.Principal)
	if err != nil {
		return err
	}
	spec, err := toStruct(i.config.Spec)
	if err != nil {
		return err
	}
	logoutErr := i.definition.logout(ctx, i.name, i.realm, i.sessionSettings(), func(call *invocation) error {
		_, err := call.handle.Identity.Logout(ctx, &pluginv1.IdentityRequest{Invocation: call.request, Kind: i.Kind(), Name: i.name, Principal: principal, Spec: spec, Realm: i.realm})
		return err
	})
	return errors.Join(logoutErr, i.materializer.Logout(context.WithoutCancel(ctx)))
}

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (i *identity) PrepareEnvironment(_ context.Context, environment map[string]string) (map[string]string, error) {
	return i.materializer.PrepareEnvironment(environment)
}

// CredentialsExist and LoadCredentials use Atmos's credential store, not plugin-owned files.
func (i *identity) CredentialsExist() (bool, error) {
	defer perf.Track(nil, "auth.identity.CredentialsExist")()

	value, err := i.LoadCredentials(context.Background())
	if err != nil {
		return false, err
	}
	return value != nil && !value.IsExpired(), nil
}

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (i *identity) LoadCredentials(ctx context.Context) (types.ICredentials, error) {
	return i.materializer.Load(ctx)
}

func (i *identity) sessionSettings() map[string]any {
	return map[string]any{"kind": i.Kind(), "spec": i.config.Spec, "principal": i.config.Principal, "via": i.config.Via, "provider": i.config.Provider}
}

// SetRootProvider binds cloud-file paths before reuse or logout in a fresh process.
func (i *identity) SetRootProvider(name string) {
	defer perf.Track(nil, "auth.identity.SetRootProvider")()

	i.rootProvider = name
	i.materializer.SetRootProvider(name)
}
