package credentials

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/providers/mock"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CredentialConstructor returns a fresh non-nil pointer for JSON decoding.
type CredentialConstructor func() types.ICredentials

// CodecRegistry maps persisted envelope tags to credential types. Registering a
// new type never changes the encoding of existing sessions.
type CodecRegistry struct {
	mu     sync.RWMutex
	byTag  map[string]CredentialConstructor
	byType map[reflect.Type]string
}

// NewCodecRegistry registers the legacy JSON representations used by existing sessions.
func NewCodecRegistry() *CodecRegistry {
	defer perf.Track(nil, "credentials.NewCodecRegistry")()

	r := &CodecRegistry{byTag: make(map[string]CredentialConstructor), byType: make(map[reflect.Type]string)}
	for tag, constructor := range map[string]CredentialConstructor{
		"aws":       func() types.ICredentials { return &types.AWSCredentials{} },
		"gcp":       func() types.ICredentials { return &types.GCPCredentials{} },
		"oidc":      func() types.ICredentials { return &types.OIDCCredentials{} },
		"atmos-pro": func() types.ICredentials { return &types.ProCredentials{} },
		"plugin/v1": func() types.ICredentials { return &types.PluginCredentials{} },
		"oci/v1":    func() types.ICredentials { return &types.OCICredentials{} },
		"mock":      func() types.ICredentials { return &mock.Credentials{} },
	} {
		// Built-in tags and Go types are static and unique.
		if err := r.Register(tag, constructor); err != nil {
			panic(err)
		}
	}
	return r
}

var defaultCodecs = NewCodecRegistry()

// RegisterCodec extends credential persistence without replacing a built-in codec.
func RegisterCodec(tag string, constructor CredentialConstructor) error {
	return defaultCodecs.Register(tag, constructor)
}

// Register binds a unique persisted tag to a unique concrete credential type.
// Constructors must return a fresh pointer of the same type on every call.
func (r *CodecRegistry) Register(tag string, constructor CredentialConstructor) error {
	if tag == "" || constructor == nil {
		return errUtils.ErrInvalidCredentialCodec
	}
	value := reflect.ValueOf(constructor())
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return errUtils.ErrInvalidCredentialCodec
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byTag[tag]; ok {
		return fmt.Errorf("%w: %s", errUtils.ErrDuplicateCredentialCodec, tag)
	}
	if _, ok := r.byType[value.Type()]; ok {
		return fmt.Errorf("%w: %s", errUtils.ErrDuplicateCredentialCodec, value.Type())
	}
	if r.byTag == nil {
		r.byTag = make(map[string]CredentialConstructor)
	}
	if r.byType == nil {
		r.byType = make(map[reflect.Type]string)
	}
	r.byTag[tag] = constructor
	r.byType[value.Type()] = tag
	return nil
}

// Marshal encodes credentials using their registered, stable envelope tag.
func (r *CodecRegistry) Marshal(creds types.ICredentials) (string, []byte, error) {
	defer perf.Track(nil, "credentials.CodecRegistry.Marshal")()

	value := reflect.ValueOf(creds)
	if !value.IsValid() || (value.Kind() == reflect.Pointer && value.IsNil()) {
		return "", nil, errors.Join(ErrCredentialStore, ErrUnsupportedCredentialType)
	}
	r.mu.RLock()
	tag, ok := r.byType[value.Type()]
	r.mu.RUnlock()
	if !ok {
		return "", nil, fmt.Errorf("%w: %T", errors.Join(ErrCredentialStore, ErrUnsupportedCredentialType), creds)
	}
	raw, err := json.Marshal(creds)
	if err != nil {
		return "", nil, errors.Join(ErrCredentialStore, err)
	}
	return tag, raw, nil
}

// Unmarshal decodes a stored envelope. Unknown tags fail without interpreting its payload.
func (r *CodecRegistry) Unmarshal(tag string, raw []byte) (types.ICredentials, error) {
	defer perf.Track(nil, "credentials.CodecRegistry.Unmarshal")()

	r.mu.RLock()
	constructor, ok := r.byTag[tag]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", errors.Join(ErrCredentialStore, ErrUnknownCredentialType), tag)
	}
	into := constructor()
	// JSON null must not turn corrupted stored credentials into a successful empty session.
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, errors.Join(ErrCredentialStore, err)
	}
	if object == nil {
		return nil, errors.Join(ErrCredentialStore, errUtils.ErrInvalidCredentialCodec)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return nil, errors.Join(ErrCredentialStore, err)
	}
	return into, nil
}

// marshalCredential maps a registered credential type to its stable JSON representation.
func marshalCredential(creds types.ICredentials) (string, []byte, error) {
	return defaultCodecs.Marshal(creds)
}

// unmarshalCredential reconstructs the registered type named by the stored envelope.
func unmarshalCredential(env credentialEnvelope) (types.ICredentials, error) {
	return defaultCodecs.Unmarshal(env.Type, env.Data)
}
