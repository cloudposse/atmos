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
			// Non-fatal: a private read will surface its own auth error downstream.
			log.Debug("Credential broker failed", "broker", p.Name(), "error", err)
			continue
		}

		for key, value := range env {
			if err := os.Setenv(key, value); err != nil {
				log.Debug("Failed to export credential broker variable", "broker", p.Name(), "key", key, "error", err)
			}
		}
	}
}

// snapshot returns a copy of the registry so brokers run without holding the lock.
func snapshot() []Provider {
	registryMu.Lock()
	defer registryMu.Unlock()

	out := make([]Provider, len(registry))
	copy(out, registry)
	return out
}
