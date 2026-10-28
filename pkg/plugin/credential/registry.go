// Package credential translates versioned wire envelopes to Atmos credential types.
package credential

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// Decoder validates and decodes one credential contract.
type Decoder func(*pluginv1.Credentials) (types.ICredentials, error)

// Registry rejects unknown contracts instead of guessing a cloud credential shape.
type Registry struct {
	mu       sync.RWMutex
	decoders map[string]Decoder
	encoders map[reflect.Type]Encoder
}

// New registers the first release's AWS and OIDC credential contracts.
func New() *Registry {
	defer perf.Track(nil, "credential.New")()

	return &Registry{decoders: map[string]Decoder{"aws/v1": decodeAWS, "oidc/v1": decodeOIDC}, encoders: map[reflect.Type]Encoder{reflect.TypeOf(&types.AWSCredentials{}): encodeAWS, reflect.TypeOf(&types.PluginCredentials{}): encodePlugin}}
}

// Register adds a credential contract without replacing a built-in decoder.
func (r *Registry) Register(name string, decoder Decoder) error {
	defer perf.Track(nil, "credential.Registry.Register")()

	if name == "" || decoder == nil {
		return errUtils.ErrInvalidCredentialCodec
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.decoders[name]; exists {
		return errUtils.ErrDuplicateCredentialCodec
	}
	if r.decoders == nil {
		r.decoders = make(map[string]Decoder)
	}
	r.decoders[name] = decoder
	return nil
}

// Decode validates the common envelope before dispatching to a registered codec.
func (r *Registry) Decode(envelope *pluginv1.Credentials, allowed []string) (types.ICredentials, error) {
	defer perf.Track(nil, "credential.Registry.Decode")()

	if envelope == nil || envelope.GetData() == nil || envelope.GetExpiresAt() == nil || envelope.GetExpiresAt().CheckValid() != nil || !time.Now().Before(envelope.GetExpiresAt().AsTime()) || !slices.Contains(allowed, envelope.GetType()) {
		return nil, errUtils.ErrPluginProtocol
	}
	r.mu.RLock()
	decoder, exists := r.decoders[envelope.GetType()]
	r.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrInvalidCredentialCodec, envelope.GetType())
	}
	return decoder(envelope)
}

func decodeAWS(envelope *pluginv1.Credentials) (types.ICredentials, error) {
	raw, err := json.Marshal(envelope.GetData().AsMap())
	if err != nil {
		return nil, errUtils.ErrPluginProtocol
	}
	var value types.AWSCredentials
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, errUtils.ErrPluginProtocol
	}
	if value.AccessKeyID == "" || value.SecretAccessKey == "" || value.SessionToken == "" {
		return nil, errUtils.ErrPluginProtocol
	}
	// The envelope controls lifetime; a contradictory payload cannot extend it.
	expiry, err := time.Parse(time.RFC3339, value.Expiration)
	if err != nil || !time.Now().Before(expiry) {
		return nil, errUtils.ErrPluginProtocol
	}
	if limit := envelope.GetExpiresAt().AsTime(); limit.Before(expiry) {
		value.Expiration = limit.UTC().Format(time.RFC3339)
	}
	return &value, nil
}

func decodeOIDC(envelope *pluginv1.Credentials) (types.ICredentials, error) {
	data := envelope.GetData().AsMap()
	for _, key := range []string{"issuer", "subject", "client_id", "access_token", "token_type", "access_token_expires_at"} {
		if value, ok := data[key].(string); !ok || value == "" {
			return nil, errUtils.ErrPluginProtocol
		}
	}
	expiry, err := time.Parse(time.RFC3339, data["access_token_expires_at"].(string))
	if err != nil || !time.Now().Before(expiry) || envelope.GetExpiresAt().AsTime().After(expiry) {
		return nil, errUtils.ErrPluginProtocol
	}
	if _, exists := data["refresh_token"]; exists {
		return nil, errUtils.ErrPluginProtocol
	}
	return &types.PluginCredentials{Type: envelope.GetType(), ExpiresAt: envelope.GetExpiresAt().AsTime(), Data: data, SensitiveKeys: []string{"access_token", "id_token"}}, nil
}
