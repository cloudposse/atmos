// Package auth adapts native plugin auth services to the shared Atmos auth authregistry.
package auth

import (
	"context"
	"maps"
	"time"

	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/auth/interactive"
	authregistry "github.com/cloudposse/atmos/pkg/auth/registry"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/browser"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/credential"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Services are Atmos-owned dependencies; plugin configuration cannot replace them.
type Services struct {
	HTTP            api.HTTP
	Keyring         keyring.Keyring
	CredentialStore types.CredentialStore
	Browser         api.Browser
	Interactive     func() bool
	LockDirectory   string
}

// Definition binds a statically validated export to its immutable package executable.
type Definition struct {
	Package     string
	Expected    *pluginv1.DescribeResponse
	Executable  string
	Directory   string
	Outputs     []string
	Schema      func(any) error
	Services    Services
	Credentials *credential.Registry
	// Timeout bounds every call into the plugin; zero means no deadline.
	Timeout time.Duration
}

// ProviderConstructor supplies the same full-configuration constructor used by built-in auth.
//
//nolint:lintroller // Returns an auth constructor closure; factory functions are exempt from tracking.
func ProviderConstructor(definition *Definition) authregistry.ProviderConfigConstructor {
	return func(name string, config *schema.Provider) (types.Provider, error) {
		return &provider{name: name, config: config, definition: definition}, nil
	}
}

type provider struct {
	name       string
	config     *schema.Provider
	realm      string
	definition *Definition
}

// UsesPrivateCredentialCache binds reuse to the plugin session configuration fingerprint.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (*provider) UsesPrivateCredentialCache() bool { return true }

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (p *provider) Kind() string { return p.config.Kind }

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (p *provider) Name() string { return p.name }

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (p *provider) SetRealm(realm string) { p.realm = realm }

// PreAuthenticate requires no chain inspection: accepted/produced types are static metadata.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (*provider) PreAuthenticate(types.AuthManager) error { return nil }

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (p *provider) Validate() error { return p.definition.Schema(p.config.Spec) }

// Authenticate uses the plugin's own persisted session for reuse and refresh.
func (p *provider) Authenticate(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "auth.provider.Authenticate")()

	spec, err := toStruct(p.config.Spec)
	if err != nil {
		return nil, err
	}
	var result types.ICredentials
	err = p.definition.invoke(ctx, p.name, p.realm, p.sessionSettings(), func(call *invocation) error {
		stream, err := call.handle.Provider.Authenticate(ctx, &pluginv1.ProviderRequest{Invocation: call.request, Name: p.name, Kind: p.Kind(), Spec: spec, Realm: p.realm})
		if err != nil {
			return err
		}
		envelope, err := receiveCredentials(stream)
		if err != nil {
			return err
		}
		result, err = p.definition.Credentials.Decode(envelope, p.definition.Outputs)
		return err
	})
	return result, err
}

// Logout revokes remote credentials and removes private state under the same session lock as refresh.
func (p *provider) Logout(ctx context.Context) error {
	defer perf.Track(nil, "auth.provider.Logout")()

	spec, err := toStruct(p.config.Spec)
	if err != nil {
		return err
	}
	return p.definition.logout(ctx, p.name, p.realm, p.sessionSettings(), func(call *invocation) error {
		_, err := call.handle.Provider.Logout(ctx, &pluginv1.ProviderRequest{Invocation: call.request, Name: p.name, Kind: p.Kind(), Spec: spec, Realm: p.realm})
		return err
	})
}

// Environment is empty because this provider exports credentials for downstream identities.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (*provider) Environment() (map[string]string, error) { return map[string]string{}, nil }

// Paths is empty because plugin-private state lives in Atmos's keyring.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (*provider) Paths() ([]types.Path, error) { return nil, nil }

//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (*provider) GetFilesDisplayPath() string { return "" }

// PrepareEnvironment preserves the caller's environment without aliasing it.
func (*provider) PrepareEnvironment(_ context.Context, environment map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "auth.provider.PrepareEnvironment")()

	return maps.Clone(environment), nil
}

func defaultBrowser(services *Services) (api.Browser, bool) {
	available := interactive.Available
	if services.Interactive != nil {
		available = services.Interactive
	}
	interactive := available()
	if services.Browser != nil {
		return services.Browser, interactive
	}
	return api.LocalBrowser{Interactive: interactive, OpenURL: browser.New().Open}, interactive
}

func (p *provider) sessionSettings() map[string]any {
	return map[string]any{"kind": p.Kind(), "spec": p.config.Spec}
}
