package s3

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	blobs3 "github.com/cloudposse/atmos/pkg/artifact/blob/s3"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
)

const testPrefix = "atmos/prod"

// bigFiles forces the multipart path (the test part size is 16 KiB) and includes an empty file.
func bigFiles() map[string]string {
	return map[string]string{
		"data.txt":       "hello\n",
		"empty.txt":      "",
		"nested/big.bin": incompressible(48 << 10),
	}
}

// The completed reference is the very last object written: the pending state and the journal come
// first, every payload and metadata object is uploaded in between, and reference writes carry the
// conditional headers that make ownership atomic.
func TestPublish_RequestOrdering(t *testing.T) {
	for _, spec := range allSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			spec = spec.fresh(t).with(func(s *rspec) { s.prefix = testPrefix })
			repo := newRepo(t, spec)
			publish(t, repo, &prepareSpec{files: bigFiles()}, artifact.PublishOptions{})

			reference := refKey(testPrefix)
			var refWrites []s3test.Request
			lastWrite := -1
			writes := mutations(spec.fake)
			for index, request := range writes {
				if request.Op == s3test.OpDeleteObject {
					continue
				}
				lastWrite = index
				if request.Key == reference {
					refWrites = append(refWrites, request)
				}
			}
			require.Len(t, refWrites, 2, "one pending write and one completed write")
			assert.Equal(t, reference, writes[lastWrite].Key, "the completed reference is written last")
			assert.Equal(t, "*", refWrites[0].Header.Get("If-None-Match"), "the pending reference is created with If-None-Match")
			assert.Empty(t, refWrites[0].Header.Get("If-Match"))
			assert.NotEmpty(t, refWrites[1].Header.Get("If-Match"), "the completed reference replaces the pending ETag with If-Match")
			assert.Empty(t, refWrites[1].Header.Get("If-None-Match"))

			journal := testPrefix + "/" + layout.OperationPath("op-"+testName+"-"+testVersion)
			journalIndex, pendingIndex := -1, -1
			for index, request := range writes {
				switch {
				case request.Key == journal && journalIndex < 0 && request.Op == s3test.OpPutObject:
					journalIndex = index
				case request.Key == reference && pendingIndex < 0:
					pendingIndex = index
				}
			}
			require.GreaterOrEqual(t, journalIndex, 0)
			assert.Less(t, journalIndex, pendingIndex, "the journal is recorded before the reference turns pending")
			for index, request := range writes {
				if request.Op == s3test.OpDeleteObject || request.Key == reference || request.Key == journal || strings.Contains(request.Key, "/locks/") {
					continue
				}
				assert.Greater(t, index, pendingIndex, "%s is written only after the reference is pending", request.Key)
			}
			assertIntact(t, repo, defaultRef())
		})
	}
}

// Every object of a publication carries the configured tags, including the parts of multipart
// uploads, documents, records, journals, and claims; portable labels never become tags.
func TestPublish_TagsOnEveryObject(t *testing.T) {
	tags := map[string]string{"team": "platform", "env": "prod"}
	for _, spec := range allSpecs {
		t.Run(specName(spec), func(t *testing.T) {
			spec = spec.fresh(t).with(func(s *rspec) { s.tags, s.prefix = tags, testPrefix })
			repo := newRepo(t, spec)
			publish(t, repo, &prepareSpec{files: bigFiles(), labels: map[string]string{"suite": "tags"}}, artifact.PublishOptions{})
			signer := newSigner()
			repo = newRepo(t, spec.withOpts(WithVerifier(&artifacttest.FakeVerifier{})))
			pinned := resolve(t, repo, defaultRef())
			subject := recordSubject(t, repo, &pinned)
			_, err := repo.Sign(context.Background(), subject, signer, artifact.SignOptions{})
			require.NoError(t, err)

			keys := bucketKeys(spec.fake)
			require.NotEmpty(t, keys)
			for _, key := range keys {
				object, ok := spec.fake.Object(key)
				require.True(t, ok)
				assert.Equal(t, tags, object.Tags, "object %s", key)
			}
			assert.NotEmpty(t, payloadKeys(spec.fake, testPrefix))
			assert.NotEmpty(t, managedKeys(spec.fake, testPrefix, layout.ManagedRoot+"/signatures"), "evidence is tagged like everything else")
			for _, request := range spec.fake.Requests() {
				if request.Op == s3test.OpPutObject || request.Op == s3test.OpCreateMultipartUpload {
					assert.NotEmpty(t, request.Header.Get("X-Amz-Tagging"), "%s %s", request.Op, request.Key)
					assert.NotContains(t, request.Header.Get("X-Amz-Tagging"), "suite", "labels are not tags")
				}
			}
			assert.GreaterOrEqual(t, spec.fake.CompletedUploads(), 1, "the large file used multipart")
		})
	}
}

