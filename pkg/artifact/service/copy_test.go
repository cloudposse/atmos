package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/materialize"
	"github.com/cloudposse/atmos/pkg/artifact/service"
)

// copyEnv has a "src" repository and a "dst" repository with chosen packagings.
func copyEnv(t *testing.T, srcPackaging, dstPackaging artifact.Packaging, options ...service.Option) *env {
	t.Helper()
	e := newEnv(t, envConfig{repos: map[string]artifacttest.Options{"src": {}, "dst": {}}, options: options})
	e.setInfo("src", func(info *service.RepositoryInfo) { info.Packaging = srcPackaging })
	e.setInfo("dst", func(info *service.RepositoryInfo) { info.Packaging = dstPackaging })
	return e
}

func TestCopy_PreservesRecordAcrossPackagingChanges(t *testing.T) {
	packagings := []artifact.Packaging{artifact.PackagingNone, artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip}
	for _, from := range packagings {
		for _, to := range packagings {
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				e := copyEnv(t, from, to)
				e.setInfo("dst", func(info *service.RepositoryInfo) {
					info.Defaults = artifact.Metadata{Labels: map[string]string{"destination-default": "x"}}
				})
				published := e.publish("src", e.source("hello"))
				e.clock.Advance(72 * 3600e9)

				result, err := e.svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst"})
				require.NoError(t, err)
				assert.Equal(t, artifact.OutcomeCreated, result.Outcome)
				assert.Equal(t, "dst", result.Repository)
				assert.Equal(t, published.Operation.RecordDigest, result.RecordDigest, "a copy preserves Record bytes and identity")
				assert.Equal(t, published.Record.ContentDigest, result.ContentDigest)
				assert.Equal(t, artifact.LockScopeHost, result.LockScope)

				inspected, err := e.svc.Inspect(context.Background(), artifact.InspectRequest{Repository: "dst", Ref: exampleRef})
				require.NoError(t, err)
				assert.Equal(t, to, inspected.Inspection.Representation.Packaging)
				want, err := published.Record.Canonical()
				require.NoError(t, err)
				got, err := inspected.Inspection.Record.Canonical()
				require.NoError(t, err)
				assert.Equal(t, want, got, "created_at and labels are not restamped; destination defaults do not apply to copies")

				dest := out(t)
				_, err = e.svc.Pull(context.Background(), artifact.PullRequest{Repository: "dst", Ref: exampleRef, Destination: dest})
				require.NoError(t, err)
				state, err := materialize.InspectDestination(dest, &published.Record)
				require.NoError(t, err)
				assert.Equal(t, materialize.DestinationMatches, state)
				e.assertStagingClean()
			})
		}
	}
}

func TestCopy_ConflictForceAndUnchanged(t *testing.T) {
	e := copyEnv(t, artifact.PackagingNone, artifact.PackagingTgz)
	e.publish("src", e.source("original"))

	first, err := e.svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst"})
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeCreated, first.Outcome)

	again, err := e.svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst"})
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeUnchanged, again.Outcome, "copying an identical artifact is a no-op")
	assert.Equal(t, 1, e.repos["dst"].Calls("Publish"))

	e.publishForce("src", e.source("different"))
	_, err = e.svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst"})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	forced, err := e.svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst", Force: true})
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeReplaced, forced.Outcome)
	e.assertStagingClean()
}

func (e *env) publishForce(repository, src string) {
	e.t.Helper()
	request := e.publishRequest(repository, src)
	request.Force = true
	_, err := e.svc.PublishDetailed(context.Background(), request)
	require.NoError(e.t, err)
}

func TestCopy_PluginReleasesStayImmutableOnTheDestinationEvenWithForce(t *testing.T) {
	e := copyEnv(t, artifact.PackagingNone, artifact.PackagingNone, service.WithImmutabilityPolicy(pluginPolicy()))
	pluginRequest := e.publishRequest("dst", e.source("plugin build"))
	pluginRequest.Metadata.Labels = map[string]string{"format": "plugin"}
	_, err := e.svc.Publish(context.Background(), pluginRequest)
	require.NoError(t, err)
	e.publish("src", e.source("generic"))

	_, err = e.svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst", Force: true})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	assert.Contains(t, err.Error(), "immutable")
}

