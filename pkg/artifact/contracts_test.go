package artifact

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Compile-time proof that every generated mock satisfies its interface.
var (
	_ Repository   = (*MockRepository)(nil)
	_ LockProvider = (*MockLockProvider)(nil)
	_ LockProvider = Repository(nil)
	_ LockHandle   = (*MockLockHandle)(nil)
	_ Producer     = (*MockProducer)(nil)
	_ Signer       = (*MockSigner)(nil)
	_ Verifier     = (*MockVerifier)(nil)
	_ Copier       = (*MockCopier)(nil)
	_ Service      = (*MockService)(nil)
	_ GraphSource  = (*MockGraphSource)(nil)
)

func TestLockScopeSatisfies(t *testing.T) {
	tests := []struct {
		have, need LockScope
		want       bool
	}{
		{LockScopeHost, LockScopeHost, true},
		{LockScopeHost, "", true},
		{LockScopeRepository, LockScopeHost, true},
		{LockScopeRepository, LockScopeRepository, true},
		{LockScopeHost, LockScopeRepository, false},
		{"", LockScopeHost, false},
		{LockScopeRepository, "cluster", false},
	}
	for _, tt := range tests {
		t.Run(string(tt.have)+"->"+string(tt.need), func(t *testing.T) {
			assert.Equal(t, tt.want, tt.have.Satisfies(tt.need))
		})
	}
}

func TestCapabilitiesLookups(t *testing.T) {
	capabilities := Capabilities{
		Packagings:      []Packaging{PackagingTgz, PackagingZip},
		EvidenceFormats: []EvidenceFormat{EvidenceSigstoreBundle},
		SubjectKinds:    []SubjectKind{SubjectRecord},
		LockScopes:      []LockScope{LockScopeHost},
	}
	assert.True(t, capabilities.HasPackaging(PackagingTgz))
	assert.False(t, capabilities.HasPackaging(PackagingNone))
	assert.True(t, capabilities.HasEvidenceFormat(EvidenceSigstoreBundle))
	assert.False(t, capabilities.HasEvidenceFormat(EvidencePublicKeyBundle), "a missing format is not silently supported")
	assert.True(t, capabilities.HasSubjectKind(SubjectRecord))
	assert.False(t, capabilities.HasSubjectKind(SubjectOCIRoot))
	assert.True(t, capabilities.HasLockScope(LockScopeHost))
	assert.False(t, capabilities.HasLockScope(LockScopeRepository))

	capabilities.LockScopes = []LockScope{LockScopeRepository}
	assert.True(t, capabilities.HasLockScope(LockScopeHost), "repository scope satisfies a host request")
}

func TestPreparedArtifactCloseRunsCleanupOnce(t *testing.T) {
	calls := 0
	boom := errors.New("cleanup failed")
	prepared := &PreparedArtifact{Cleanup: func() error { calls++; return boom }}
	require.ErrorIs(t, prepared.Close(), boom)
	require.ErrorIs(t, prepared.Close(), boom, "later calls return the first result")
	assert.Equal(t, 1, calls)

	assert.NoError(t, (&PreparedArtifact{}).Close(), "nil cleanup is fine")
}

type trackingCloser struct {
	io.Reader
	closed *int
	err    error
}

func (c trackingCloser) Close() error { *c.closed++; return c.err }

func TestContentCloseClosesEveryStream(t *testing.T) {
	closed := 0
	first := errors.New("first")
	content := &Content{Streams: []ContentStream{
		{Reader: trackingCloser{Reader: strings.NewReader("a"), closed: &closed, err: first}},
		{Reader: nil},
		{Reader: trackingCloser{Reader: strings.NewReader("b"), closed: &closed, err: errors.New("second")}},
	}}
	require.ErrorIs(t, content.Close(), first)
	assert.Equal(t, 2, closed, "an early failure does not leak later streams")
}

func TestSignatureSubjectValidate(t *testing.T) {
	good := SignatureSubject{Kind: SubjectRecord, Digest: digest.FromString("r"), MediaType: MediaTypeConfig, Size: 10}
	require.NoError(t, good.Validate())

	bad := good
	bad.Kind = "archive"
	require.ErrorIs(t, bad.Validate(), errUtils.ErrArtifactRefInvalid)
	bad = good
	bad.Digest = "sha256:short"
	require.ErrorIs(t, bad.Validate(), errUtils.ErrArtifactRefInvalid)
	bad = good
	bad.Size = -1
	require.ErrorIs(t, bad.Validate(), errUtils.ErrArtifactRefInvalid)
}

// TestRepositoryContractThroughMocks drives the interfaces the way the service will: ask a
// repository for a guarantee it lacks and observe a typed error, not a no-op.
func TestRepositoryContractThroughMocks(t *testing.T) {
	ctrl := gomock.NewController(t)
	repository := NewMockRepository(ctrl)

	repository.EXPECT().Capabilities().Return(Capabilities{LockScopes: []LockScope{LockScopeHost}})
	repository.EXPECT().
		Lock(gomock.Any(), LockRequest{Resource: "vpc:1.0", Owner: "host-1", Required: LockScopeRepository}).
		Return(nil, NewUnsupportedCapability("shared", "lock", "lock/repository", "configure locking.repository"))

	request := LockRequest{Resource: "vpc:1.0", Owner: "host-1", Required: LockScopeRepository}
	var repo Repository = repository
	capabilities := repo.Capabilities()
	require.False(t, capabilities.HasLockScope(request.Required))

	handle, err := repo.Lock(context.Background(), request)
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	assert.Nil(t, handle)
}

func TestOptionalCopierIsDetectedByTypeAssertion(t *testing.T) {
	ctrl := gomock.NewController(t)
	var plain Repository = NewMockRepository(ctrl)
	_, ok := plain.(Copier)
	assert.False(t, ok, "a repository without native copy falls back to staged copy")

	var native Repository = struct {
		*MockRepository
		*MockCopier
	}{NewMockRepository(ctrl), NewMockCopier(ctrl)}
	_, ok = native.(Copier)
	assert.True(t, ok)
}
