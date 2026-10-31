// Package sign signs immutable plugin releases and publishes Sigstore evidence.
package sign

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	pluginpolicy "github.com/cloudposse/atmos/pkg/plugin/policy"
	sigcore "github.com/cloudposse/atmos/pkg/signature"
)

// BlobSigner produces a standard Sigstore bundle for the exact supplied bytes.
//
//go:generate go run go.uber.org/mock/mockgen -destination=mock_signer_test.go -package=sign . BlobSigner
type BlobSigner interface {
	Sign(context.Context, []byte) ([]byte, error)
}

// Service separates signing from registry credentials and release validation.
type Service struct {
	Registry    oci.Registry
	Signer      BlobSigner
	Descriptors *extension.Registry
}

// Release validates a plugin index and attaches a signature to its immutable digest.
// Tags are intentionally rejected so a concurrent publisher cannot change the subject.
func (s Service) Release(ctx context.Context, reference string) (*pluginoci.Release, error) {
	defer perf.Track(nil, "pluginsign.Service.Release")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ref, err := name.NewDigest(strings.TrimPrefix(reference, "oci://"), name.StrictValidation)
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	if err := pluginpolicy.Source(ctx, "sign", ref.Context().Name()); err != nil {
		return nil, err
	}
	descriptor, err := s.Registry.Get(ctx, ref)
	if err != nil {
		return nil, err
	}
	index, err := descriptor.ImageIndex()
	if err != nil {
		return nil, err
	}
	release, err := pluginoci.Inspect(index, ref.DigestStr())
	if err != nil {
		return nil, err
	}
	if err := pluginpolicy.Package(ctx, "sign", ref.Context().Name(), release.Definition, pluginpolicy.WithDigest(release.Digest), pluginpolicy.WithExtensions(s.Descriptors)); err != nil {
		return nil, err
	}
	if err := s.signIndex(ctx, ref, &descriptor.Descriptor, index); err != nil {
		return nil, err
	}
	return release, nil
}

func (s Service) signIndex(ctx context.Context, ref name.Digest, descriptor *v1.Descriptor, index v1.ImageIndex) error {
	raw, err := index.RawManifest()
	if err != nil {
		return err
	}
	signed, err := s.Signer.Sign(ctx, raw)
	if err != nil {
		return err
	}
	if err := validateBundle(signed, raw); err != nil {
		return err
	}
	if err := s.attach(ctx, ref, descriptor, signed); err != nil {
		return err
	}
	return nil
}

func validateBundle(raw, artifact []byte) error {
	err := sigcore.CheckBundleSubject(raw, artifact)
	if err == nil {
		return nil
	}
	var shared *sigcore.Error
	if errors.As(err, &shared) && shared.Cause != nil {
		return errors.Join(errUtils.ErrPluginSignatureInvalid, shared.Cause)
	}
	return errUtils.ErrPluginSignatureInvalid
}

func (s Service) attach(ctx context.Context, ref name.Digest, subject *v1.Descriptor, raw []byte) error {
	var metadata struct {
		MediaType string `json:"mediaType"`
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return errors.Join(errUtils.ErrPluginSignatureInvalid, err)
	}
	image, err := mutate.AppendLayers(empty.Image, static.NewLayer(raw, types.MediaType("application/json")))
	if err != nil {
		return err
	}
	image = mutate.MediaType(image, types.OCIManifestSchema1)
	image = mutate.ConfigMediaType(image, types.MediaType(metadata.MediaType))
	image = mutate.Subject(image, *subject).(v1.Image)
	digest, err := image.Digest()
	if err != nil {
		return err
	}
	return s.Registry.WriteImage(ctx, ref.Context().Digest(digest.String()), image)
}