func TestCopy_TagMovedMidCopyCopiesTheOriginallyPinnedSubject(t *testing.T) {
	e := copyEnv(t, artifact.PackagingNone, artifact.PackagingTgz)
	original := e.publish("src", e.source("ORIGINAL"))
	decoy := e.publishRequest("src", e.source("DECOY"))
	decoy.Version = "2.0.0"
	_, err := e.svc.Publish(context.Background(), decoy)
	require.NoError(t, err)
	decoyDigest, _ := e.repos["src"].RecordDigestOf(ref("tools/example", "2.0.0"))
	moved := false
	e.repos["src"].SetOnResolved(func(resolved artifact.Ref) {
		if resolved == exampleRef && !moved {
			moved = true
			e.repos["src"].MoveRef(exampleRef, decoyDigest)
		}
	})
	before := e.repos["src"].Calls("Resolve")

	result, err := e.svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst"})
	require.NoError(t, err)
	assert.Equal(t, 1, e.repos["src"].Calls("Resolve")-before, "the source reference is resolved once")
	assert.Equal(t, original.Operation.RecordDigest, result.RecordDigest)
	dest := out(t)
	_, err = e.svc.Pull(context.Background(), artifact.PullRequest{Repository: "dst", Ref: exampleRef, Destination: dest})
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dest, "data.txt"))
	require.NoError(t, err)
	assert.Equal(t, "ORIGINAL", string(data))
}

func TestCopy_ValidationAndCapabilitiesPrecedeAnyRead(t *testing.T) {
	readOnly := artifacttest.DefaultCapabilities()
	readOnly.Write = false
	e := newEnv(t, envConfig{repos: map[string]artifacttest.Options{"src": {}, "dst": {Capabilities: &readOnly}}})
	e.publish("src", e.source("hello"))
	reads := e.repos["src"].TotalCalls()

	_, err := e.svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst"})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	assert.Equal(t, reads, e.repos["src"].TotalCalls(), "the source is untouched when the destination cannot be written")

	for name, request := range map[string]artifact.CopyRequest{
		"same repository": {Ref: exampleRef, From: "src", To: "src"},
		"missing source":  {Ref: exampleRef, To: "dst"},
		"missing target":  {Ref: exampleRef, From: "src"},
		"invalid ref":     {Ref: artifact.Ref{Name: "x"}, From: "src", To: "dst"},
	} {
		_, err := e.svc.Copy(context.Background(), request)
		require.Error(t, err, name)
	}
	writable := copyEnv(t, artifact.PackagingNone, artifact.PackagingNone)
	_, err = writable.svc.Copy(context.Background(), artifact.CopyRequest{Ref: ref("tools/example", "9"), From: "src", To: "dst"})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
}

func TestCopy_SourceAndDestinationPoliciesBothApplyAndForceNeverBypassesThem(t *testing.T) {
	trusted := releaseSigner.TrustedPolicy()
	tests := []struct {
		name      string
		srcPolicy artifact.VerificationPolicy
		dstPolicy artifact.VerificationPolicy
	}{
		{"source requires", trusted, artifact.VerificationPolicy{}},
		{"destination requires", artifact.VerificationPolicy{}, trusted},
		{"both require", trusted, trusted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := copyEnv(t, artifact.PackagingNone, artifact.PackagingTgz, service.WithSigner(releaseSigner))
			published := e.publish("src", e.source("hello"))
			e.setInfo("src", func(info *service.RepositoryInfo) { info.Policy = tt.srcPolicy })
			e.setInfo("dst", func(info *service.RepositoryInfo) { info.Policy = tt.dstPolicy })
			dstCalls := e.repos["dst"].TotalCalls()

			_, err := e.svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst", Force: true})
			require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed, "unsigned content cannot satisfy either policy, and force does not help")
			assert.Equal(t, dstCalls, e.repos["dst"].TotalCalls(), "verification ran before the destination was touched")
			_, found := e.repos["dst"].RecordDigestOf(exampleRef)
			assert.False(t, found)

			_, err = e.svc.Sign(context.Background(), artifact.SignRequest{Repository: "src", Ref: artifact.Ref{Name: "tools/example", Digest: published.Operation.RecordDigest}})
			require.NoError(t, err)
			result, err := e.svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst"})
			require.NoError(t, err)
			assert.Equal(t, artifact.OutcomeCreated, result.Outcome)

			verified, err := e.svc.Verify(context.Background(), artifact.VerifyRequest{Repository: "dst", Ref: exampleRef})
			if verificationConfigured(tt.dstPolicy) {
				require.NoError(t, err, "evidence travels with the copy so the destination verifies independently")
				assert.Equal(t, artifact.VerificationVerified, verified.Verification.Status)
			}
		})
	}
}

