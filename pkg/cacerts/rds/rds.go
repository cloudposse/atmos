package rds

import (
	_ "embed"

	"github.com/cloudposse/atmos/pkg/perf"
)

// globalBundle is Amazon's global RDS/Aurora CA certificate trust bundle. See doc.go for
// provenance (source URL, fetch date) and staleness behavior.
//
//go:embed global-bundle.pem
var globalBundle []byte

// Bundle returns the embedded Amazon RDS global CA certificate trust bundle, PEM-encoded. Callers
// combine this with the host's system CA store (see pkg/cacerts.BuildBundle) rather than using it
// standalone, so non-RDS TLS verification (e.g. a component's own CA) keeps working too.
func Bundle() []byte {
	defer perf.Track(nil, "rds.Bundle")()

	return globalBundle
}
