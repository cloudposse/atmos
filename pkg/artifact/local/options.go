package local

import (
	"fmt"
	"os"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// settings holds the construction options of a local repository.
type settings struct {
	verifier artifact.Verifier
	owner    string
	faults   faultHook
}

// Option configures a local repository or its registration.
type Option func(*settings)

// WithVerifier injects the cryptographic verifier used by Verify. The formats it understands are
// the evidence formats the repository advertises. Without a verifier the repository stores no
// evidence and Sign and Verify return unsupported-capability errors.
func WithVerifier(verifier artifact.Verifier) Option {
	defer perf.Track(nil, "local.WithVerifier")()

	return func(s *settings) { s.verifier = verifier }
}

// WithOwner sets the owner recorded in lock requests and operation journals when the caller does
// not supply one. The default is "<hostname>:<pid>".
func WithOwner(owner string) Option {
	defer perf.Track(nil, "local.WithOwner")()

	return func(s *settings) { s.owner = owner }
}

func applySettings(opts []Option) settings {
	var applied settings
	for _, opt := range opts {
		if opt != nil {
			opt(&applied)
		}
	}
	if applied.owner == "" {
		applied.owner = defaultOwner()
	}
	return applied
}

func defaultOwner() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s:%d", host, os.Getpid())
}
