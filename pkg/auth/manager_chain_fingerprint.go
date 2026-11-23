package auth

import (
	"encoding/json"
	"strings"

	"github.com/cloudposse/atmos/pkg/auth/fingerprint"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// chainStepConfig is the effective configuration of one chain step, serialized into
// the process credential cache key so that stack or component auth overrides that
// give the same name a different role, account, or provider never share credentials.
type chainStepConfig struct {
	Step     string           `json:"step"`
	Name     string           `json:"name"`
	Provider *schema.Provider `json:"provider,omitempty"`
	Identity *schema.Identity `json:"identity,omitempty"`
}

// chainCacheKey returns a unique cache key for the current chain, realm, and the
// effective configuration of every chain step. It returns "" when the configuration
// cannot be fingerprinted; callers must then bypass the process cache.
func (m *manager) chainCacheKey() string {
	config, ok := m.chainConfigFingerprint()
	if !ok {
		return ""
	}
	prefix := m.realm.Value + ":"
	if m.operationScope != nil {
		prefix += m.operationScope.realm + ":"
	}
	return prefix + strings.Join(m.chain, "->") + "#" + config
}

// chainConfigFingerprint returns a process-local HMAC of each chain step's effective
// provider or identity configuration. The configuration may hold secrets (for example
// a provider password), so it is keyed with the per-process secret, never a plain digest.
func (m *manager) chainConfigFingerprint() (string, bool) {
	steps := make([]chainStepConfig, 0, len(m.chain))
	for i, name := range m.chain {
		steps = append(steps, m.chainStepConfig(i, name))
	}
	serialized, err := json.Marshal(steps)
	if err != nil {
		log.Debug("Chain configuration cannot be fingerprinted; bypassing process credential cache", logKeyChain, m.chain, "error", err)
		return "", false
	}
	return fingerprint.Key(serialized), true
}

// chainStepConfig resolves the effective configuration of the chain step at index i.
// Only the root can be a provider; a standalone root is an identity.
func (m *manager) chainStepConfig(i int, name string) chainStepConfig {
	step := chainStepConfig{Step: "missing", Name: name}
	if m.config == nil {
		return step
	}
	if i == 0 {
		if provider, ok := m.config.Providers[name]; ok {
			step.Step, step.Provider = "provider", &provider
			return step
		}
	}
	if identity, ok := m.config.Identities[name]; ok {
		step.Step, step.Identity = "identity", &identity
	}
	return step
}
