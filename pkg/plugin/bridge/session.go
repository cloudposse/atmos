package bridge

import (
	"encoding/base64"
	"encoding/json"
	"errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	maxSessionEntries  = 64
	maxSessionKeyBytes = 256
)

type privateSession struct {
	Version int               `json:"version"`
	Values  map[string][]byte `json:"values"`
}

func sessionRecordKey(namespace string) string { return namespacePrefix(namespace) + "session-v1" }

func readSession(storage keyring.Keyring, namespace string) (*privateSession, error) {
	raw, err := storage.Get(sessionRecordKey(namespace))
	if errors.Is(err, keyring.ErrNotFound) {
		return &privateSession{Version: 1, Values: make(map[string][]byte)}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) > base64.StdEncoding.EncodedLen(api.MaxHTTPBody) {
		return nil, errUtils.ErrPluginPayloadTooLarge
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, errUtils.ErrPluginProtocol
	}
	var session privateSession
	if err := json.Unmarshal(decoded, &session); err != nil {
		return nil, errUtils.ErrPluginProtocol
	}
	if session.Version != 1 || session.Values == nil || len(session.Values) > maxSessionEntries {
		return nil, errUtils.ErrPluginProtocol
	}
	return &session, nil
}

func writeSession(storage keyring.Keyring, namespace string, session *privateSession) error {
	if len(session.Values) == 0 {
		return storage.Delete(sessionRecordKey(namespace))
	}
	if len(session.Values) > maxSessionEntries {
		return errUtils.ErrPluginPayloadTooLarge
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return errUtils.ErrPluginProtocol
	}
	if len(raw) > api.MaxHTTPBody {
		return errUtils.ErrPluginPayloadTooLarge
	}
	return storage.Set(sessionRecordKey(namespace), base64.StdEncoding.EncodeToString(raw))
}

// ClearPrivateSession deletes a whole host-selected scope without enumerating the keyring.
// OS keychains support this just like encrypted file and memory backends.
func ClearPrivateSession(storage keyring.Keyring, namespace string) error {
	defer perf.Track(nil, "bridge.ClearPrivateSession")()
	if storage == nil {
		return errUtils.ErrPluginAPI
	}
	if err := storage.Delete(sessionRecordKey(namespace)); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return errors.Join(errUtils.ErrPluginAPI, err)
	}
	return nil
}
