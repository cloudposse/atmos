package oci

import (
	"context"
	"net/http"

	"github.com/google/go-containerregistry/pkg/name"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"

	"github.com/cloudposse/atmos/pkg/perf"
)

// RegistryCatalog lists the repositories a registry exposes through its repository catalog
// (GET /v2/_catalog). Many registries do not implement it; callers must treat a failure as
// "enumeration is unsupported" and never as an empty registry.
type RegistryCatalog interface {
	// Repositories lists every repository name the registry reports, in registry order.
	Repositories(ctx context.Context, registry string) ([]string, error)
}

// Repositories lists the repositories of a registry with credentials resolved up front, exactly
// like Repository. A rejected or unsupported catalog request is returned as an error.
func (c *Client) Repositories(ctx context.Context, registry string) ([]string, error) {
	defer perf.Track(c.config, "oci.Client.Repositories")()

	resource, err := name.NewRegistry(registry)
	if err != nil {
		return nil, wrapRegistry(err)
	}
	authenticator, err := c.authentication(ctx, resource)
	if err != nil {
		return nil, err
	}
	host := orasHost(resource.RegistryStr())
	remoteRegistry, err := remote.NewRegistry(host)
	if err != nil {
		return nil, wrapRegistry(err)
	}
	remoteRegistry.PlainHTTP = resource.Scheme() == "http"
	remoteRegistry.Client = &auth.Client{
		Client:     &http.Client{Transport: newRegistryTransport(c.transport)},
		Header:     http.Header{"User-Agent": {userAgent}},
		Cache:      auth.NewCache(),
		Credential: hostScopedCredential(host, authenticator),
	}
	var repositories []string
	err = remoteRegistry.Repositories(ctx, "", func(page []string) error {
		repositories = append(repositories, page...)
		return nil
	})
	if err != nil {
		return nil, wrapRegistry(err)
	}
	return repositories, nil
}

var _ RegistryCatalog = (*Client)(nil)
