package s3

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
)

// A location an interrupted attempt abandoned stays claimed by its reference until a retry retires
// the payload. Until then another reference must not reserve it, even while it holds no objects:
// the retiring attempt deletes by the journaled payload and would destroy the newcomer's archive
// and metadata sidecar.
func TestAbandonedLocationStaysClaimedUntilRetired(t *testing.T) {
	const abandonedArchive = "team/app/1.0.0.tgz"
	fake := s3test.NewServer(t)
	onFake := func(s *rspec) { s.fake = fake }

	// The first attempt stops before uploading anything, then a retry that needs a different
	// packaging stops too: its journal lists the tgz location as abandoned although it is empty.
	first := newRepo(t, archiveSpec(artifact.PackagingTgz).with(onFake).withOpts(withFaults(crashAt(faultBeforePayload))))
	_, err := first.Publish(context.Background(), prepare(t, first, &prepareSpec{opID: "op-first"}), artifact.PublishOptions{})
	require.ErrorIs(t, err, errCrash)
	stopped := newRepo(t, archiveSpec(artifact.PackagingZip).with(onFake).withOpts(withFaults(crashAt(faultBeforePayload))))
	_, err = stopped.Publish(context.Background(), prepare(t, stopped, &prepareSpec{opID: "op-stopped"}), artifact.PublishOptions{})
	require.ErrorIs(t, err, errCrash)
	_, present := fake.Object(abandonedArchive)
	require.False(t, present, "the abandoned location holds no object yet")

	newcomer := newRepo(t, archiveSpec(artifact.PackagingTgz).withPattern("team/app/{{ .Version }}.tgz").with(onFake))
	newcomerRef := ref("other/app", testVersion)
	before := bucketKeys(fake)
	_, err = newcomer.Publish(context.Background(), prepare(t, newcomer, &prepareSpec{name: "other/app", files: map[string]string{"a.txt": "newcomer"}}), artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	assert.ErrorContains(t, err, abandonedArchive)
	assert.Equal(t, before, bucketKeys(fake), "the refused reservation changed nothing")
	_, err = newcomer.Resolve(context.Background(), newcomerRef)
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)

	// The retry that completes the first reference retires the abandoned payload and releases the
	// claim: the location is free again.
	final := newRepo(t, archiveSpec(artifact.PackagingZip).with(onFake))
	_, err = final.Publish(context.Background(), prepare(t, final, &prepareSpec{opID: "op-final"}), artifact.PublishOptions{})
	require.NoError(t, err)
	assertIntact(t, final, defaultRef())
	_, err = newcomer.Publish(context.Background(), prepare(t, newcomer, &prepareSpec{name: "other/app", files: map[string]string{"a.txt": "newcomer"}}), artifact.PublishOptions{})
	require.NoError(t, err, "a retired location is claimable again")
	assertIntact(t, newcomer, newcomerRef)
	assertIntact(t, final, defaultRef())
}
