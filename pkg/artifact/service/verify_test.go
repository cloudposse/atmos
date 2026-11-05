package service_test

import (
	"context"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/service"
)

var releaseSigner = &artifacttest.FakeSigner{Identity: "release@example.com", Issuer: "https://issuer.example.com"}

// signedEnv publishes the example artifact and signs it through the service.
func signedEnv(t *testing.T, config envConfig) (*env, *service.PublishOutcome) {
	t.Helper()
	config.options = append(config.options, service.WithSigner(releaseSigner))
	e := newEnv(t, config)
	published := e.publish("dst", e.source("hello"))
	_, err := e.svc.Sign(context.Background(), artifact.SignRequest{Repository: "dst", Ref: artifact.Ref{Name: "tools/example", Digest: published.Operation.RecordDigest}})
	require.NoError(t, err)
	return e, published
}

func TestSign_BindsEvidenceToTheDigestAndReportsSignedNotVerified(t *testing.T) {
	e := newEnv(t, envConfig{options: []service.Option{service.WithSigner(releaseSigner)}})
	published := e.publish("dst", e.source("hello"))

	result, err := e.svc.Sign(context.Background(), artifact.SignRequest{Repository: "dst", Ref: artifact.Ref{Name: "tools/example", Digest: published.Operation.RecordDigest}})
	require.NoError(t, err)
	assert.Equal(t, artifact.SubjectRecord, result.Subject.Kind)
	assert.Equal(t, published.Operation.RecordDigest, result.Subject.Digest)
	require.Len(t, result.Evidence, 1)
	assert.Equal(t, artifact.OutcomeCreated, result.Operation.Outcome)
	assert.Equal(t, artifact.LockScopeHost, result.Operation.LockScope)
	var notVerified bool
	for _, diagnostic := range result.Operation.Diagnostics {
		notVerified = notVerified || diagnostic.Code == "signed-not-verified"
	}
	assert.True(t, notVerified, "signing alone reports signed, not policy-verified")
	assert.Empty(t, e.repos["dst"].HeldLocks())

	inspected, err := e.svc.Inspect(context.Background(), artifact.InspectRequest{Repository: "dst", Ref: exampleRef})
	require.NoError(t, err)
	assert.Nil(t, inspected.Verification, "with no trust configuration the artifact stays unchecked even though it is signed")
	assert.Len(t, inspected.Inspection.Evidence, 1)
}

func TestSign_Rejections(t *testing.T) {
	e := newEnv(t, envConfig{options: []service.Option{service.WithSigner(releaseSigner)}})
	published := e.publish("dst", e.source("hello"))
	good := artifact.Ref{Name: "tools/example", Digest: published.Operation.RecordDigest}
	tests := []struct {
		name    string
		request artifact.SignRequest
		want    error
	}{
		{"version reference", artifact.SignRequest{Repository: "dst", Ref: exampleRef}, errUtils.ErrArtifactRefInvalid},
		{"missing repository", artifact.SignRequest{Ref: good}, errUtils.ErrArtifactConfigInvalid},
		{"digest of another subject", artifact.SignRequest{Repository: "dst", Ref: artifact.Ref{Name: "tools/example", Digest: digest.FromString("other")}}, errUtils.ErrArtifactNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.svc.Sign(context.Background(), tt.request)
			require.ErrorIs(t, err, tt.want)
			require.NotNil(t, result)
		})
	}
	assert.Zero(t, e.repos["dst"].Calls("Sign"))
}

func TestSign_CapabilityChecksPrecedeLocking(t *testing.T) {
	noEvidence := artifacttest.DefaultCapabilities()
	noEvidence.EvidenceFormats = nil
	e := newEnv(t, envConfig{
		repos:   map[string]artifacttest.Options{"dst": {Capabilities: &noEvidence}},
		options: []service.Option{service.WithSigner(releaseSigner)},
	})
	published := e.publish("dst", e.source("hello"))
	locks := e.repos["dst"].Calls("Lock")
	_, err := e.svc.Sign(context.Background(), artifact.SignRequest{Repository: "dst", Ref: artifact.Ref{Name: "tools/example", Digest: published.Operation.RecordDigest}})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	assert.Equal(t, locks, e.repos["dst"].Calls("Lock"), "no lock is taken for an unsupported signing request")

	noSigner := newEnv(t, envConfig{})
	published = noSigner.publish("dst", noSigner.source("hello"))
	_, err = noSigner.svc.Sign(context.Background(), artifact.SignRequest{Repository: "dst", Ref: artifact.Ref{Name: "tools/example", Digest: published.Operation.RecordDigest}})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
}

