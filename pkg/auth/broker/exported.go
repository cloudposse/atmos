package broker

import (
	"os"
	"runtime"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// envSeparator separates a variable name from its value in a "KEY=value" entry.
const envSeparator = "="

// priorVariable is the state a process environment variable had before a broker replaced it.
type priorVariable struct {
	value string
	set   bool
}

// The exportedEnv map records every variable a broker exported into the process environment this
// process, keyed by its normalized name, with the state it had before. Guarded by brokeredMu.
//
// A remote read that names an Atmos Auth identity uses that identity's credentials, never the
// ambient broker's. The broker exports into the process environment, which every later subprocess
// inherits, so such a read must be able to see the environment as it was before the broker ran.
var exportedEnv = map[string]priorVariable{}

// recordExport remembers the state of key before a broker replaces it. Only the first export of a
// variable is recorded, so the remembered state is the one from before any broker ran.
func recordExport(key string) {
	normalized := normalizeEnvKey(key)

	brokeredMu.Lock()
	defer brokeredMu.Unlock()
	if _, recorded := exportedEnv[normalized]; recorded {
		return
	}
	// A snapshot of the live process variable a broker is about to replace.
	value, set := os.LookupEnv(key)
	exportedEnv[normalized] = priorVariable{value: value, set: set}
}

// forgetExport drops the record for a variable whose export failed, so it is not treated as
// broker-owned.
func forgetExport(key string) {
	brokeredMu.Lock()
	defer brokeredMu.Unlock()
	delete(exportedEnv, normalizeEnvKey(key))
}

// PreBrokerValue reports the value a variable had before a broker exported it. The second result
// is false when no broker exported the variable, in which case the process environment is
// authoritative. A variable that was unset before the broker ran reports "" and true.
func PreBrokerValue(name string) (string, bool) {
	defer perf.Track(nil, "auth.broker.PreBrokerValue")()

	brokeredMu.Lock()
	defer brokeredMu.Unlock()
	prior, exported := exportedEnv[normalizeEnvKey(name)]
	return prior.value, exported
}

// WithoutBrokeredEnvironment returns env ("KEY=value" entries) with every broker-exported variable
// restored to the state it had before the broker ran: removed when it was unset, or set back to
// its earlier value. Variables no broker exported are returned unchanged, and env is not modified.
func WithoutBrokeredEnvironment(env []string) []string {
	defer perf.Track(nil, "auth.broker.WithoutBrokeredEnvironment")()

	brokeredMu.Lock()
	defer brokeredMu.Unlock()
	if len(exportedEnv) == 0 {
		return env
	}

	restored := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, found := strings.Cut(entry, envSeparator)
		if !found {
			restored = append(restored, entry)
			continue
		}
		prior, exported := exportedEnv[normalizeEnvKey(key)]
		switch {
		case !exported:
			restored = append(restored, entry)
		case prior.set:
			restored = append(restored, key+envSeparator+prior.value)
		}
	}
	return restored
}

// normalizeEnvKey folds case on Windows, where environment names are case-insensitive.
func normalizeEnvKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(key)
	}
	return key
}