func verificationConfigured(policy artifact.VerificationPolicy) bool {
	return policy.RequireSignature || policy.HasTrustConstraints()
}

// plainRepository hides optional extension interfaces of the wrapped repository.
type plainRepository struct{ artifact.Repository }

func TestCopy_WarnsWhenEvidenceCannotTravel(t *testing.T) {
	e := copyEnv(t, artifact.PackagingNone, artifact.PackagingNone, service.WithSigner(releaseSigner))
	published := e.publish("src", e.source("hello"))
	_, err := e.svc.Sign(context.Background(), artifact.SignRequest{Repository: "src", Ref: artifact.Ref{Name: "tools/example", Digest: published.Operation.RecordDigest}})
	require.NoError(t, err)
	e.static.Repositories["dst"] = plainRepository{Repository: e.repos["dst"]}
	e.setInfo("src", func(info *service.RepositoryInfo) { info.Policy = releaseSigner.TrustedPolicy() })
	e.setInfo("dst", func(info *service.RepositoryInfo) { info.Policy = releaseSigner.TrustedPolicy() })

	result, err := e.svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst"})
	require.NoError(t, err)
	var warned bool
	for _, diagnostic := range result.Diagnostics {
		warned = warned || diagnostic.Code == "evidence-not-carried"
	}
	assert.True(t, warned, "missing evidence transfer is reported, never silently dropped")
}

// nativeRepository adds a compatible native transfer to the in-memory repository.
type nativeRepository struct {
	*artifacttest.Repository
	copies  int
	options artifact.CopyFromOptions
	compat  bool
}

func (n *nativeRepository) CanCopyFrom(artifact.Repository) bool { return n.compat }

//nolint:gocritic // The pinned reference is passed by value because artifact.Copier fixes the signature.
func (n *nativeRepository) CopyFrom(ctx context.Context, source artifact.Repository, pinned artifact.PinnedReference, options artifact.CopyFromOptions) (*artifact.PublishResult, error) {
	n.copies++
	n.options = options
	inspection, err := source.Inspect(ctx, pinned)
	if err != nil {
		return nil, err
	}
	return &artifact.PublishResult{Outcome: artifact.OutcomeCreated, Pinned: pinned, Representation: inspection.Representation, Location: "native://copy"}, nil
}

func TestCopy_UsesNativeTransferWhenCompatible(t *testing.T) {
	e := copyEnv(t, artifact.PackagingNone, artifact.PackagingNone)
	published := e.publish("src", e.source("hello"))
	native := &nativeRepository{Repository: e.repos["dst"], compat: true}
	e.static.Repositories["dst"] = native
	reads := e.repos["src"].Calls("Read")

	result, err := e.svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst", Force: true})
	require.NoError(t, err)
	assert.Equal(t, 1, native.copies)
	assert.Equal(t, reads, e.repos["src"].Calls("Read"), "the staging fallback is not used")
	assert.Equal(t, "native://copy", result.Location)
	assert.Equal(t, published.Operation.RecordDigest, result.RecordDigest)
	assert.Nil(t, native.options.ExpectedPrior)
	assert.NotNil(t, native.options.Lock, "the destination lock is held across a native transfer")
	assert.False(t, native.options.Force, "force applies only when replacement was decided")

	native.compat = false
	_, err = e.svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "dst"})
	require.NoError(t, err)
	assert.Equal(t, 1, native.copies, "an incompatible pair falls back to staging")
}
