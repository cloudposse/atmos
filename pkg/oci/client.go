package oci

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"

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
func (c *Client) Head(ctx context.Context, ref name.Reference) (*v1.Descriptor, error) {
	defer perf.Track(c.config, "oci.Client.Head")()

	options, err := c.options(ctx, ref.Context())
	if err != nil {
		return nil, err
	}
	result, err := remote.Head(ref, options...)
	if err != nil {
		return nil, errors.Join(errUtils.ErrPullImage, err)
	}
	return result, nil
}

// Tags lists repository tags without interpreting them as versions.
func (c *Client) Tags(ctx context.Context, repo name.Repository) ([]string, error) {
	defer perf.Track(c.config, "oci.Client.Tags")()

	options, err := c.options(ctx, repo)
	if err != nil {
		return nil, err
	}
	result, err := remote.List(repo, options...)
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCIRegistry, err)
	}
	return result, nil
}

// WriteImage uploads an image and its blobs. Rejected writes never retry anonymously.
func (c *Client) WriteImage(ctx context.Context, ref name.Reference, img v1.Image) error {
	defer perf.Track(c.config, "oci.Client.WriteImage")()

	options, err := c.options(ctx, ref.Context())
	if err != nil {
		return err
	}
	if err := remote.Write(ref, img, options...); err != nil {
		return errors.Join(errUtils.ErrOCIRegistry, err)
	}
	return nil
}

// WriteIndex uploads the complete index and its children without platform filtering.
func (c *Client) WriteIndex(ctx context.Context, ref name.Reference, index v1.ImageIndex) error {
	defer perf.Track(c.config, "oci.Client.WriteIndex")()

	options, err := c.options(ctx, ref.Context())
	if err != nil {
		return err
	}
	if err := remote.WriteIndex(ref, index, options...); err != nil {
		return errors.Join(errUtils.ErrOCIRegistry, err)
	}
	return nil
}

// Delete deletes only an explicitly selected digest, never a mutable tag.
func (c *Client) Delete(ctx context.Context, ref name.Digest) error {
	defer perf.Track(c.config, "oci.Client.Delete")()

	options, err := c.options(ctx, ref.Context())
	if err != nil {
		return err
	}
	if err := remote.Delete(ref, options...); err != nil {
		return errors.Join(errUtils.ErrOCIRegistry, err)
	}
	return nil
}
