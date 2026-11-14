package oci

import (
	"context"
	"os"
	"strconv"

	"github.com/cloudposse/atmos/pkg/artifact"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TransportFactory builds the registry transport of one configured repository. The resolve
// argument is the composition root's identity resolver; an implementation binds the repository's identity to the
// transport it returns, so the credentials of one repository are never presented for another.
// Credentials are resolved when a repository is opened for an operation, not here.
type TransportFactory func(ctx context.Context, alias string, cfg *schema.ArtifactRepositoryConfig, resolve artifact.IdentityResolver) (atmosoci.Transport, error)

// settings holds the construction options of an OCI repository or its registration.
type settings struct {
	transport   atmosoci.Transport
	factory     TransportFactory
	verifier    artifact.Verifier
	owner       string
	lockDir     string
	noCatalog   bool
	concurrency int
	pageSize    int
}

// Option configures an OCI repository or its registration.
type Option func(*settings)

// WithTransport supplies the registry transport used by every oci/registry repository the
// registered factory constructs. Tests inject an in-process registry client; a transport that
// also implements [atmosoci.RegistryCatalog] enables collection enumeration.
func WithTransport(transport atmosoci.Transport) Option {
	defer perf.Track(nil, "oci.WithTransport")()

	return func(s *settings) { s.transport = transport }
}

// WithTransportFactory supplies a per-repository transport factory that binds the repository's
// identity. It takes precedence over [WithTransport]. A repository that configures an identity
// requires a factory, so an identity is never silently ignored.
func WithTransportFactory(factory TransportFactory) Option {
	defer perf.Track(nil, "oci.WithTransportFactory")()

	return func(s *settings) { s.factory = factory }
}

// WithVerifier injects the cryptographic verifier used by Verify. The formats it understands are
// the evidence formats the repository advertises. Without a verifier the repository stores no
// evidence, and Sign and Verify return unsupported-capability errors.
func WithVerifier(verifier artifact.Verifier) Option {
	defer perf.Track(nil, "oci.WithVerifier")()

	return func(s *settings) { s.verifier = verifier }
}

// WithOwner sets the owner recorded in lock requests when the caller does not supply one. The
// default is "<hostname>:<pid>".
func WithOwner(owner string) Option {
	defer perf.Track(nil, "oci.WithOwner")()

	return func(s *settings) { s.owner = owner }
}

// WithLockDir sets the directory holding the host-scope write guards. The default is a directory
// below the user's data directory, so the guard is shared by every process on the host.
func WithLockDir(dir string) Option {
	defer perf.Track(nil, "oci.WithLockDir")()

	return func(s *settings) { s.lockDir = dir }
}

// WithoutCollectionEnumeration stops an oci/registry repository from advertising or attempting
// registry-wide repository enumeration, for registries known not to support it.
func WithoutCollectionEnumeration() Option {
	defer perf.Track(nil, "oci.WithoutCollectionEnumeration")()

	return func(s *settings) { s.noCatalog = true }
}

// WithConcurrency bounds how many manifests listing inspects at once. The default is
// [artifact.DefaultConcurrency].
func WithConcurrency(n int) Option {
	defer perf.Track(nil, "oci.WithConcurrency")()

	return func(s *settings) { s.concurrency = n }
}

// WithPageSize sets the list page size used when a query does not choose one.
func WithPageSize(n int) Option {
	defer perf.Track(nil, "oci.WithPageSize")()

	return func(s *settings) { s.pageSize = n }
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
	if applied.concurrency < 1 {
		applied.concurrency = artifact.DefaultConcurrency
	}
	if applied.pageSize < 1 {
		applied.pageSize = defaultPageSize
	}
	return applied
}

func defaultOwner() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	return host + ":" + strconv.Itoa(os.Getpid())
}
