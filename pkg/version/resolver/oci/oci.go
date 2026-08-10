// Package oci implements the "oci-tags" and "docker-tags" datasource
// resolvers backed by go-containerregistry: tag listing for version discovery
// and manifest digests for immutable pinning.
package oci

import (
	"context"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1remote "github.com/google/go-containerregistry/pkg/v1/remote"

	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/resolver"
)

// Datasource names served by this resolver.
const (
	DatasourceOCI    = "oci-tags"
	DatasourceDocker = "docker-tags"
)

// ghcrHost is the GitHub Container Registry host, which gets token-based auth
// from Atmos settings when the default keychain has no credentials.
const ghcrHost = "ghcr.io"

// Resolver resolves OCI image tags and digests.
type Resolver struct{}

// Names returns the datasource names this resolver serves.
func (Resolver) Names() []string {
	defer perf.Track(nil, "oci.Resolver.Names")()

	return []string{DatasourceOCI, DatasourceDocker}
}

// Versions lists the repository's tags as candidates. OCI registries provide
// neither timestamps nor digests in tag listings; digests resolve via Pin.
func (Resolver) Versions(ctx context.Context, req *resolver.Request) ([]resolver.Candidate, error) {
	defer perf.Track(nil, "oci.Resolver.Versions")()

	repo, err := repository(req)
	if err != nil {
		return nil, err
	}
	tags, err := v1remote.List(repo, remoteOptions(ctx, req, repo)...)
	if err != nil {
		return nil, err
	}
	candidates := make([]resolver.Candidate, 0, len(tags))
	for _, tag := range tags {
		candidates = append(candidates, resolver.Candidate{Version: tag})
	}
	return candidates, nil
}

// Pin resolves a tag to its immutable manifest digest (sha256:...).
func (Resolver) Pin(ctx context.Context, req *resolver.Request, version string) (string, error) {
	defer perf.Track(nil, "oci.Resolver.Pin")()

	repo, err := repository(req)
	if err != nil {
		return "", err
	}
	descriptor, err := v1remote.Head(repo.Tag(version), remoteOptions(ctx, req, repo)...)
	if err != nil {
		return "", err
	}
	return descriptor.Digest.String(), nil
}

// repository builds the repository reference for a request. The package may
// carry its own registry host (e.g. ghcr.io/acme/app); otherwise the
// provider's URL is prepended, and with neither the Docker Hub default
// applies (docker-tags: `library/nginx` -> index.docker.io/library/nginx).
func repository(req *resolver.Request) (name.Repository, error) {
	repoRef := req.Package
	if req.Provider.URL != "" && !hasRegistryHost(repoRef) {
		repoRef = strings.TrimSuffix(req.Provider.URL, "/") + "/" + repoRef
	}
	opts := []name.Option{}
	if req.Provider.Insecure {
		opts = append(opts, name.Insecure)
	}
	return name.NewRepository(repoRef, opts...)
}

// hasRegistryHost reports whether the first path segment looks like a
// registry host (contains a dot or port), following the Docker reference
// convention.
func hasRegistryHost(repoRef string) bool {
	first := strings.SplitN(repoRef, "/", 2)[0]
	return strings.ContainsAny(first, ".:")
}

// remoteOptions assembles remote call options: request context plus
// authentication from the default keychain (docker login, ambient cloud
// credential helpers), with a GHCR token fallback from Atmos settings.
func remoteOptions(ctx context.Context, req *resolver.Request, repo name.Repository) []v1remote.Option {
	opts := []v1remote.Option{v1remote.WithContext(ctx)}
	if repo.RegistryStr() == ghcrHost {
		if auth, _ := atmosoci.GHCRAuth(req.Config); auth != nil {
			return append(opts, v1remote.WithAuth(auth))
		}
	}
	return append(opts, v1remote.WithAuthFromKeychain(authn.DefaultKeychain))
}

func init() {
	resolver.Register(Resolver{})
}
