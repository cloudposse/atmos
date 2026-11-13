package oci

import (
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// An unchanged tag still honors the compare-and-swap guard: a caller that expected the tag absent
// gets a conflict, while the matching expected prior reports unchanged.
func TestDecidePublishUnchangedHonorsExpectedPrior(t *testing.T) {
	ref, err := artifact.ParseRef("team/app:1.0.0")
	require.NoError(t, err)
	current := digest.FromString("current")
	state := &tagState{exists: true, desc: ocispec.Descriptor{Digest: current}}
	incoming := &incomingRoot{digest: current}

	_, err = decidePublish(ref, state, incoming, artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "an absent expected prior must not report unchanged")

	outcome, err := decidePublish(ref, state, incoming, artifact.PublishOptions{ExpectedPrior: &artifact.PinnedReference{ResolvedDigest: current}})
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeUnchanged, outcome)

	_, err = decidePublish(ref, state, incoming, artifact.PublishOptions{ExpectedPrior: &artifact.PinnedReference{ResolvedDigest: digest.FromString("stale")}})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "a stale expected prior must not report unchanged")
}

// Referrers that name another artifact type are skipped before they cost inspection budget.
func TestMayBeBundleFiltersByArtifactType(t *testing.T) {
	assert.True(t, mayBeBundle(&ocispec.Descriptor{}), "an untyped referrer must still be inspected")
	assert.True(t, mayBeBundle(&ocispec.Descriptor{ArtifactType: ocispec.MediaTypeEmptyJSON}), "a registry-derived empty config type is only a hint")
	assert.True(t, mayBeBundle(&ocispec.Descriptor{ArtifactType: bundleMediaTypePrefix + "v0.3+json"}))
	assert.False(t, mayBeBundle(&ocispec.Descriptor{ArtifactType: "application/vnd.example.sbom+json"}))
}

// A truncated scan is reported as incomplete rather than as missing evidence.
func TestTruncatedDetectsIncompleteEvidenceScans(t *testing.T) {
	assert.False(t, truncated(nil))
	assert.False(t, truncated([]artifact.Diagnostic{{Code: "malformed-evidence"}}))
	assert.True(t, truncated([]artifact.Diagnostic{{Code: "malformed-evidence"}, {Code: "evidence-truncated"}}))
}
