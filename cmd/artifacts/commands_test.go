package artifacts

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/schema"
)

var namedOutputs = []string{"ref", "location", "resolved_digest", "record_digest", "content_digest", "representation_digest", "size", "outcome"}

func decodeObject(t *testing.T, raw string) map[string]any {
	t.Helper()
	var object map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &object), raw)
	return object
}

func TestPublishMapsEveryFlagAndEmitsTheNamedOutputs(t *testing.T) {
	h, svc := newHarness(t)
	svc.EXPECT().Publish(gomock.Any(), artifact.PublishRequest{
		Repository: "dir", Source: "./dist", Name: "builds/app", Version: "build-1", Kind: "component/terraform",
		Metadata: artifact.Metadata{Labels: map[string]string{"team": "platform", "purpose": "backup", "env": "dev"}},
		Readme:   "README.md", Changelog: "CHANGELOG.md", Force: true, DryRun: false,
	}).Return(operation(artifact.ActionPublish, artifact.OutcomeCreated), nil)

	got := h.run("publish", "./dist", "--repository=dir", "--name=builds/app", "--version=build-1", "--kind=component/terraform",
		"--labels=team=platform,purpose:backup", "--labels", "env=dev", "--readme=README.md", "--changelog=CHANGELOG.md", "--force", "--format=json")
	require.NoError(t, got.err)
	object := decodeObject(t, got.stdout)
	for _, name := range namedOutputs {
		assert.Contains(t, object, name)
	}
	assert.Equal(t, "created", object["outcome"])
	assert.Equal(t, string(manifestDigest), object["resolved_digest"])
	assert.Equal(t, string(recordDigest), object["record_digest"])
	assert.Empty(t, got.stderr, "structured output prints no human status")
}

func TestShortFlagsEnvironmentAndPrecedence(t *testing.T) {
	h, svc := newHarness(t)
	t.Setenv(envRepository, "archive")
	t.Setenv(envVersion, "9.9.9")
	t.Setenv(envDryRun, "true")
	var got artifact.PublishRequest
	svc.EXPECT().Publish(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, request artifact.PublishRequest) (*artifact.OperationResult, error) {
		got = request
		return operation(artifact.ActionPublish, artifact.OutcomeDryRun), nil
	}).Times(2)

	require.NoError(t, h.run("publish", "./dist", "--format=json").err)
	assert.Equal(t, artifact.PublishRequest{Repository: "archive", Source: "./dist", Version: "9.9.9", DryRun: true}, got,
		"environment variables supply repository, version, and dry-run")

	require.NoError(t, h.run("publish", "./dist", "-r", "dir", "--version=1.0.0", "-n", "-f", "--format=json").err)
	assert.Equal(t, "dir", got.Repository, "a flag overrides the environment")
	assert.Equal(t, "1.0.0", got.Version)
	assert.True(t, got.Force)
}

func TestRequiredRepositoryIsAnErrorForEverySingleRepositoryCommand(t *testing.T) {
	h, _ := newHarness(t)
	digestRef := "team/app@" + string(recordDigest)
	for _, args := range [][]string{
		{"publish", "./dist", "--version=1"},
		{"pull", "team/app:1", "--output=./out"},
		{"inspect", "team/app:1"},
		{"delete", "team/app:1"},
		{"sign", digestRef},
		{"verify", "team/app:1"},
		{"show", "readme", "team/app:1"},
		{"show", "changelog", "team/app:1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			got := h.run(args...)
			require.ErrorIs(t, got.err, errUtils.ErrRequiredFlagNotProvided)
			hints := cockroach.GetAllHints(got.err)
			require.NotEmpty(t, hints)
			assert.Contains(t, strings.Join(hints, "\n"), "archive, dir", "the error names the configured repositories")
			assert.Equal(t, 2, errUtils.GetExitCode(got.err))
			assert.Empty(t, got.stdout, "a text-mode usage error prints nothing on stdout")
		})
	}
}

