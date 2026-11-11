// Package secrets grants native providers access to explicitly configured secret references.
package secrets

import (
	"context"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/bridge"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	storedeferred "github.com/cloudposse/atmos/pkg/store/deferred"
)

const maxSecretBytes = 1 << 20

type reader struct {
	config     *schema.AtmosConfiguration
	provider   string
	references map[string]any
}

// Factory binds access to the configured provider, excluding identity invocations.
func Factory(config *schema.AtmosConfiguration) func(string, map[string]any) bridge.SecretReader {
	defer perf.Track(config, "secrets.Factory")()

	return func(name string, settings map[string]any) bridge.SecretReader {
		if _, identity := settings["principal"]; identity {
			return nil
		}
		provider, exists := config.Auth.Providers[name]
		if !exists || provider.Kind != "secrets/store" || settings["kind"] != provider.Kind {
			return nil
		}
		references, _ := provider.Spec["credentials"].(map[string]any)
		return &reader{config: config, provider: name, references: references}
	}
}

// ReadSecret resolves only a configured logical name, masking its value before returning it.
func (r *reader) ReadSecret(ctx context.Context, name string) (string, error) {
	defer perf.Track(r.config, "secrets.reader.ReadSecret")()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validateGraph(r.config, "provider:"+r.provider, make(map[string]bool)); err != nil {
		return "", err
	}
	reference, err := r.reference(name)
	if err != nil {
		return "", err
	}
	value, err := r.readValue(ctx, reference)
	if err != nil {
		return "", errUtils.ErrAuthenticationFailed
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok || strings.ContainsRune(text, 0) || len(text) > maxSecretBytes {
		return "", errUtils.ErrPluginProtocol
	}
	atmosio.RegisterSecret(text)
	return text, nil
}

type secretReference struct {
	store, path, field, stack, component string
}

func (r *reader) reference(name string) (*secretReference, error) {
	raw, ok := r.references[name].(map[string]any)
	if !ok {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	ref := &secretReference{}
	ref.store, _ = raw["store"].(string)
	ref.path, _ = raw["path"].(string)
	ref.stack, _ = raw["stack"].(string)
	ref.component, _ = raw["component"].(string)
	if r.config.Stores[ref.store] == nil || ref.path == "" || !r.config.StoresConfig[ref.store].Secret || ((ref.stack == "") != (ref.component == "")) {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	if rawField, exists := raw["field"]; exists {
		var ok bool
		ref.field, ok = rawField.(string)
		if !ok || ref.field == "" {
			return nil, errUtils.ErrPluginInvalidConfig
		}
		if _, ok := r.config.Stores[ref.store].(store.FieldStore); !ok {
			return nil, errUtils.ErrPluginInvalidConfig
		}
	}
	return ref, nil
}

func (r *reader) readValue(ctx context.Context, ref *secretReference) (any, error) {
	backend := r.config.Stores[ref.store]
	// A fresh resolver avoids re-entering the manager currently authenticating this
	// provider. Unpinned stores use ambient credentials, never the current identity.
	config := *r.config
	config.AuthManager = authdeferred.NewManager(authdeferred.AuthOptions{Config: &config})
	info := &schema.ConfigAndStacksInfo{Stack: ref.stack, AuthDisabled: config.StoresConfig[ref.store].Identity == ""}
	return storedeferred.WithStoreAuth(&config, info, ref.store, func() (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if ref.field != "" {
			fieldBackend := backend.(store.FieldStore)
			if ref.stack != "" {
				return fieldBackend.GetField(ref.stack, ref.component, ref.path, ref.field)
			}
			return fieldBackend.GetKeyField(ref.path, ref.field)
		}
		if ref.stack != "" {
			return backend.Get(ref.stack, ref.component, ref.path)
		}
		return backend.GetKey(ref.path)
	})
}
