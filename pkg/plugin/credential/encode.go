package credential

import (
	"encoding/json"
	"reflect"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// Encoder converts a concrete Atmos credential to a versioned wire contract.
type Encoder func(types.ICredentials) (*pluginv1.Credentials, error)

// RegisterEncoder adds an upstream credential type without replacing an existing mapping.
func (r *Registry) RegisterEncoder(prototype types.ICredentials, encoder Encoder) error {
	defer perf.Track(nil, "credential.Registry.RegisterEncoder")()

	if prototype == nil || encoder == nil {
		return errUtils.ErrInvalidCredentialCodec
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.encoders[reflect.TypeOf(prototype)]; exists {
		return errUtils.ErrDuplicateCredentialCodec
	}
	if r.encoders == nil {
		r.encoders = make(map[reflect.Type]Encoder)
	}
	r.encoders[reflect.TypeOf(prototype)] = encoder
	return nil
}

// Encode supplies upstream credentials only if their contract is accepted by the identity.
func (r *Registry) Encode(value types.ICredentials, accepted []string) (*pluginv1.Credentials, error) {
	defer perf.Track(nil, "credential.Registry.Encode")()

	if value == nil || (reflect.ValueOf(value).Kind() == reflect.Pointer && reflect.ValueOf(value).IsNil()) {
		return nil, errUtils.ErrPluginProtocol
	}
	r.mu.RLock()
	encoder, exists := r.encoders[reflect.TypeOf(value)]
	r.mu.RUnlock()
	if !exists {
		return nil, errUtils.ErrInvalidCredentialCodec
	}
	result, err := encoder(value)
	if err != nil {
		return nil, err
	}
	if _, err := r.Decode(result, accepted); err != nil {
		return nil, err
	}
	return result, nil
}

func encodeAWS(value types.ICredentials) (*pluginv1.Credentials, error) {
	expiration, err := value.GetExpiration()
	if err != nil || expiration == nil {
		return nil, errUtils.ErrPluginProtocol
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, errUtils.ErrPluginProtocol
	}
	data := make(map[string]any)
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, errUtils.ErrPluginProtocol
	}
	fields, err := structpb.NewStruct(data)
	if err != nil {
		return nil, errUtils.ErrPluginProtocol
	}
	return &pluginv1.Credentials{Type: "aws/v1", Data: fields, ExpiresAt: timestamppb.New(*expiration), SensitiveKeys: []string{"access_key_id", "secret_access_key", "session_token"}}, nil
}

func encodePlugin(value types.ICredentials) (*pluginv1.Credentials, error) {
	credential, ok := value.(*types.PluginCredentials)
	if !ok {
		return nil, errUtils.ErrPluginProtocol
	}
	fields, err := structpb.NewStruct(credential.Data)
	if err != nil {
		return nil, errUtils.ErrPluginProtocol
	}
	return &pluginv1.Credentials{Type: credential.Type, Data: fields, ExpiresAt: timestamppb.New(credential.ExpiresAt), SensitiveKeys: credential.SensitiveKeys}, nil
}
