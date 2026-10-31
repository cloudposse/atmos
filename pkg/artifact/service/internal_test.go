package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

func TestCanonicalOrder_SortsByRepositoryThenResourceAndDropsDuplicates(t *testing.T) {
	targets := []lockTarget{
		{alias: "b", resource: "z"},
		{alias: "a", resource: "y"},
		{alias: "a", resource: "x"},
		{alias: "b", resource: "a"},
		{alias: "a", resource: "x"},
	}
	ordered := canonicalOrder(targets)
	var got []string
	for _, target := range ordered {
		got = append(got, target.alias+"/"+target.resource)
	}
	assert.Equal(t, []string{"a/x", "a/y", "b/a", "b/z"}, got)
	assert.Equal(t, "b", targets[0].alias, "the caller's slice is not reordered")
}

func TestAcquireLocks_MultiResourceUsesCanonicalOrderAndReleasesInReverse(t *testing.T) {
	ctrl := gomock.NewController(t)
	var order []string
	mkRepo := func(alias string) *artifact.MockRepository {
		repo := artifact.NewMockRepository(ctrl)
		handle := artifact.NewMockLockHandle(ctrl)
		handle.EXPECT().Resource().Return("res").AnyTimes()
		handle.EXPECT().Owner().Return("owner").AnyTimes()
		handle.EXPECT().Scope().Return(artifact.LockScopeHost).AnyTimes()
		repo.EXPECT().Lock(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, request artifact.LockRequest) (artifact.LockHandle, error) {
			order = append(order, "lock "+alias+"/"+request.Resource)
			assert.Equal(t, "owner-x", request.Owner)
			assert.Equal(t, "op-9", request.OperationID)
			assert.Equal(t, artifact.LockScopeHost, request.Required)
			return handle, nil
		})
		handle.EXPECT().Release(gomock.Any()).DoAndReturn(func(context.Context) error {
			order = append(order, "release "+alias)
			return nil
		})
		return repo
	}
	repoA := mkRepo("a")
	repoB := mkRepo("b")
	caps := artifact.Capabilities{LockScopes: []artifact.LockScope{artifact.LockScopeHost}}
	infoA := &RepositoryInfo{Alias: "a", Capabilities: caps}
	infoB := &RepositoryInfo{Alias: "b", Capabilities: caps}

	s := &Service{owner: "owner-x"}
	op := &operation{id: "op-9"}
	set, err := s.acquireLocks(context.Background(), op, "copy", []lockTarget{
		{alias: "b", repo: repoB, info: infoB, resource: "r2"},
		{alias: "a", repo: repoA, info: infoA, resource: "r1"},
	})
	require.NoError(t, err)
	assert.NotNil(t, set.handleFor("a"))
	assert.Nil(t, set.handleFor("c"))
	assert.Equal(t, artifact.LockScopeHost, set.scope())
	require.NoError(t, set.release(context.Background(), op))
	assert.Equal(t, []string{"lock a/r1", "lock b/r2", "release b", "release a"}, order)
}

func TestAcquireLocks_FailureReleasesWhatWasTakenAndInsufficientScopeFailsFirst(t *testing.T) {
	ctrl := gomock.NewController(t)
	first := artifact.NewMockRepository(ctrl)
	second := artifact.NewMockRepository(ctrl)
	handle := artifact.NewMockLockHandle(ctrl)
	handle.EXPECT().Resource().Return("res").AnyTimes()
	handle.EXPECT().Owner().Return("owner").AnyTimes()
	first.EXPECT().Lock(gomock.Any(), gomock.Any()).Return(handle, nil)
	handle.EXPECT().Release(gomock.Any()).Return(nil)
	second.EXPECT().Lock(gomock.Any(), gomock.Any()).Return(nil, errUtils.ErrArtifactConflict)
	caps := artifact.Capabilities{LockScopes: []artifact.LockScope{artifact.LockScopeHost}}

	s := &Service{owner: "o"}
	op := &operation{id: "op"}
	_, err := s.acquireLocks(context.Background(), op, "copy", []lockTarget{
		{alias: "a", repo: first, info: &RepositoryInfo{Capabilities: caps}, resource: "r"},
		{alias: "b", repo: second, info: &RepositoryInfo{Capabilities: caps}, resource: "r"},
	})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict)

	weak := &RepositoryInfo{Alias: "w", Capabilities: caps, RequiredLockScope: artifact.LockScopeRepository}
	untouched := artifact.NewMockRepository(ctrl) // No Lock expectation: it must never be called.
	_, err = s.acquireLocks(context.Background(), op, "copy", []lockTarget{{alias: "w", repo: untouched, info: weak, resource: "r"}})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
}