func TestVerify_RequiresATrustedValidSignature(t *testing.T) {
	e, _ := signedEnv(t, envConfig{})
	request := artifact.VerifyRequest{Repository: "dst", Ref: exampleRef}

	t.Run("no trust configuration is not a green result", func(t *testing.T) {
		result, err := e.svc.Verify(context.Background(), request)
		require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
		assert.Equal(t, artifact.VerificationUnchecked, result.Verification.Status)
		assert.NotEqual(t, artifact.VerificationVerified, result.Verification.Status)
	})

	t.Run("trusted signer verifies", func(t *testing.T) {
		e.setInfo("dst", func(info *service.RepositoryInfo) { info.Policy = releaseSigner.TrustedPolicy() })
		result, err := e.svc.Verify(context.Background(), request)
		require.NoError(t, err)
		assert.Equal(t, artifact.VerificationVerified, result.Verification.Status)
		assert.Equal(t, artifact.SubjectRecord, result.Verification.Subject.Kind)
		assert.NotEmpty(t, result.Verification.EvidenceDigests)
		assert.NotEmpty(t, result.Verification.PolicyFingerprint)
		require.NotEmpty(t, result.Verification.Claims)
		assert.Equal(t, "release@example.com", result.Verification.Claims[0].Identity)
	})

	t.Run("untrusted signer fails", func(t *testing.T) {
		e.setInfo("dst", func(info *service.RepositoryInfo) { info.Policy = releaseSigner.UntrustedPolicy() })
		result, err := e.svc.Verify(context.Background(), request)
		require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
		assert.Equal(t, artifact.VerificationFailed, result.Verification.Status)
	})
}

func TestVerify_GlobalAndRepositoryTrustSetsBothMustBeSatisfied(t *testing.T) {
	e, published := signedEnv(t, envConfig{})
	second := &artifacttest.FakeSigner{Identity: "security@example.com", Issuer: "https://issuer.example.com"}
	policy := func(signers ...*artifacttest.FakeSigner) artifact.VerificationPolicy {
		var merged artifact.VerificationPolicy
		merged.RequireSignature = true
		for _, s := range signers {
			merged.TrustSets = append(merged.TrustSets, s.TrustedPolicy().TrustSets...)
		}
		return merged
	}
	e.setInfo("dst", func(info *service.RepositoryInfo) { info.Policy = policy(releaseSigner, second) })

	_, err := e.svc.Verify(context.Background(), artifact.VerifyRequest{Repository: "dst", Ref: exampleRef})
	require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed, "satisfying one of two required trust sets is not enough; they are never unioned")

	subject := artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: published.Operation.RecordDigest, MediaType: artifact.MediaTypeConfig}
	_, err = e.repos["dst"].Sign(context.Background(), subject, second, artifact.SignOptions{})
	require.NoError(t, err)
	result, err := e.svc.Verify(context.Background(), artifact.VerifyRequest{Repository: "dst", Ref: exampleRef})
	require.NoError(t, err)
	assert.Equal(t, artifact.VerificationVerified, result.Verification.Status)
	assert.Len(t, result.Verification.Claims, 2)
}

func TestConsumption_RequiredVerificationRunsBeforeMaterializationAndForceDoesNotBypassIt(t *testing.T) {
	e := newEnv(t, envConfig{options: []service.Option{service.WithSigner(releaseSigner)}})
	published := e.publish("dst", e.source("hello"))
	e.setInfo("dst", func(info *service.RepositoryInfo) { info.Policy = releaseSigner.TrustedPolicy() })
	reads := e.repos["dst"].Calls("Read")

	dest := out(t)
	result, err := e.svc.Pull(context.Background(), artifact.PullRequest{Repository: "dst", Ref: exampleRef, Destination: dest, Force: true})
	require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed, "force never bypasses verification")
	assert.NotNil(t, result)
	assert.Equal(t, reads, e.repos["dst"].Calls("Read"), "verification ran before the payload was read")
	assert.True(t, missing(dest), "nothing was materialized")

	_, err = e.svc.Sign(context.Background(), artifact.SignRequest{Repository: "dst", Ref: artifact.Ref{Name: "tools/example", Digest: published.Operation.RecordDigest}})
	require.NoError(t, err)
	_, err = e.svc.Pull(context.Background(), artifact.PullRequest{Repository: "dst", Ref: exampleRef, Destination: dest})
	require.NoError(t, err)
}

