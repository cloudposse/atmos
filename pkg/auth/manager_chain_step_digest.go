package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// The cross-process credential cache (the keyring and each identity's own credential files) is
// keyed by step name only. A stack or component auth override can give the same name another
// account, role, or start URL, so every persisted step also records a digest of the
// configuration that produced it, and a later process reuses the step only when the digest of
// its current configuration matches (#3720 item 2.1).
//
// The digest is persisted, so it must be computable in any process and must not be derived
// from secrets: it is a plain SHA-256 over canonical JSON of the non-secret fields below, never
// the per-process HMAC of manager_chain_fingerprint.go.
//
// Hashed provider fields: kind, start_url, url, region, username, and spec.
// Hashed identity fields: kind, provider, via (provider and identity), principal, spec, and
// emulator.
// Never hashed: the provider password and identity credentials (secrets); provider driver,
// provider_type, download_browser_driver, browser_type, browser_executable_path,
// auto_provision_identities, session, console, default, and tags; identity default, required,
// alias, env, session, and tags. These do not change which principal a step resolves to.
// Inside principal and spec, map keys that name secret material (see isSecretConfigKey) are
// dropped at every nesting level.

// persistedProviderConfig is the non-secret part of a provider's effective configuration.
type persistedProviderConfig struct {
	Kind     string         `json:"kind"`
	StartURL string         `json:"start_url,omitempty"`
	URL      string         `json:"url,omitempty"`
	Region   string         `json:"region,omitempty"`
	Username string         `json:"username,omitempty"`
	Spec     map[string]any `json:"spec,omitempty"`
}

// persistedIdentityConfig is the non-secret part of an identity's effective configuration.
type persistedIdentityConfig struct {
	Kind      string              `json:"kind"`
	Provider  string              `json:"provider,omitempty"`
	Via       *schema.IdentityVia `json:"via,omitempty"`
	Principal map[string]any      `json:"principal,omitempty"`
	Spec      map[string]any      `json:"spec,omitempty"`
	Emulator  string              `json:"emulator,omitempty"`
}

// persistedStepConfig is one chain step in the persisted configuration digest.
type persistedStepConfig struct {
	Step     string                   `json:"step"`
	Name     string                   `json:"name"`
	Provider *persistedProviderConfig `json:"provider,omitempty"`
	Identity *persistedIdentityConfig `json:"identity,omitempty"`
}

// secretConfigKeyFragments mark principal and spec keys whose values are secret material.
var secretConfigKeyFragments = []string{
	"password", "passwd", "passphrase", "secret", "token", "credential",
	"private_key", "privatekey", "access_key", "accesskey", "api_key", "apikey",
	"external_id", "assertion",
}

// nonSecretConfigKeySuffixes mark keys that only reference secret material (a path, a URL, an
// environment variable name, or an ARN) and are therefore hashed despite a matching fragment.
var nonSecretConfigKeySuffixes = []string{"_path", "_file", "_url", "_uri", "_endpoint", "_env", "_arn"}

// isSecretConfigKey reports whether a principal or spec key holds secret material and must be
// left out of the persisted digest.
func isSecretConfigKey(key string) bool {
	lower := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	for _, suffix := range nonSecretConfigKeySuffixes {
		if strings.HasSuffix(lower, suffix) {
			return false
		}
	}
	for _, fragment := range secretConfigKeyFragments {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

// nonSecretMap returns a copy of m without secret keys, recursively.
func nonSecretMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for key, value := range m {
		if isSecretConfigKey(key) {
			continue
		}
		out[key] = nonSecretValue(value)
	}
	return out
}

// nonSecretValue removes secret keys from maps nested anywhere inside value.
func nonSecretValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return nonSecretMap(v)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = nonSecretValue(item)
		}
		return out
	default:
		return value
	}
}

// persistedStepConfigAt returns the non-secret effective configuration of chain step i.
func (m *manager) persistedStepConfigAt(i int, name string) persistedStepConfig {
	step := m.chainStepConfig(i, name)
	out := persistedStepConfig{Step: step.Step, Name: step.Name}
	if p := step.Provider; p != nil {
		out.Provider = &persistedProviderConfig{
			Kind: p.Kind, StartURL: p.StartURL, URL: p.URL, Region: p.Region, Username: p.Username,
			Spec: nonSecretMap(p.Spec),
		}
	}
	if id := step.Identity; id != nil {
		out.Identity = &persistedIdentityConfig{
			Kind: id.Kind, Provider: id.Provider, Via: id.Via,
			Principal: nonSecretMap(id.Principal), Spec: nonSecretMap(id.Spec), Emulator: id.Emulator,
		}
	}
	return out
}

// chainPrefixDigest returns the persisted configuration digest of chain[index]: a SHA-256 over
// the non-secret configuration of every step from the root through index, because a step's
// credentials also depend on the steps that produced its source credentials. It returns false
// when the configuration cannot be serialized; such a step is never reused across processes.
func (m *manager) chainPrefixDigest(chain []string, index int) (string, bool) {
	if index < 0 || index >= len(chain) {
		return "", false
	}
	steps := make([]persistedStepConfig, 0, index+1)
	for i := 0; i <= index; i++ {
		steps = append(steps, m.persistedStepConfigAt(i, chain[i]))
	}
	serialized, err := json.Marshal(steps)
	if err != nil {
		log.Debug("Chain step configuration cannot be digested; its cached credentials are not reused", logKeyChain, chain[:index+1], "error", err)
		return "", false
	}
	sum := sha256.Sum256(serialized)
	return hex.EncodeToString(sum[:]), true
}

// namedStepDigest returns the persisted configuration digest of the named identity or provider,
// computed over the chain that leads to it, so it equals chainPrefixDigest at that step's index
// in any chain that passes through it.
func (m *manager) namedStepDigest(name string) (string, bool) {
	if m.config != nil {
		if _, isIdentity := m.config.Identities[name]; isIdentity {
			chain, err := m.buildAuthenticationChain(name)
			if err != nil {
				return "", false
			}
			return m.chainPrefixDigest(chain, len(chain)-1)
		}
	}
	return m.chainPrefixDigest([]string{name}, 0)
}
