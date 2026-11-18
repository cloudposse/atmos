package s3

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// A reference owns the exact keys its payload consists of, not the whole location. An object a user
// added below an owned location is never overwritten by a later publication that writes that key;
// the reservation refuses before the journal or any payload object is written.
func TestReservation_RejectsAnUnmanagedKeyBelowAnOwnedLocation(t *testing.T) {
	const (
		perFileKey = "team/app/1.0.0/two.txt"
		sidecarKey = "team/app/1.0.0.tgz.docs/README.md"
	)
	tests := []struct {
		name      string
		spec      *rspec
		baseFiles map[string]string
		userKey   string
		replace   map[string]string
		wantKey   string
	}{
		{
			name: "per-file payload key", spec: noneSpec(), userKey: perFileKey,
			baseFiles: map[string]string{"one.txt": "1"}, replace: map[string]string{"one.txt": "1b", "two.txt": "2"}, wantKey: perFileKey,
		},
		{
			name: "per-file distribution document", spec: noneSpec(), userKey: "team/app/1.0.0/.atmos/artifact/README.md",
			baseFiles: map[string]string{"one.txt": "1"}, replace: map[string]string{"one.txt": "1b", ".atmos/artifact/README.md": "doc"},
			wantKey: "team/app/1.0.0/.atmos/artifact/README.md",
		},
		{
			name: "archive document sidecar", spec: archiveSpec(artifact.PackagingTgz), userKey: sidecarKey,
			baseFiles: map[string]string{"one.txt": "1"}, replace: map[string]string{"one.txt": "1b", ".atmos/artifact/README.md": "doc"}, wantKey: sidecarKey,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.spec.fresh(t)
			repo := newRepo(t, spec)
			base := publish(t, repo, &prepareSpec{files: tt.baseFiles}, artifact.PublishOptions{})
			spec.fake.Seed(tt.userKey, []byte("mine"), nil)
			before := bucketKeys(spec.fake)

			_, err := repo.Publish(context.Background(), prepare(t, repo, &prepareSpec{files: tt.replace, opID: "op-replace"}),
				artifact.PublishOptions{Force: true, ExpectedPrior: &base.Pinned})
			require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
			assert.ErrorContains(t, err, tt.wantKey)

			object, ok := spec.fake.Object(tt.userKey)
			require.True(t, ok)
			assert.Equal(t, []byte("mine"), object.Data, "the user's object was not overwritten")
			assert.Equal(t, before, bucketKeys(spec.fake), "nothing was written: no journal, no pending reference, no payload")
			assertIntact(t, repo, defaultRef())
		})
	}
}

// The reference's own keys stay exempt, and an extra user object the publication does not write is
// left alone: only a key the publication would write can conflict.
func TestReservation_AllowsOwnedKeysAndLeavesUnrelatedUserObjects(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	base := publish(t, repo, &prepareSpec{files: map[string]string{"one.txt": "1", "gone.txt": "g"}}, artifact.PublishOptions{})
	spec.fake.Seed("team/app/1.0.0/user.txt", []byte("mine"), nil)

	replaced, err := repo.Publish(context.Background(), prepare(t, repo, &prepareSpec{files: map[string]string{"one.txt": "1b"}, opID: "op-replace"}),
		artifact.PublishOptions{Force: true, ExpectedPrior: &base.Pinned})
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeReplaced, replaced.Outcome)
	assertIntact(t, repo, defaultRef())
	keys := payloadKeys(spec.fake, "")
	assert.Contains(t, keys, "team/app/1.0.0/user.txt", "an unmanaged object the publication does not write stays")
	assert.NotContains(t, keys, "team/app/1.0.0/gone.txt", "the replaced payload's own key was retired")
	object, _ := spec.fake.Object("team/app/1.0.0/user.txt")
	assert.Equal(t, []byte("mine"), object.Data)
}

// A resumed publication owns the keys its interrupted attempt already wrote, so the retry is not
// refused by its own partial upload, while a user's key it did not write still is.
func TestReservation_ResumeOwnsItsOwnPartialUpload(t *testing.T) {
	var fault armedFault
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec.withOpts(fault.option()))
	prepared := prepare(t, repo, &prepareSpec{files: map[string]string{"one.txt": "1", "two.txt": "2"}, opID: "op-first"})

	fault.hook = crashAt(faultDuringPayload)
	_, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
	require.ErrorIs(t, err, errCrash)
	require.NotEmpty(t, payloadKeys(spec.fake, ""), "the interrupted attempt wrote part of the payload")
	fault.hook = nil

	result, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
	require.NoError(t, err, "the retry owns the keys it wrote")
	assert.Equal(t, artifact.OutcomeCreated, result.Outcome)
	assertIntact(t, repo, defaultRef())
}

// A location no reference owns is still refused whole, whichever key the user's object has.
func TestReservation_RejectsAnyObjectAtAnUnownedLocation(t *testing.T) {
	spec := noneSpec().fresh(t)
	repo := newRepo(t, spec)
	spec.fake.Seed("team/app/1.0.0/unrelated.txt", []byte("mine"), nil)

	_, err := repo.Publish(context.Background(), prepare(t, repo, &prepareSpec{files: map[string]string{"one.txt": "1"}}), artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	assert.ErrorContains(t, err, "not managed by this repository")
	assert.Equal(t, []string{"team/app/1.0.0/unrelated.txt"}, payloadKeys(spec.fake, ""))
}