func TestStructuredFailureStillEmitsTheEnvelopeAndKeepsTheErrorForTheExitCode(t *testing.T) {
	h, svc := newHarness(t)
	failure := artifact.OperationResult{OperationID: "op-9", Action: artifact.ActionVerify, Repository: "dir", Requested: "team/app:1.0.0", ResolvedDigest: manifestDigest}
	svc.EXPECT().Verify(gomock.Any(), gomock.Any()).Return(
		&artifact.VerifyResult{Operation: failure, Verification: artifact.VerificationResult{Status: artifact.VerificationUnchecked}},
		errors.Join(errUtils.ErrArtifactVerificationFailed, errors.New("no permitted signature")),
	).Times(2)

	for _, format := range []string{"json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			got := h.run("verify", "team/app:1.0.0", "-r", "dir", "--format="+format)
			require.ErrorIs(t, got.err, errUtils.ErrArtifactVerificationFailed, "the failure is returned so the process exits nonzero")
			assert.NotEmpty(t, cockroach.GetAllHints(got.err))

			var object map[string]any
			if format == "json" {
				object = decodeObject(t, got.stdout)
			} else {
				require.NoError(t, yaml.Unmarshal([]byte(got.stdout), &object), got.stdout)
			}
			assert.Equal(t, "failed", object["outcome"])
			assert.Contains(t, object["error"], "no permitted signature")
			assert.Equal(t, "verify", object["action"])
			assert.Equal(t, string(manifestDigest), object["resolved_digest"])
			assert.Equal(t, "unchecked", object["verification"].(map[string]any)["status"])
		})
	}
}

func TestTextFailureWritesNothingToStdout(t *testing.T) {
	h, svc := newHarness(t)
	svc.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(&artifact.OperationResult{Action: artifact.ActionDelete}, errUtils.ErrArtifactNotFound)

	got := h.run("delete", "team/app:1.0.0", "-r", "dir")
	require.ErrorIs(t, got.err, errUtils.ErrArtifactNotFound)
	assert.Empty(t, got.stdout)
	assert.Contains(t, strings.Join(cockroach.GetAllHints(got.err), "\n"), "atmos artifacts list")
}

func TestTextSuccessSeparatesDataFromStatus(t *testing.T) {
	h, svc := newHarness(t)
	svc.EXPECT().Publish(gomock.Any(), gomock.Any()).Return(operation(artifact.ActionPublish, artifact.OutcomeCreated), nil)

	got := h.run("publish", "./dist", "-r", "dir", "--version=1.0.0")
	require.NoError(t, got.err)
	assert.Contains(t, got.stdout, "resolved_digest:")
	assert.Contains(t, got.stdout, "record_digest:")
	assert.Contains(t, got.stdout, string(manifestDigest))
	assert.NotContains(t, got.stdout, "Published", "status belongs on stderr")
	assert.Contains(t, got.stderr, "Published team/app:1.0.0 to repository dir (created)")
}

func TestInvalidFormatAndArgumentsAreRejectedBeforeAnyServiceCall(t *testing.T) {
	h, _ := newHarness(t)
	got := h.run("publish", "./dist", "-r", "dir", "--format=xml")
	require.ErrorIs(t, got.err, errUtils.ErrInvalidFlag)

	got = h.run("publish")
	require.ErrorIs(t, got.err, errUtils.ErrInvalidPositionalArgs)
	got = h.run("copy", "team/app:1", "--from=dir")
	require.ErrorIs(t, got.err, errUtils.ErrRequiredFlagNotProvided, "copy needs both --from and --to")
	got = h.run("pull", "team/app:1", "-r", "dir")
	require.ErrorIs(t, got.err, errUtils.ErrRequiredFlagNotProvided, "pull needs --output")
	got = h.run("inspect", "team/app", "-r", "dir")
	require.ErrorIs(t, got.err, errUtils.ErrArtifactRefInvalid, "there is no implicit latest")
	assert.Contains(t, strings.Join(cockroach.GetAllHints(got.err), "\n"), "no implicit `latest`")
	got = h.run("publish", "./dist", "-r", "dir", "--labels=broken")
	require.ErrorIs(t, got.err, errUtils.ErrInvalidFlag)
}

