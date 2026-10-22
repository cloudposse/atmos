// Package oci implements the "oci-tags" and "docker-tags" datasource
// resolvers backed by go-containerregistry: tag listing for version discovery
// and manifest digests for immutable pinning.
package oci

import (
	"context"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/resolver"
)

// Datasource names served by this resolver.
const (
	DatasourceOCI    = "oci-tags"
	DatasourceDocker = "docker-tags"
)

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
	tags, err := versionClient(req.Config, repo).Tags(ctx, repo)
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
	descriptor, err := versionClient(req.Config, repo).Head(ctx, repo.Tag(version))
	if err != nil {
		return "", err
	}
	return descriptor.Digest.String(), nil
}

// Version discovery historically prefers an available Atmos GHCR token to Docker credentials.
// Keep that read-only policy local; shared clients retain Docker-first authentication.
func versionClient(config *schema.AtmosConfiguration, repo name.Repository) *atmosoci.Client {
	if strings.EqualFold(repo.RegistryStr(), "ghcr.io") {
		if auth, _ := atmosoci.GHCRAuth(config); auth != nil {
			return atmosoci.NewClient(config, atmosoci.WithKeychain(nil))
		}
	}
	return atmosoci.NewClient(config)
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

func init() {
	resolver.Register(Resolver{})
}
