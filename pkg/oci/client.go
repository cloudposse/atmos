package oci

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Registry is the transport boundary shared by OCI consumers. Images and indexes
// retain their original bytes; package-specific extraction lives above it.
// A client resolves credentials independently for every repository operation.
type Registry interface {
	Get(context.Context, name.Reference) (*remote.Descriptor, error)
	Head(context.Context, name.Reference) (*v1.Descriptor, error)
	Tags(context.Context, name.Repository) ([]string, error)
	WriteImage(context.Context, name.Reference, v1.Image) error
	WriteIndex(context.Context, name.Reference, v1.ImageIndex) error
	Delete(context.Context, name.Digest) error
}

// Client performs registry operations without plugin or component semantics.
type Client struct {
	config    *schema.AtmosConfiguration
	keychain  authn.Keychain
	transport http.RoundTripper
	fallback  authn.Keychain
	identity  AuthenticationResolver
}

// ClientOption configures authentication or transport for one client.
type ClientOption func(*Client)

// WithKeychain sets a repository-aware credential resolver. Callers transferring
// between registries can supply separate source and destination clients.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func WithKeychain(keychain authn.Keychain) ClientOption {
	return func(client *Client) { client.keychain = keychain }
}

// WithCredentialFallback supplies credentials after Docker helpers and GHCR settings.
// It does not retry failed authenticated registry requests with another identity.
//
//nolint:lintroller // Functional option only.
func WithCredentialFallback(chain authn.Keychain) ClientOption {
	return func(client *Client) { client.fallback = chain }
}

// WithTransport supplies TLS, CA, proxy, and instrumentation settings.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func WithTransport(transport http.RoundTripper) ClientOption {
	return func(client *Client) { client.transport = transport }
}

// NewClient constructs a client using the existing Docker and GHCR auth policy.
func NewClient(config *schema.AtmosConfiguration, options ...ClientOption) *Client {
	defer perf.Track(nil, "oci.NewClient")()

	client := &Client{config: config, keychain: authn.DefaultKeychain}
	for _, option := range options {
		option(client)
	}
	return client
}

func (c *Client) options(ctx context.Context, resource authn.Resource) ([]remote.Option, error) {
	auth, err := c.authentication(ctx, resource)
	if err != nil {
		return nil, err
	}
	options := []remote.Option{remote.WithContext(ctx), remote.WithAuth(auth)}
	if c.transport != nil {
		options = append(options, remote.WithTransport(c.transport))
	}
	return options, nil
}

// Get resolves an index or image without selecting a platform or extracting it.
func (c *Client) Get(ctx context.Context, ref name.Reference) (*remote.Descriptor, error) {
	defer perf.Track(c.config, "oci.Client.Get")()

	options, err := c.options(ctx, ref.Context())
	if err != nil {
		return nil, err
	}
	result, err := remote.Get(ref, options...)
	if err != nil {
		return nil, errors.Join(errUtils.ErrPullImage, err)
	}
	return result, nil
}

// Head resolves the immutable descriptor without downloading artifact layers.
// It runs on the shared ORAS transport.
func (c *Client) Head(ctx context.Context, ref name.Reference) (*v1.Descriptor, error) {
	defer perf.Track(c.config, "oci.Client.Head")()

	store, err := c.Repository(ctx, repositoryOf(ref.Context()))
	if err != nil {
		return nil, err
	}
	resolved, err := store.Resolve(ctx, ref.Identifier())
	if err != nil {
		return nil, errors.Join(errUtils.ErrPullImage, err)
	}
	hash, err := v1.NewHash(resolved.Digest.String())
	if err != nil {
		return nil, errors.Join(errUtils.ErrPullImage, err)
	}
	return &v1.Descriptor{MediaType: types.MediaType(resolved.MediaType), Size: resolved.Size, Digest: hash}, nil
}

// Tags lists repository tags without interpreting them as versions.
func (c *Client) Tags(ctx context.Context, repo name.Repository) ([]string, error) {
	defer perf.Track(c.config, "oci.Client.Tags")()

	store, err := c.Repository(ctx, repositoryOf(repo))
	if err != nil {
		return nil, err
	}
	return store.Tags(ctx)
}

// WriteImage uploads an image and its blobs, preserving the image's existing raw bytes.
// Rejected writes never retry anonymously, and an unknown outcome is reconciled by
// resolving the destination rather than writing again.
func (c *Client) WriteImage(ctx context.Context, ref name.Reference, img v1.Image) error {
	defer perf.Track(c.config, "oci.Client.WriteImage")()

	store, err := c.Repository(ctx, repositoryOf(ref.Context()))
	if err != nil {
		return err
	}
	graph, err := legacyImageGraph(img)
	if err != nil {
		return err
	}
	_, err = Copy(ctx, Source{Store: graph, Reference: legacyRoot}, Destination{Store: store, Reference: ref.Identifier()}, WithoutSubjectTraversal())
	return err
}

// WriteIndex uploads the complete index and its children without platform filtering.
func (c *Client) WriteIndex(ctx context.Context, ref name.Reference, index v1.ImageIndex) error {
	defer perf.Track(c.config, "oci.Client.WriteIndex")()

	store, err := c.Repository(ctx, repositoryOf(ref.Context()))
	if err != nil {
		return err
	}
	graph, err := legacyIndexGraph(index)
	if err != nil {
		return err
	}
	_, err = Copy(ctx, Source{Store: graph, Reference: legacyRoot}, Destination{Store: store, Reference: ref.Identifier()}, WithoutSubjectTraversal())
	return err
}

// Delete deletes only an explicitly selected digest, never a mutable tag.
func (c *Client) Delete(ctx context.Context, ref name.Digest) error {
	defer perf.Track(c.config, "oci.Client.Delete")()

	store, err := c.Repository(ctx, repositoryOf(ref.Context()))
	if err != nil {
		return err
	}
	target, err := store.Resolve(ctx, ref.DigestStr())
	if err != nil {
		return err
	}
	if err := store.Delete(ctx, target); err != nil {
		return c.reconcileDelete(ctx, store, ref, err)
	}
	return nil
}

// reconcileDelete treats a failed delete as complete only when the digest is gone.
func (c *Client) reconcileDelete(ctx context.Context, store RepositoryStore, ref name.Digest, cause error) error {
	if !ambiguousWrite(ctx, cause) {
		return cause
	}
	if _, err := store.Resolve(ctx, ref.DigestStr()); IsNotFound(err) {
		return nil
	}
	return cause
}
