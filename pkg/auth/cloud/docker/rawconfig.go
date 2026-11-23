package docker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	// Top-level config.json keys Atmos reads. Every other key is carried through untouched.
	keyAuths       = "auths"
	keyCredsStore  = "credsStore"
	keyCredHelpers = "credHelpers"
)

// rawConfig is a lossless view of Docker's config.json. Only the keys Atmos needs
// are decoded; every top-level key and every auths entry is kept as raw JSON so
// WriteAuth/RemoveAuth never drop fields Atmos does not model (credsStore,
// credHelpers, currentContext, plugins, proxies, identitytoken, and so on).
type rawConfig struct {
	// fields holds every top-level key exactly as read.
	fields map[string]json.RawMessage
	// auths holds every auths entry exactly as read, keyed by registry.
	auths map[string]json.RawMessage
	// hadAuths records whether the file had an auths key, so a round trip that
	// leaves auths empty does not introduce a key the user never had.
	hadAuths bool
	// credsStore is the global credential helper name, if any.
	credsStore string
	// credHelpers maps registry hosts to per-registry credential helper names.
	credHelpers map[string]string
}

// newRawConfig returns an empty config.
func newRawConfig() *rawConfig {
	return &rawConfig{
		fields: make(map[string]json.RawMessage),
		auths:  make(map[string]json.RawMessage),
	}
}

// parseRawConfig decodes config.json content. Empty or whitespace-only content is
// treated as an empty config, matching docker/cli.
func parseRawConfig(data []byte) (*rawConfig, error) {
	cfg := newRawConfig()
	if len(bytes.TrimSpace(data)) == 0 {
		return cfg, nil
	}

	if err := json.Unmarshal(data, &cfg.fields); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON: %w", errUtils.ErrDockerConfigRead, err)
	}
	if cfg.fields == nil {
		// The document was the JSON literal null.
		cfg.fields = make(map[string]json.RawMessage)
	}

	if raw, ok := cfg.fields[keyAuths]; ok {
		cfg.hadAuths = true
		if err := decodeOptional(raw, &cfg.auths); err != nil {
			return nil, fmt.Errorf("%w: invalid %q: %w", errUtils.ErrDockerConfigRead, keyAuths, err)
		}
	}
	if raw, ok := cfg.fields[keyCredsStore]; ok {
		if err := decodeOptional(raw, &cfg.credsStore); err != nil {
			return nil, fmt.Errorf("%w: invalid %q: %w", errUtils.ErrDockerConfigRead, keyCredsStore, err)
		}
	}
	if raw, ok := cfg.fields[keyCredHelpers]; ok {
		if err := decodeOptional(raw, &cfg.credHelpers); err != nil {
			return nil, fmt.Errorf("%w: invalid %q: %w", errUtils.ErrDockerConfigRead, keyCredHelpers, err)
		}
	}

	// cfg.auths stays non-nil: newRawConfig allocates it, decoding an object
	// merges into it, and decodeOptional skips JSON null.
	return cfg, nil
}

// decodeOptional unmarshals raw into v, treating JSON null as "not set".
func decodeOptional(raw json.RawMessage, v any) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	return json.Unmarshal(raw, v)
}

// credentialHelperFor returns the credential helper Docker uses for registry, or
// "" when Docker reads the inline auths entry. It mirrors docker/cli
// (cli/config/configfile/file.go, getConfiguredCredentialStore): a credHelpers
// entry for the registry wins, even when empty, otherwise the global credsStore.
func (c *rawConfig) credentialHelperFor(registry string) string {
	if helper, ok := c.credHelpers[registry]; ok {
		return helper
	}
	return c.credsStore
}

// setAuth replaces the auths entry for registry.
func (c *rawConfig) setAuth(registry string, entry json.RawMessage) {
	c.auths[registry] = entry
}

// hasAuth reports whether config.json has an auths entry for registry.
func (c *rawConfig) hasAuth(registry string) bool {
	_, ok := c.auths[registry]
	return ok
}

// removeAuth deletes the auths entry for registry.
func (c *rawConfig) removeAuth(registry string) {
	delete(c.auths, registry)
}

// authKeysFor returns, sorted, every auths key Docker reads for host: the exact
// key and any legacy key that normalizes to it ("https://host", "host/v2/").
// The docker/cli file store falls back to such keys when the exact one is
// missing (cli/config/credentials/file_store.go, fileStore.Get), so a logout
// must remove them all for the registry to stay logged out.
func (c *rawConfig) authKeysFor(host string) []string {
	var keys []string
	for key := range c.auths {
		if key == host || convertToHostname(key) == host {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// registries returns the registries with an auths entry, sorted.
func (c *rawConfig) registries() []string {
	out := make([]string, 0, len(c.auths))
	for registry := range c.auths {
		out = append(out, registry)
	}
	sort.Strings(out)
	return out
}

// marshal encodes the config. Untouched values are emitted verbatim: no HTML
// escaping and no re-formatting of numbers or strings.
func (c *rawConfig) marshal() ([]byte, error) {
	if len(c.auths) > 0 || c.hadAuths {
		authsJSON, err := encodeJSON(c.auths, "")
		if err != nil {
			return nil, err
		}
		c.fields[keyAuths] = authsJSON
	} else {
		delete(c.fields, keyAuths)
	}
	return encodeJSON(c.fields, "  ")
}

// encodeJSON marshals v without HTML escaping, optionally indented.
func encodeJSON(v any, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if indent != "" {
		enc.SetIndent("", indent)
	}
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// inlineAuthEntry returns the auths entry docker/cli writes for the file store:
// {"auth": base64(username:password)}.
func inlineAuthEntry(auth string) json.RawMessage {
	entry, _ := encodeJSON(map[string]string{"auth": auth}, "")
	return entry
}

// helperPlaceholderEntry is the empty auths entry docker/cli writes next to a
// credential stored in a helper (credentials/native_store.go, Store), so the
// registry is still listed as logged in.
var helperPlaceholderEntry = json.RawMessage(`{}`)