func TestLockSet_NilAndEmptySetsAreSafe(t *testing.T) {
	var set *lockSet
	assert.Nil(t, set.handleFor("a"))
	assert.Empty(t, set.scope())
	require.NoError(t, set.release(context.Background(), &operation{}))
	assert.Empty(t, (&lockSet{}).scope())
}

func TestLockSet_ScopeIsTheWeakestHeld(t *testing.T) {
	ctrl := gomock.NewController(t)
	repoHandle := artifact.NewMockLockHandle(ctrl)
	hostHandle := artifact.NewMockLockHandle(ctrl)
	repoHandle.EXPECT().Scope().Return(artifact.LockScopeRepository).AnyTimes()
	hostHandle.EXPECT().Scope().Return(artifact.LockScopeHost).AnyTimes()
	set := &lockSet{held: []heldLock{{handle: repoHandle}, {handle: hostHandle}}}
	assert.Equal(t, artifact.LockScopeHost, set.scope())
	set = &lockSet{held: []heldLock{{handle: hostHandle}, {handle: repoHandle}}}
	assert.Equal(t, artifact.LockScopeHost, set.scope())
}

func TestSplitPolicyAndMerge(t *testing.T) {
	global := artifact.VerificationPolicy{RequireSignature: true, TrustSets: []artifact.TrustSet{{Keys: []string{"g.pub"}}}}
	repo := artifact.VerificationPolicy{TrustSets: []artifact.TrustSet{{Identities: []artifact.TrustedIdentity{{Identity: "i", Issuer: "https://x"}}}}, Offline: true}
	merged := mergePolicies(global, repo)
	assert.True(t, merged.RequireSignature)
	assert.True(t, merged.Offline)
	require.Len(t, merged.TrustSets, 2, "sets are kept separate, never unioned")
	calls := splitPolicy(merged)
	require.Len(t, calls, 2)
	assert.True(t, calls[0].Offline)
	assert.Len(t, calls[1].TrustSets, 1)

	bare := splitPolicy(artifact.VerificationPolicy{})
	require.Len(t, bare, 1)
	assert.True(t, bare[0].RequireSignature, "an empty policy that is consulted still demands a signature")

	assert.False(t, verificationRequired(artifact.VerificationPolicy{}))
	assert.True(t, verificationRequired(artifact.VerificationPolicy{RequireSignature: true}))
	assert.True(t, verificationRequired(artifact.VerificationPolicy{TrustSets: []artifact.TrustSet{{Keys: []string{"k"}}}}))
}

func TestSubjectOf(t *testing.T) {
	_, err := subjectOf(&artifact.Inspection{})
	require.ErrorIs(t, err, errUtils.ErrArtifactRecordInvalid)

	root := artifact.Inspection{Representation: artifact.Representation{OriginalRoot: &ocispecRoot}}
	subject, err := subjectOf(&root)
	require.NoError(t, err)
	assert.Equal(t, artifact.SubjectOCIRoot, subject.Kind)
	assert.Equal(t, ocispecRoot.Digest, subject.Digest, "a native graph is signed by its original root, never an archive checksum")
}

func TestMergeMetadataAndDifferences(t *testing.T) {
	merged := mergeMetadata(artifact.Metadata{Description: "d", Labels: map[string]string{"a": "1"}}, artifact.Metadata{Labels: map[string]string{"a": "2", "b": "3"}})
	assert.Equal(t, "d", merged.Description)
	assert.Equal(t, map[string]string{"a": "2", "b": "3"}, merged.Labels)
	assert.Nil(t, mergeMetadata(artifact.Metadata{}, artifact.Metadata{}).Labels)

	a := &artifact.Record{Kind: "files"}
	b := &artifact.Record{Kind: "other", KindData: artifact.KindData(`{"x":1}`)}
	assert.Contains(t, describeDifferences(a, b), "kind data")
	assert.Equal(t, "intent", describeDifferences(a, a))
	assert.Contains(t, describeDifferences(nil, a), "no portable record")
}

func TestIsDefinitiveAndTemporary(t *testing.T) {
	assert.True(t, isDefinitive(errUtils.ErrArtifactConflict))
	assert.True(t, isDefinitive(errUtils.ErrArtifactUnsupportedCapability))
	assert.False(t, isDefinitive(context.Canceled))
	assert.False(t, isTemporary(context.Canceled))
}

var ocispecRoot = ocispecDescriptor()
