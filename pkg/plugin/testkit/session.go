package testkit

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ExpireOIDCSession advances the starter's cached access-token expiry in fixture storage.
// This helper is only used by conformance against isolated memory/file backends.
func ExpireOIDCSession(storage keyring.Keyring) error {
	defer perf.Track(nil, "testkit.ExpireOIDCSession")()
	keys, err := storage.List()
	if err != nil {
		return err
	}
	for _, key := range keys {
		if !strings.HasPrefix(key, "atmos_plugin_") {
			continue
		}
		encoded, err := storage.Get(key)
		if err != nil {
			return err
		}
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return err
		}
		var envelope struct {
			Version int               `json:"version"`
			Values  map[string][]byte `json:"values"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return err
		}
		session, found := envelope.Values["session/v1"]
		if !found {
			continue
		}
		expired, err := expireToken(session)
		if err != nil {
			return err
		}
		envelope.Values["session/v1"] = expired
		raw, err = json.Marshal(envelope)
		if err != nil {
			return err
		}
		return storage.Set(key, base64.StdEncoding.EncodeToString(raw))
	}
	return errUtils.ErrTestsFailed
}

func expireToken(raw []byte) ([]byte, error) {
	var session map[string]any
	if err := json.Unmarshal(raw, &session); err != nil {
		return nil, err
	}
	token, found := session["token"].(map[string]any)
	if !found {
		return nil, errUtils.ErrTestsFailed
	}
	token["expiry"] = time.Unix(1, 0).UTC().Format(time.RFC3339)
	return json.Marshal(session)
}