func TestInspect_ReportsVerificationWhenPolicyConstrainsIt(t *testing.T) {
	e := newEnv(t, envConfig{options: []service.Option{service.WithSigner(releaseSigner)}})
	published := e.publish("dst", e.source("hello"))
	e.setInfo("dst", func(info *service.RepositoryInfo) { info.Policy = releaseSigner.TrustedPolicy() })

	unsigned, err := e.svc.Inspect(context.Background(), artifact.InspectRequest{Repository: "dst", Ref: exampleRef})
	require.NoError(t, err, "inspect consumes no payload, so a failed verification is information, not a hidden artifact")
	require.NotNil(t, unsigned.Verification)
	assert.Equal(t, artifact.VerificationFailed, unsigned.Verification.Status)

	_, err = e.svc.Sign(context.Background(), artifact.SignRequest{Repository: "dst", Ref: artifact.Ref{Name: "tools/example", Digest: published.Operation.RecordDigest}})
	require.NoError(t, err)
	signed, err := e.svc.Inspect(context.Background(), artifact.InspectRequest{Repository: "dst", Ref: exampleRef})
	require.NoError(t, err)
	require.NotNil(t, signed.Verification)
	assert.Equal(t, artifact.VerificationVerified, signed.Verification.Status)
}

func TestShowDocument_RequiredVerificationApplies(t *testing.T) {
	e := newEnv(t, envConfig{})
	e.publish("dst", e.source("hello"))
	e.setInfo("dst", func(info *service.RepositoryInfo) { info.Policy = releaseSigner.TrustedPolicy() })
	_, err := e.svc.ShowDocument(context.Background(), artifact.ShowDocumentRequest{Repository: "dst", Ref: exampleRef, Role: artifact.DocumentReadme})
	require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
}

func TestVerification_UnsupportedSubjectKindFailsClosed(t *testing.T) {
	caps := artifacttest.DefaultCapabilities()
	caps.SubjectKinds = nil
	e := newEnv(t, envConfig{repos: map[string]artifacttest.Options{"dst": {Capabilities: &caps}}})
	e.publish("dst", e.source("hello"))
	e.setInfo("dst", func(info *service.RepositoryInfo) { info.Policy = releaseSigner.TrustedPolicy() })
	_, err := e.svc.Pull(context.Background(), artifact.PullRequest{Repository: "dst", Ref: exampleRef, Destination: out(t)})
	require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "unsupported required evidence fails closed and says why")
}

func TestVerification_RepositoryResultsAreNeverTrustedBlindly(t *testing.T) {
	record := artifact.Record{Name: "tools/example", Version: "1", Kind: artifact.KindFiles, CreatedAt: artifact.NewTimestamp(startTime)}
	require.NoError(t, record.Seal())
	recordDigest, err := record.RecordDigest()
	require.NoError(t, err)
	wrongSubject := artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: digest.FromString("other")}

	tests := []struct {
		name   string
		result *artifact.VerificationResult
		err    error
	}{
		{"nil result", nil, nil},
		{"status unchecked", &artifact.VerificationResult{Status: artifact.VerificationUnchecked}, nil},
		{"result for another subject", &artifact.VerificationResult{Subject: wrongSubject, Status: artifact.VerificationVerified}, nil},
		{"verified but errored", &artifact.VerificationResult{Status: artifact.VerificationVerified}, errTransport},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := artifact.NewMockRepository(ctrl)
			caps := artifacttest.DefaultCapabilities()
			repo.EXPECT().Capabilities().Return(caps).AnyTimes()
			pinned := artifact.PinnedReference{Ref: exampleRef, ResolvedDigest: recordDigest, RecordDigest: recordDigest}
			repo.EXPECT().Resolve(gomock.Any(), exampleRef).Return(pinned, nil)
			repo.EXPECT().Inspect(gomock.Any(), pinned).Return(&artifact.Inspection{Pinned: pinned, Record: &record}, nil)
			repo.EXPECT().Verify(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, subject artifact.SignatureSubject, _ artifact.VerificationPolicy) (*artifact.VerificationResult, error) {
					if tt.result != nil && tt.result.Subject.Digest == "" {
						tt.result.Subject = subject
					}
					return tt.result, tt.err
				},
			)
			repos := &service.StaticRepositories{
				Repositories: map[string]artifact.Repository{"dst": repo},
				Info:         map[string]service.RepositoryInfo{"dst": {Capabilities: caps, Packaging: artifact.PackagingNone, Policy: releaseSigner.TrustedPolicy()}},
			}
			svc, err := service.New(repos, nil, service.WithStagingRoot(t.TempDir()))
			require.NoError(t, err)
			result, err := svc.Verify(context.Background(), artifact.VerifyRequest{Repository: "dst", Ref: exampleRef})
			require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
			assert.NotEqual(t, artifact.VerificationVerified, result.Verification.Status)
		})
	}
}
