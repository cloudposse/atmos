// Package broker provides a registry of credential brokers: extension points that
// lazily provision credentials from the ambient environment (not claimed by any stack)
// and contribute environment variables for remote git and subprocess operations.
//
// A broker is registered once at startup (typically from an implementer package's init())
// and is consulted the first time Atmos performs a remote read (go-getter fetch) or spawns
// a credential-bearing subprocess (terraform/helmfile/packer). Atmos Pro's github/sts
// integration is the first broker; future brokers (e.g., a Vault git-token broker) plug in
// here without touching the downloader or the command layer.
//
// This package intentionally depends only on the standard library, the schema package, and
// the logger so that low-level packages (e.g., pkg/downloader) can import it without creating
// an import cycle back through pkg/auth.
package broker

import (
	"context"
	"os"
	"sync"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Provider is a credential broker. Implementers register themselves via Register
// (usually from an init() function) and are consulted by EnsureCredentials.
type Provider interface {
	// Name returns a stable identifier for the broker (used only for logging).
	Name() string

	// Enabled reports whether this broker should run given the current configuration
	// and environment. Implementations gate on cheap signals (e.g., CI detection and
	// the presence of the relevant auth config) and must not perform network calls.
	Enabled(atmosConfig *schema.AtmosConfiguration) bool

	// Provision provisions credentials and returns environment variables to export
	// into the current process. Returning an empty map is a valid no-op outcome.
	// Token values must never be logged by the implementation.
	Provision(ctx context.Context, atmosConfig *schema.AtmosConfiguration) (map[string]string, error)
}

var (
	registryMu sync.Mutex
	registry   []Provider

	// The ensureOnce guard makes EnsureCredentials run the brokers at most once per process,
	// so repeated remote reads within a single command do not re-provision.
	ensureOnce sync.Once

	// The brokeredMu mutex guards brokeredCredentials.
	brokeredMu sync.Mutex
	// The brokeredCredentials flag records whether any enabled broker provisioned a
	// non-empty environment this process (e.g., Atmos Pro github/sts minted a token).
	// Consumers use HasBrokeredCredentials to decide whether to tolerate a freshly minted
	// token's brief post-creation auth window (see HasBrokeredCredentials).
	brokeredCredentials bool
)

// Register adds a credential broker to the registry. It is safe to call from init().
func Register(p Provider) {
	if p == nil {
		return
	}

	registryMu.Lock()
	defer registryMu.Unlock()
	registry = append(registry, p)
}

// EnsureCredentials runs every enabled broker exactly once per process and exports each
// broker's contributed environment variables into the current process via os.Setenv, so
// that Atmos's own go-getter git subprocesses and any downstream terraform/helmfile/packer
// subprocesses (whose environments start from os.Environ()) transparently pick them up.
//
// It is best-effort: a broker that is not enabled is skipped, and a broker that errors is
// logged at debug and skipped — the in-progress remote read then proceeds and fails
// naturally if the credentials were truly required.
func EnsureCredentials(ctx context.Context, atmosConfig *schema.AtmosConfiguration) {
	defer perf.Track(atmosConfig, "auth.broker.EnsureCredentials")()

	ensureOnce.Do(func() {
		runEnabledBrokers(ctx, atmosConfig)
	})
}

// runEnabledBrokers runs every enabled broker once and exports its env via os.Setenv.
// Split out from EnsureCredentials (which guards it with sync.Once) so it can be tested directly.
func runEnabledBrokers(ctx context.Context, atmosConfig *schema.AtmosConfiguration) {
	for _, p := range snapshot() {
		if !p.Enabled(atmosConfig) {
			continue
		}

		log.Debug("Running credential broker", "broker", p.Name())
		env, err := p.Provision(ctx, atmosConfig)
		if err != nil {
			// Non-fatal, but surfaced at Warn (not Debug): when a broker fails, the private
			// reads it was meant to authorize will fail downstream with a bare
			// `git Authentication failed`, and at the default Warning log level a Debug line
			// here would be invisible — leaving the user no causal link to the real cause.
			log.Warn("Credential broker failed; private reads may fail without these credentials", "broker", p.Name(), "error", err)
			continue
		}

		if len(env) > 0 {
			markBrokered()
		}

		for key, value := range env {
			if err := os.Setenv(key, value); err != nil {
				log.Warn("Failed to export credential broker variable", "broker", p.Name(), "key", key, "error", err)
			}
		}
	}
}

// markBrokered records that a broker provisioned credentials this process.
func markBrokered() {
	brokeredMu.Lock()
	brokeredCredentials = true
	brokeredMu.Unlock()
}

// HasBrokeredCredentials reports whether any enabled broker provisioned credentials (a
// non-empty environment) this process. The git downloader uses this to enable bounded
// retry of transient auth failures: a freshly minted GitHub token can briefly return 401
// before GitHub propagates it across its git frontends, and Atmos should tolerate that
// window only when it minted the token — never for ordinary static credentials, where a
// wrong/expired token must fail fast.
func HasBrokeredCredentials() bool {
	brokeredMu.Lock()
	defer brokeredMu.Unlock()
	return brokeredCredentials
}

// SwapRegistryForTest clears the broker registry and the once guard, returning a restore function
// that puts the previous state back when invoked. Intended for use in tests (including tests in
// other packages, such as pkg/stack/imports) that exercise broker-triggering code paths and need to
// register a fake provider in isolation. Because EnsureCredentials runs brokers at most once per
// process, resetting the once is required for a test to observe a freshly registered broker.
// Production code must never call this.
func SwapRegistryForTest() func() {
	registryMu.Lock()
	prev := registry
	registry = nil
	ensureOnce = sync.Once{}
	registryMu.Unlock()
	resetBrokeredForTest()

	return func() {
		registryMu.Lock()
		registry = prev
		ensureOnce = sync.Once{}
		registryMu.Unlock()
		resetBrokeredForTest()
	}
}

// resetBrokeredForTest clears the brokered-credentials flag so a test observes only what
// its own registered providers provision. Pairs with SwapRegistryForTest.
func resetBrokeredForTest() {
	brokeredMu.Lock()
	brokeredCredentials = false
	brokeredMu.Unlock()
}

// snapshot returns a copy of the registry so brokers run without holding the lock.
func snapshot() []Provider {
	registryMu.Lock()
	defer registryMu.Unlock()

	out := make([]Provider, len(registry))
	copy(out, registry)
	return out
}
