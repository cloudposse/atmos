package oci

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestParseDestination(t *testing.T) {
	valid := map[string]string{
		"oci://ghcr.io/acme/plugin":        "ghcr.io/acme/plugin",
		"ghcr.io/acme/plugin":              "ghcr.io/acme/plugin",
		"oci://localhost:5000/acme/plugin": "localhost:5000/acme/plugin",
	}
	for input, want := range valid {
		repository, err := ParseDestination(input)
		require.NoError(t, err, input)
		require.Equal(t, want, repository.Name())
	}
	invalid := map[string]string{
		"oci://ghcr.io/acme/plugin:1.0.0":                             "includes a tag or digest",
		"ghcr.io/acme/plugin:latest":                                  "includes a tag or digest",
		"oci://ghcr.io/acme/plugin@sha256:" + strings.Repeat("a", 64): "includes a tag or digest",
		"bad repository":                                              "is not an OCI repository",
		"":                                                            "is not an OCI repository",
		"oci://ghcr.io/ACME/Plugin":                                   "is not an OCI repository",
	}
	for input, message := range invalid {
		_, err := ParseDestination(input)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig, input)
		require.Contains(t, err.Error(), message)
		require.NotContains(t, err.Error(), "could not parse", "raw registry-library text must not leak")
		require.NotEmpty(t, errUtils.AllHints(err))
	}
}

func TestParseDigestReference(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	reference, err := ParseDigestReference("oci://ghcr.io/acme/plugin@" + digest)
	require.NoError(t, err)
	require.Equal(t, digest, reference.DigestStr())
	for _, input := range []string{"ghcr.io/acme/plugin:tag", "ghcr.io/acme/plugin", "ghcr.io/acme/plugin@sha256:short", ""} {
		_, err := ParseDigestReference(input)
		require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig, input)
		require.Contains(t, strings.Join(errUtils.AllHints(err), " "), "sha256")
	}
}

func TestDescribeDeletionListsOnlyTagsPointingAtTheDigest(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	other := "sha256:" + strings.Repeat("b", 64)
	reference, err := name.NewDigest("ghcr.io/acme/plugin@"+digest, name.StrictValidation)
	require.NoError(t, err)
	hash := func(value string) v1.Hash {
		parsed, err := v1.NewHash(value)
		require.NoError(t, err)
		return parsed
	}
	client := NewMockRegistry(gomock.NewController(t))
	client.EXPECT().Tags(gomock.Any(), reference.Context()).Return([]string{"2.0.0", "1.0.0", "stable", "missing"}, nil)
	client.EXPECT().Head(gomock.Any(), reference.Context().Tag("1.0.0")).Return(&v1.Descriptor{Digest: hash(digest)}, nil)
	client.EXPECT().Head(gomock.Any(), reference.Context().Tag("2.0.0")).Return(&v1.Descriptor{Digest: hash(other)}, nil)
	client.EXPECT().Head(gomock.Any(), reference.Context().Tag("stable")).Return(&v1.Descriptor{Digest: hash(digest)}, nil)
	client.EXPECT().Head(gomock.Any(), reference.Context().Tag("missing")).Return(nil, errors.New("not found"))
	target := DescribeDeletion(t.Context(), client, reference)
	require.Equal(t, DeletionTarget{Repository: "ghcr.io/acme/plugin", Digest: digest, Tags: []string{"1.0.0", "stable"}}, target)

	failing := NewMockRegistry(gomock.NewController(t))
	failing.EXPECT().Tags(gomock.Any(), reference.Context()).Return(nil, errors.New("listing unsupported"))
	require.Equal(t, DeletionTarget{Repository: "ghcr.io/acme/plugin", Digest: digest}, DescribeDeletion(t.Context(), failing, reference), "tag listing is best effort")
}
