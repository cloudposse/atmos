package policy_test

import (
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginpolicy "github.com/cloudposse/atmos/pkg/plugin/policy"
)

//go:generate go run go.uber.org/mock/mockgen -destination mock_registry_test.go -package policy_test github.com/cloudposse/atmos/pkg/oci Registry,ReferrerRegistry

func TestRegistryPolicyPrecedesAllTransportOperations(t *testing.T) {
	governingPolicy(t, `input.registry != 'registry.example.com'`)
	next := NewMockReferrerRegistry(gomock.NewController(t))
	registry := pluginpolicy.Registry{Next: next}
	denied := name.MustParseReference("public.example.com/acme/auth:1.0.0")
	deniedDigest := denied.Context().Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	_, err := registry.Get(t.Context(), denied)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	_, err = registry.Head(t.Context(), denied)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	_, err = registry.Tags(t.Context(), denied.Context())
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	require.ErrorIs(t, registry.WriteImage(t.Context(), denied, empty.Image), errUtils.ErrPolicyDenied)
	require.ErrorIs(t, registry.WriteIndex(t.Context(), denied, empty.Index), errUtils.ErrPolicyDenied)
	require.ErrorIs(t, registry.Delete(t.Context(), deniedDigest), errUtils.ErrPolicyDenied)
	_, err = registry.Referrers(t.Context(), deniedDigest)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	// No expectations are registered: denied operations must never reach transport.
}

func TestAllowedRegistryPreservesTransportAndSignatureDiscovery(t *testing.T) {
	governingPolicy(t, `input.registry != 'registry.example.com'`)
	next := NewMockReferrerRegistry(gomock.NewController(t))
	registry := pluginpolicy.Registry{Next: next}
	ref := name.MustParseReference("registry.example.com/acme/auth:1.0.0")
	digest := ref.Context().Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	descriptor := &remote.Descriptor{}
	head := &v1.Descriptor{}
	next.EXPECT().Get(gomock.Any(), ref).Return(descriptor, nil)
	next.EXPECT().Head(gomock.Any(), ref).Return(head, nil)
	next.EXPECT().Tags(gomock.Any(), ref.Context()).Return([]string{"1.0.0"}, nil)
	next.EXPECT().WriteImage(gomock.Any(), ref, empty.Image).Return(nil)
	next.EXPECT().WriteIndex(gomock.Any(), ref, empty.Index).Return(nil)
	next.EXPECT().Delete(gomock.Any(), digest).Return(nil)
	next.EXPECT().Referrers(gomock.Any(), digest).Return(empty.Index, nil)
	actual, err := registry.Get(t.Context(), ref)
	require.NoError(t, err)
	require.Same(t, descriptor, actual)
	actualHead, err := registry.Head(t.Context(), ref)
	require.NoError(t, err)
	require.Same(t, head, actualHead)
	tags, err := registry.Tags(t.Context(), ref.Context())
	require.NoError(t, err)
	require.Equal(t, []string{"1.0.0"}, tags)
	require.NoError(t, registry.WriteImage(t.Context(), ref, empty.Image))
	require.NoError(t, registry.WriteIndex(t.Context(), ref, empty.Index))
	require.NoError(t, registry.Delete(t.Context(), digest))
	index, err := registry.Referrers(t.Context(), digest)
	require.NoError(t, err)
	require.Equal(t, empty.Index, index)
	registry.Next = NewMockRegistry(gomock.NewController(t))
	_, err = registry.Referrers(t.Context(), digest)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
}