func TestMissingConfigurationIsAnActionableError(t *testing.T) {
	h, _ := newHarness(t)
	atmosConfigPtr = nil
	got := h.run("inspect", "team/app:1", "-r", "dir", "--format=json")
	require.ErrorIs(t, got.err, errUtils.ErrArtifactConfigInvalid)
	assert.Equal(t, "failed", decodeObject(t, got.stdout)["outcome"])
}

func TestServiceConstructionFailureIsReportedLikeAnyOtherFailure(t *testing.T) {
	h, _ := newHarness(t)
	newService = func(*schema.AtmosConfiguration, ServiceOptions) (artifact.Service, error) {
		return nil, errUtils.ErrArtifactConfigInvalid
	}
	got := h.run("inspect", "team/app:1", "-r", "dir")
	require.ErrorIs(t, got.err, errUtils.ErrArtifactConfigInvalid)
}

func TestSignPassesTheTransparencyLogConsentToTheComposition(t *testing.T) {
	h, svc := newHarness(t)
	ref := "team/app@" + string(recordDigest)
	svc.EXPECT().Sign(gomock.Any(), artifact.SignRequest{Repository: "dir", Ref: artifact.Ref{Name: "team/app", Digest: recordDigest}}).
		Return(&artifact.SignResult{
			Operation: *operation(artifact.ActionSign, artifact.OutcomeCreated),
			Subject:   artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: recordDigest},
		}, nil).Times(2)

	require.NoError(t, h.run("sign", ref, "-r", "dir").err)
	require.NoError(t, h.run("sign", ref, "-r", "dir", "--yes").err)
	require.Len(t, h.options, 2)
	assert.False(t, h.options[0].AcceptTransparencyLog)
	assert.True(t, h.options[1].AcceptTransparencyLog)
}

func TestSignResultExposesTheSubjectAndBothDigests(t *testing.T) {
	h, svc := newHarness(t)
	svc.EXPECT().Sign(gomock.Any(), gomock.Any()).Return(&artifact.SignResult{
		Operation: *operation(artifact.ActionSign, artifact.OutcomeCreated),
		Subject:   artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: recordDigest},
	}, nil)

	got := h.run("sign", "team/app@"+string(manifestDigest), "-r", "dir", "--format=json")
	require.NoError(t, got.err)
	object := decodeObject(t, got.stdout)
	assert.Equal(t, string(manifestDigest), object["resolved_digest"], "the digest the user sees")
	assert.Equal(t, string(recordDigest), object["record_digest"])
	assert.Equal(t, string(recordDigest), object["subject"].(map[string]any)["digest"], "the signed subject")
}

func TestVerifyReportsSignersAndInspectReportsUncheckedWithoutTrust(t *testing.T) {
	h, svc := newHarness(t)
	svc.EXPECT().Verify(gomock.Any(), gomock.Any()).Return(&artifact.VerifyResult{
		Operation: *operation(artifact.ActionVerify, artifact.OutcomeRead),
		Verification: artifact.VerificationResult{
			Status: artifact.VerificationVerified, Subject: artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: recordDigest},
			Claims: []artifact.SignerClaim{{Identity: "release@example.com", Issuer: "https://issuer.example.com"}},
		},
	}, nil)
	svc.EXPECT().Inspect(gomock.Any(), gomock.Any()).Return(&artifact.InspectResult{
		Operation: *operation(artifact.ActionInspect, artifact.OutcomeRead),
		Inspection: artifact.Inspection{
			Record:         &artifact.Record{Name: "team/app", Version: "1.0.0", Kind: "files", Metadata: artifact.Metadata{Description: "demo", Labels: map[string]string{"team": "platform"}}},
			Representation: artifact.Representation{Packaging: artifact.PackagingTgz},
			Evidence:       []artifact.SignatureEvidence{{}},
		},
	}, nil)

	verified := h.run("verify", "team/app:1.0.0", "-r", "dir")
	require.NoError(t, verified.err)
	assert.Contains(t, verified.stdout, "verification:")
	assert.Contains(t, verified.stdout, "verified")
	assert.Contains(t, verified.stdout, "release@example.com (issuer https://issuer.example.com)")

	inspected := h.run("inspect", "team/app:1.0.0", "-r", "dir")
	require.NoError(t, inspected.err)
	assert.Contains(t, inspected.stdout, "verification:          unchecked", "without trust configuration the artifact is unchecked, never verified")
	assert.Contains(t, inspected.stdout, "labels:                team=platform")
	assert.Contains(t, inspected.stdout, "signatures:            1 (untrusted until verified)")
}

