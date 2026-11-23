// Package broker provides a registry of credential brokers: extension points that
// lazily provision credentials from the ambient environment (not claimed by any stack)
// and contribute environment variables for remote git and subprocess operations.
//
// A broker is registered once at startup (typically from an implementer package's init())
// and is consulted when Atmos performs a remote read (go-getter fetch) or spawns a
// credential-bearing subprocess (terraform/helmfile/packer): the first time, again after a
// failed or cancelled attempt, and periodically so a long-lived process refreshes tokens. Atmos Pro's github/sts
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
	"time"

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

	// The ensureMu mutex serializes EnsureCredentials and guards ensureState, so concurrent
	// callers provision once and every caller observes the exported env after it returns.
	ensureMu sync.Mutex
	// The ensureState value records the last completed broker run.
	ensureState brokerRun

	// The refreshInterval duration is how long a fully successful run is reused before the
	// brokers run again. Brokers reuse fresh persisted tokens, so a refresh is cheap, and it
	// keeps short-lived tokens (GitHub installation tokens last one hour) valid in a
	// long-lived process such as the MCP server or a long --all run.
	refreshInterval = 30 * time.Minute
	// The retryBackoff duration is how long a failed run is reused before retrying, so a
	// failing broker is not re-run (and re-warned) on every remote read.
	retryBackoff = 30 * time.Second
	// The now function is the clock, replaceable in tests.
	now = time.Now

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

// brokerRun records when the brokers last ran and whether every enabled broker succeeded.
type brokerRun struct {
	at time.Time
	ok bool
}

// due reports whether the brokers should run again: never run, a successful run older
// than refreshInterval, or a failed run older than retryBackoff.
func (r brokerRun) due() bool {
	if r.at.IsZero() {
		return true
	}
	wait := refreshInterval
	if !r.ok {
		wait = retryBackoff
	}
	return now().Sub(r.at) >= wait
}

// EnsureCredentials runs every enabled broker and exports each broker's contributed
// environment variables into the current process via os.Setenv, so that Atmos's own
// go-getter git subprocesses and any downstream terraform/helmfile/packer subprocesses
// (whose environments start from os.Environ()) transparently pick them up.
//
// Repeated calls within refreshInterval of a successful run do not re-provision. A failed
// run is retried after retryBackoff, and a run interrupted by the caller's cancellation is
// not recorded, so one cancelled caller never disables brokering for the whole process.
//
// It is best-effort: a broker that is not enabled is skipped, and a broker that errors is
// logged and skipped — the in-progress remote read then proceeds and fails naturally if
// the credentials were truly required.
func EnsureCredentials(ctx context.Context, atmosConfig *schema.AtmosConfiguration) {
	defer perf.Track(atmosConfig, "auth.broker.EnsureCredentials")()

	ensureMu.Lock()
	defer ensureMu.Unlock()
	if ctx.Err() != nil || !ensureState.due() {
		return
	}
	ok := runEnabledBrokers(ctx, atmosConfig)
	if !ok && ctx.Err() != nil {
		// Cancelled mid-run: leave the state untouched so the next caller retries.
		return
	}
	ensureState = brokerRun{at: now(), ok: ok}
}

// runEnabledBrokers runs every enabled broker once and exports its env via os.Setenv. It
// reports whether every enabled broker succeeded. Split out from EnsureCredentials (which
// serializes and schedules it) so it can be tested directly.
func runEnabledBrokers(ctx context.Context, atmosConfig *schema.AtmosConfiguration) bool {
	ok := true
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
			ok = false
			continue
		}

		// Mark brokered only after at least one variable is actually exported: if every
		// os.Setenv failed there are no usable credentials in the process env, and enabling
		// downstream auth-retry would be wrong.
		exported := false
		for key, value := range env {
			if err := os.Setenv(key, value); err != nil {
				log.Warn("Failed to export credential broker variable", "broker", p.Name(), "key", key, "error", err)
				continue
			}
			exported = true
		}
		if exported {
			markBrokered()
		}
	}
	return ok
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

// SwapRegistryForTest clears the broker registry and the run state, returning a restore function
// that puts the previous state back when invoked. Intended for use in tests (including tests in
// other packages, such as pkg/stack/imports) that exercise broker-triggering code paths and need to
// register a fake provider in isolation. Because EnsureCredentials reuses a successful run for
// refreshInterval, resetting the run state is required for a test to observe a freshly registered
// broker. Production code must never call this.
func SwapRegistryForTest() func() {
	registryMu.Lock()
	prev := registry
	registry = nil
	registryMu.Unlock()
	resetEnsureStateForTest()
	resetBrokeredForTest()

	return func() {
		registryMu.Lock()
		registry = prev
		registryMu.Unlock()
		resetEnsureStateForTest()
		resetBrokeredForTest()
	}
}

// resetEnsureStateForTest forgets the last broker run so the next EnsureCredentials runs.
func resetEnsureStateForTest() {
	ensureMu.Lock()
	ensureState = brokerRun{}
	ensureMu.Unlock()
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