// Two writers racing on one reference: the loser of the conditional write gets a conflict and never
// overwrites the winner.
func TestPublish_ConditionalConflictBetweenTwoWriters(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, winner *Repository) *artifact.PublishOptions
	}{
		{name: "creation loses If-None-Match", setup: func(*testing.T, *Repository) *artifact.PublishOptions {
			return &artifact.PublishOptions{Lock: dummyLock{}}
		}},
		{name: "replacement loses If-Match", setup: func(t *testing.T, winner *Repository) *artifact.PublishOptions {
			base := publish(t, winner, &prepareSpec{files: map[string]string{"data.txt": "base"}}, artifact.PublishOptions{})
			return &artifact.PublishOptions{Force: true, ExpectedPrior: &base.Pinned, Lock: dummyLock{}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := noneSpec().fresh(t)
			winner := newRepo(t, spec)
			options := tt.setup(t, winner)

			var finished *artifact.PublishResult
			loserStore := &hookStore{Store: newStoreFor(t, spec)}
			loserStore.beforePut = func(key string, _ blob.PutOptions) {
				if finished != nil || !strings.Contains(key, "/locks/") {
					return
				}
				// The winner publishes completely between the loser's read and its reservation.
				won, err := winner.Publish(context.Background(), prepare(t, winner, &prepareSpec{files: map[string]string{"data.txt": "winner"}, opID: "winner-op"}),
					artifact.PublishOptions{Force: options.Force, ExpectedPrior: options.ExpectedPrior, Lock: dummyLock{}})
				require.NoError(t, err)
				finished = won
			}
			loser, err := newRepository("loser", spec.config(spec.fake.Bucket()), []Option{WithStore(loserStore), withLockPoll(5 * time.Millisecond)}, &artifact.RepositoryOptions{})
			require.NoError(t, err)

			_, err = loser.Publish(context.Background(), prepare(t, loser, &prepareSpec{files: map[string]string{"data.txt": "loser"}, opID: "loser-op"}), *options)
			require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
			require.NotNil(t, finished)
			pinned := resolve(t, winner, defaultRef())
			assert.Equal(t, finished.Pinned.RecordDigest, pinned.RecordDigest, "the winner's artifact is untouched")
			assertIntact(t, winner, defaultRef())
		})
	}
}

// An endpoint that cannot enforce a needed condition fails the operation clearly and never
// overwrites: nothing is written to the reference namespace.
func TestPublish_UnsupportedConditionalFailsClearly(t *testing.T) {
	tests := []struct {
		name string
		spec func(*testing.T) *rspec
	}{
		{name: "endpoint answers 501", spec: func(t *testing.T) *rspec {
			return &rspec{packaging: artifact.PackagingTgz, fake: s3test.NewServer(t, s3test.WithConditionalBehavior(s3test.ConditionalNotImplemented))}
		}},
		{name: "endpoint ignores the headers and is probed", spec: func(t *testing.T) *rspec {
			return &rspec{packaging: artifact.PackagingTgz, probe: true, fake: s3test.NewServer(t, s3test.WithConditionalBehavior(s3test.ConditionalIgnore))}
		}},
		{name: "conditional writes disabled", spec: func(t *testing.T) *rspec {
			return &rspec{packaging: artifact.PackagingTgz, storeOpts: []blobs3.Option{blobs3.WithConditionalMode(blobs3.ConditionalDisabled)}, fake: s3test.NewServer(t)}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.spec(t)
			repo := newRepo(t, spec)
			prepared := prepare(t, repo, &prepareSpec{})

			_, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
			require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
			capability, ok := artifact.AsCapabilityError(err)
			require.True(t, ok)
			assert.Contains(t, capability.Capability, "lock", "the lock claim is the first thing that needs a condition")

			// Bypassing the lock reaches the reference write itself, which must fail just as clearly.
			_, err = repo.Publish(context.Background(), prepared, artifact.PublishOptions{Lock: dummyLock{}})
			require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
			assert.Empty(t, managedKeys(spec.fake, "", layout.ReferencesDir()), "no reference was written")
			assert.Empty(t, payloadKeys(spec.fake, ""), "no payload was written")
		})
	}
}

// A conditional-less store advertises no lock scope and no conditional update.
func TestCapabilities_WithoutConditionalWrites(t *testing.T) {
	spec := noneSpec().fresh(t)
	spec.storeOpts = []blobs3.Option{blobs3.WithConditionalMode(blobs3.ConditionalDisabled)}
	caps := newRepo(t, spec).Capabilities()
	assert.False(t, caps.ConditionalUpdate)
	assert.Empty(t, caps.LockScopes)
	_, err := newRepo(t, spec).Lock(context.Background(), artifact.LockRequest{Resource: "x"})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
}