func TestShowPrintsTheDocumentVerbatimOnStdout(t *testing.T) {
	h, svc := newHarness(t)
	svc.EXPECT().ShowDocument(gomock.Any(), artifact.ShowDocumentRequest{Repository: "dir", Ref: artifact.Ref{Name: "team/app", Version: "1.0.0"}, Role: artifact.DocumentReadme}).
		Return(&artifact.DocumentResult{Operation: *operation(artifact.ActionShow, artifact.OutcomeRead), Document: artifact.Document{Role: artifact.DocumentReadme, Digest: contentDigest}, Content: []byte("# App\n\nbody\n")}, nil)
	svc.EXPECT().ShowDocument(gomock.Any(), artifact.ShowDocumentRequest{Repository: "dir", Ref: artifact.Ref{Name: "team/app", Version: "1.0.0"}, Role: artifact.DocumentChangelog}).
		Return(&artifact.DocumentResult{Operation: *operation(artifact.ActionShow, artifact.OutcomeRead), Document: artifact.Document{Role: artifact.DocumentChangelog, Digest: contentDigest}, Content: []byte("- first\n")}, nil)

	readme := h.run("show", "readme", "team/app:1.0.0", "-r", "dir")
	require.NoError(t, readme.err)
	assert.Equal(t, "# App\n\nbody\n", readme.stdout)
	assert.Empty(t, readme.stderr)

	changelog := h.run("show", "changelog", "team/app:1.0.0", "-r", "dir", "--format=json")
	require.NoError(t, changelog.err)
	assert.Equal(t, "- first\n", decodeObject(t, changelog.stdout)["content"])
}

func TestCopyPullAndDeleteMapTheirFlags(t *testing.T) {
	h, svc := newHarness(t)
	ref := artifact.Ref{Name: "team/app", Version: "1.0.0"}
	svc.EXPECT().Copy(gomock.Any(), artifact.CopyRequest{Ref: ref, From: "dir", To: "archive", Force: true}).Return(operation(artifact.ActionCopy, artifact.OutcomeCreated), nil)
	svc.EXPECT().Pull(gomock.Any(), artifact.PullRequest{Repository: "dir", Ref: ref, Destination: "./out", Force: true}).Return(operation(artifact.ActionPull, artifact.OutcomeRead), nil)
	svc.EXPECT().Delete(gomock.Any(), artifact.DeleteRequest{Repository: "dir", Ref: ref}).Return(operation(artifact.ActionDelete, artifact.OutcomeDeleted), nil)

	require.NoError(t, h.run("copy", "team/app:1.0.0", "--from=dir", "--to=archive", "--force", "--format=json").err)
	require.NoError(t, h.run("pull", "team/app:1.0.0", "-r", "dir", "--output=./out", "-f", "--format=json").err)
	got := h.run("delete", "team/app:1.0.0", "-r", "dir")
	require.NoError(t, got.err)
	assert.Contains(t, got.stderr, "Deleted team/app:1.0.0")
}
