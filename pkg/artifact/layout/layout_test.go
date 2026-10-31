package layout

import (
	"strings"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

var (
	digestA = digest.FromString("a")
	digestB = digest.FromString("b")
	digestC = digest.FromString("c")
)

func TestReferenceKey(t *testing.T) {
	key := ReferenceKey("components/terraform/vpc", "1.2.0")
	assert.Len(t, key, 64, "lowercase hex SHA-256")
	assert.Equal(t, digest.FromString("components/terraform/vpc:1.2.0").Encoded(), key, "the key is the SHA-256 of name:version")
	assert.NotEqual(t, key, ReferenceKey("components/terraform", "vpc:1.2.0"), "names with slashes cannot collide with other name/version splits")
	assert.NotContains(t, key, "/", "no slash can reach a control path")
}

func TestPaths(t *testing.T) {
	key := ReferenceKey("a", "1")
	assert.Equal(t, ".atmos-artifacts/v1/refs/"+key+".json", ReferencePath(key))
	assert.Equal(t, ".atmos-artifacts/v1/representations/"+key+".json", RepresentationPath(key))
	assert.Equal(t, ".atmos-artifacts/v1/records/"+digestA.Encoded()+".json", RecordPath(digestA))
	assert.Equal(t, ".atmos-artifacts/v1/refs", ReferencesDir())
	assert.Equal(t,
		".atmos-artifacts/v1/signatures/record/"+digestA.Encoded()+"/"+digestB.Encoded()+".json",
		SignaturePath(artifact.SubjectRecord, digestA, digestB))
	assert.Equal(t, "x/y.tgz.metadata.json", MetadataSidecarPath("x/y.tgz"))
	assert.Equal(t, "x/y.tgz.docs/README.md", DocumentSidecarPath("x/y.tgz", artifact.DocumentReadme))
	assert.Equal(t, "x/y.tgz.docs/CHANGELOG.md", DocumentSidecarPath("x/y.tgz", artifact.DocumentChangelog))
	assert.Equal(t, "x/y/.atmos/artifact/manifest.json", DistributionPath("x/y", ManifestName))
	assert.True(t, strings.HasPrefix(LockPath("any resource/with:odd chars"), ManagedRoot+"/locks/"))
	assert.NotEqual(t, LockPath("a"), LockPath("b"))
}

func TestOperationFileName(t *testing.T) {
	tests := []struct {
		name string
		id   string
		safe bool
	}{
		{"uuid", "9f1c6d52-48b6-4c3c-9f0e-3a5b7f1d1c11", true},
		{"dotted", "op.1_x-2", true},
		{"slash", "op-team/app-1.0.0", false},
		{"space", "op one", false},
		{"traversal", "../x", false},
		{"overlong", strings.Repeat("a", 200), false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OperationFileName(tt.id)
			if tt.safe {
				assert.Equal(t, tt.id, got)
			} else {
				assert.True(t, strings.HasPrefix(got, "sha256-"), got)
			}
			assert.NotContains(t, got, "/")
			assert.Equal(t, got, OperationFileName(tt.id), "stable")
			assert.Equal(t, ManagedRoot+"/operations/"+got+".json", OperationPath(tt.id))
			assert.Equal(t, ManagedRoot+"/staging/"+got, StagingPath(tt.id))
		})
	}
}

func TestInManagedNamespace(t *testing.T) {
	assert.True(t, InManagedNamespace(".atmos-artifacts/v1/refs/x.json"))
	assert.True(t, InManagedNamespace("./.ATMOS-Artifacts"))
	assert.False(t, InManagedNamespace("team/.atmos-artifacts"), "only the first segment is the root namespace")
	assert.False(t, InManagedNamespace("atmos-artifacts/x"))
}

func TestRenderLocation(t *testing.T) {
	captured := time.Date(2026, 10, 30, 23, 59, 59, 0, time.UTC)
	in := &PatternInput{Name: "components/terraform/vpc", Version: "1.2.0", Kind: "component/terraform", CreatedAt: captured}
	tests := []struct {
		name      string
		pattern   string
		packaging artifact.Packaging
		want      string
		wantErr   error
	}{
		{"default copied files", "", artifact.PackagingNone, "components/terraform/vpc/1.2.0", nil},
		{"default tgz", "", artifact.PackagingTgz, "components/terraform/vpc/1.2.0.tgz", nil},
		{"default tar", "", artifact.PackagingTar, "components/terraform/vpc/1.2.0.tar", nil},
		{"default zip", "", artifact.PackagingZip, "components/terraform/vpc/1.2.0.zip", nil},
		{"dated root from the PRD", `{{ .CreatedAt | date "2006/01/02" }}/{{ .Name }}/{{ .Version }}.tgz`, artifact.PackagingTgz, "2026/10/30/components/terraform/vpc/1.2.0.tgz", nil},
		{"kind field", "{{ .Kind }}/{{ .Version }}", artifact.PackagingNone, "component/terraform/1.2.0", nil},
		{"trailing slash is trimmed", "x/{{ .Version }}/", artifact.PackagingNone, "x/1.2.0", nil},
		{"sprig function", "{{ .Name | upper }}/{{ .Version }}", artifact.PackagingNone, "COMPONENTS/TERRAFORM/VPC/1.2.0", nil},
		{"mismatched archive extension", "{{ .Name }}.zip", artifact.PackagingTgz, "", errUtils.ErrArtifactPackagingInvalid},
		{"tar.gz agrees with tgz", "{{ .Name }}.tar.gz", artifact.PackagingTgz, "components/terraform/vpc.tar.gz", nil},
		{"no extension is not renamed", "{{ .Name }}", artifact.PackagingTgz, "components/terraform/vpc", nil},
		{"root escape", "../{{ .Name }}", artifact.PackagingNone, "", errUtils.ErrArtifactUnsafePath},
		{"rendered root escape", "{{ \"..\" }}/x", artifact.PackagingNone, "", errUtils.ErrArtifactUnsafePath},
		{"absolute", "/abs/{{ .Name }}", artifact.PackagingNone, "", errUtils.ErrArtifactUnsafePath},
		{"empty segment", "a//b", artifact.PackagingNone, "", errUtils.ErrArtifactUnsafePath},
		{"reserved namespace", ".atmos-artifacts/{{ .Name }}", artifact.PackagingNone, "", errUtils.ErrArtifactUnsafePath},
		{"reserved namespace any case", "x/.ATMOS-ARTIFACTS/y", artifact.PackagingNone, "", errUtils.ErrArtifactUnsafePath},
		{"windows reserved character", "a:b/{{ .Name }}", artifact.PackagingNone, "", errUtils.ErrArtifactUnsafePath},
		{"unknown field", "{{ .Stack }}", artifact.PackagingNone, "", errUtils.ErrArtifactConfigInvalid},
		{"syntax error", "{{ .Name", artifact.PackagingNone, "", errUtils.ErrArtifactConfigInvalid},
		{"clock is not available", "{{ now | date \"2006\" }}/x", artifact.PackagingNone, "", errUtils.ErrArtifactConfigInvalid},
		{"environment is not available", "{{ env \"HOME\" }}/x", artifact.PackagingNone, "", errUtils.ErrArtifactConfigInvalid},
		{"empty result", "{{ \"\" }}", artifact.PackagingNone, "", errUtils.ErrArtifactUnsafePath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderLocation(tt.pattern, in, tt.packaging)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRenderLocation_DateIsUTCAndStable(t *testing.T) {
	// 23:30 at UTC-5 is already the next day in UTC; the captured instant decides, not the zone.
	zone := time.FixedZone("UTC-5", -5*60*60)
	local := time.Date(2026, 10, 30, 23, 30, 0, 0, zone)
	in := &PatternInput{Name: "a", Version: "1", Kind: "files", CreatedAt: local}
	pattern := `{{ .CreatedAt | date "2006/01/02" }}/{{ .Name }}`
	first, err := RenderLocation(pattern, in, artifact.PackagingNone)
	require.NoError(t, err)
	assert.Equal(t, "2026/10/31/a", first)

	// Rendering again with the same captured time is stable even if the wall clock moved on.
	second, err := RenderLocation(pattern, in, artifact.PackagingNone)
	require.NoError(t, err)
	assert.Equal(t, first, second)

	rolled := *in
	rolled.CreatedAt = local.Add(48 * time.Hour)
	third, err := RenderLocation(pattern, &rolled, artifact.PackagingNone)
	require.NoError(t, err)
	assert.NotEqual(t, first, third, "a different captured time renders a different dated root; callers persist the first")
}

func TestCleanLocation(t *testing.T) {
	for _, valid := range []string{"a", "a/b", "a/b/", "a/b.tgz", "team/app/1.0.0", "ünï/çode"} {
		_, err := CleanLocation(valid)
		require.NoError(t, err, valid)
	}
	for _, bad := range []string{"", "/", "/a", "a/../b", "./a", "a/./b", `a\b`, "a\x00b", "a\nb", ".atmos-artifacts", "a/.Atmos-Artifacts/b", "a|b", "a\xff", strings.Repeat("a", 2000)} {
		_, err := CleanLocation(bad)
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath, "%q", bad)
	}
}

func TestClaims(t *testing.T) {
	archive := func(key, location string) Claim {
		return Claim{ReferenceKey: key, Paths: PayloadPaths(artifact.RepositoryKindLocalArchive, location)}
	}
	dir := func(key, location string) Claim {
		return Claim{ReferenceKey: key, Paths: PayloadPaths(artifact.RepositoryKindLocalDir, location)}
	}
	assert.Equal(t, []string{"x/1.tgz", "x/1.tgz.metadata.json", "x/1.tgz.docs"}, archive("k", "x/1.tgz").Paths)
	assert.Equal(t, []string{"x/1"}, dir("k", "x/1").Paths)

	tests := []struct {
		name      string
		candidate Claim
		existing  []Claim
		conflict  bool
	}{
		{"identical location", dir("b", "x/1"), []Claim{dir("a", "x/1")}, true},
		{"case-folded location", dir("b", "X/1"), []Claim{dir("a", "x/1")}, true},
		{"candidate inside existing directory", dir("b", "x/1/y"), []Claim{dir("a", "x/1")}, true},
		{"existing inside candidate directory", dir("b", "x"), []Claim{dir("a", "x/1")}, true},
		{"archive sidecar collides with another archive", archive("b", "x/1.tgz.metadata.json"), []Claim{archive("a", "x/1.tgz")}, true},
		{"archive docs directory collides", archive("b", "x/1.tgz.docs/README.md"), []Claim{archive("a", "x/1.tgz")}, true},
		{"siblings do not collide", dir("b", "x/2"), []Claim{dir("a", "x/1")}, false},
		{"prefix without a separator is not an overlap", dir("b", "x/10"), []Claim{dir("a", "x/1")}, false},
		{"the same reference never conflicts with itself", dir("a", "x/1"), []Claim{dir("a", "x/1")}, false},
		{"no claims", dir("a", "x/1"), nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := FindConflict(&tt.candidate, tt.existing)
			assert.Equal(t, tt.conflict, found != nil)
			if tt.conflict {
				assert.Equal(t, "a", found.ReferenceKey)
			}
		})
	}
}

func TestReference_StateModel(t *testing.T) {
	first := NewPendingReference("team/app", "1.0.0", "op-1", digestA, nil)
	require.NoError(t, first.Validate())
	assert.False(t, first.Available(), "a pending reference is unavailable to readers")
	assert.EqualValues(t, 1, first.Generation)
	assert.Empty(t, first.PriorRecordDigest)
	assert.Equal(t, ReferenceKey("team/app", "1.0.0"), first.Key())

	done := first.Completed(digestB)
	require.NoError(t, done.Validate())
	assert.True(t, done.Available())
	assert.EqualValues(t, 2, done.Generation, "every state change bumps the generation")
	assert.Equal(t, digestB, done.RecordDigest)
	assert.Equal(t, StatePending, first.State, "Completed does not mutate its receiver")

	replacing := NewPendingReference("team/app", "1.0.0", "op-2", digestC, &done)
	require.NoError(t, replacing.Validate())
	assert.False(t, replacing.Available())
	assert.EqualValues(t, 3, replacing.Generation)
	assert.Equal(t, digestB, replacing.PriorRecordDigest, "a replacement remembers what it replaces")
	assert.Empty(t, replacing.RecordDigest, "the pending reference must not advertise either record")

	again := NewPendingReference("team/app", "1.0.0", "op-3", digestC, &replacing)
	assert.Equal(t, digestB, again.PriorRecordDigest, "a re-pending interrupted replacement keeps the original prior")
	assert.EqualValues(t, 4, again.Generation)

	finished := replacing.Completed(digestC)
	assert.Empty(t, finished.PriorRecordDigest)
	assert.EqualValues(t, 4, finished.Generation)
}

func TestReference_RoundTripAndStrictness(t *testing.T) {
	ref := NewPendingReference("team/app", "1.0.0", "op-1", digestA, nil)
	ref = ref.Completed(digestB)
	raw, err := ref.Marshal()
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "\n")

	decoded, err := DecodeReference(raw)
	require.NoError(t, err)
	assert.Equal(t, ref, *decoded)

	again, err := decoded.Marshal()
	require.NoError(t, err)
	assert.Equal(t, raw, again, "serialization is deterministic")

	tests := []struct {
		name   string
		mutate func(string) string
	}{
		{"unknown field", func(s string) string { return strings.Replace(s, `{"`, `{"extra":1,"`, 1) }},
		{"not canonical (whitespace)", func(s string) string { return strings.Replace(s, `,"`, `, "`, 1) }},
		{"truncated", func(s string) string { return s[:len(s)-2] }},
		{"wrong schema", func(s string) string { return strings.Replace(s, `"schema_version":1`, `"schema_version":2`, 1) }},
		{"garbage", func(string) string { return "not json" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeReference([]byte(tt.mutate(string(raw))))
			require.Error(t, err)
			require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
		})
	}
}

func TestReference_ValidateRejectsInconsistentStates(t *testing.T) {
	base := NewPendingReference("team/app", "1.0.0", "op-1", digestA, nil)
	completed := base.Completed(digestB)
	tests := []struct {
		name   string
		ref    func() Reference
		wantOK bool
	}{
		{"pending without record", func() Reference { return base }, true},
		{"completed", func() Reference { return completed }, true},
		{"pending naming a record", func() Reference { r := base; r.RecordDigest = digestB; return r }, false},
		{"completed without a record", func() Reference { r := completed; r.RecordDigest = ""; return r }, false},
		{"completed with a prior", func() Reference { r := completed; r.PriorRecordDigest = digestA; return r }, false},
		{"zero generation", func() Reference { r := base; r.Generation = 0; return r }, false},
		{"missing operation", func() Reference { r := base; r.OperationID = ""; return r }, false},
		{"unknown state", func() Reference { r := base; r.State = "weird"; return r }, false},
		{"bad intent digest", func() Reference { r := base; r.IntentDigest = "sha256:zz"; return r }, false},
		{"invalid name", func() Reference { r := base; r.Name = "../x"; return r }, false},
		{"invalid version", func() Reference { r := base; r.Version = ""; return r }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := tt.ref()
			err := ref.Validate()
			if tt.wantOK {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
}

func sampleRepresentation() Representation {
	return Representation{
		SchemaVersion: SchemaVersion, RepositoryKind: artifact.RepositoryKindLocalArchive, Name: "team/app", Version: "1.0.0",
		RecordDigest: digestA, OperationID: "op-1", Packaging: artifact.PackagingTgz, Location: "2026/10/30/team/app/1.0.0.tgz",
		Payload:              []ocispec.Descriptor{{MediaType: artifact.MediaTypeTarGzip, Digest: digestB, Size: 10}},
		Documents:            []ocispec.Descriptor{},
		RepresentationDigest: digestB,
	}
}

func TestRepresentation_PersistsResolvedLocation(t *testing.T) {
	rep := sampleRepresentation()
	raw, err := rep.Marshal()
	require.NoError(t, err)
	decoded, err := DecodeRepresentation(raw)
	require.NoError(t, err)
	assert.Equal(t, rep.Location, decoded.Location, "the dated location survives serialization verbatim")
	assert.Equal(t, rep.Payload, decoded.Payload)
	assert.Equal(t, []string{rep.Location, rep.Location + ".metadata.json", rep.Location + ".docs"}, decoded.Claim().Paths)

	converted := decoded.Artifact("/abs/root/" + rep.Location)
	assert.Equal(t, artifact.PackagingTgz, converted.Packaging)
	assert.Equal(t, "/abs/root/"+rep.Location, converted.Locator)
	assert.Equal(t, digestA, converted.RecordDigest)
	converted.Payload[0].Size = 99
	assert.EqualValues(t, 10, decoded.Payload[0].Size, "the conversion copies the descriptors")
}

func TestRepresentation_RejectsUnsafeStoredLocation(t *testing.T) {
	for _, location := range []string{"", "../escape", "/abs", "a/../b", ".atmos-artifacts/x", "a//b", "a/"} {
		rep := sampleRepresentation()
		rep.Location = location
		_, err := rep.Marshal()
		require.Error(t, err, "%q", location)
	}
	missing := sampleRepresentation()
	missing.Payload = nil
	_, err := missing.Marshal()
	require.Error(t, err)
}

func sampleOperation() Operation {
	return Operation{
		SchemaVersion: SchemaVersion, RepositoryKind: artifact.RepositoryKindLocalDir, OperationID: "op/1", Name: "team/app", Version: "1.0.0",
		IntentDigest: digestA, RecordDigest: digestB, CapturedAt: artifact.NewTimestamp(time.Date(2026, 10, 30, 23, 59, 59, 0, time.UTC)),
		Packaging: artifact.PackagingNone, Location: "2026/10/30/team/app/1.0.0", ExpectedPrior: digestC,
		Prior:   &Prior{RecordDigest: digestC, Location: "2026/10/29/team/app/1.0.0"},
		Payload: []ocispec.Descriptor{}, Documents: []ocispec.Descriptor{}, Owner: "host:1",
	}
}

func TestOperation_JournalRoundTrip(t *testing.T) {
	op := sampleOperation()
	raw, err := op.Marshal()
	require.NoError(t, err)
	decoded, err := DecodeOperation(raw)
	require.NoError(t, err)
	assert.Equal(t, op.CapturedAt.Time(), decoded.CapturedAt.Time(), "the captured time is journaled")
	assert.Equal(t, op.Location, decoded.Location)
	assert.Equal(t, op.Prior, decoded.Prior)
	assert.Equal(t, op.ExpectedPrior, decoded.ExpectedPrior)

	claim := decoded.Claim()
	assert.Equal(t, ReferenceKey("team/app", "1.0.0"), claim.ReferenceKey)
	assert.Equal(t, "op/1", claim.OperationID)
	assert.Equal(t, []string{"2026/10/30/team/app/1.0.0"}, claim.Paths)

	broken := sampleOperation()
	broken.CapturedAt = artifact.Timestamp{}
	_, err = broken.Marshal()
	require.Error(t, err, "the captured time is required")
	broken = sampleOperation()
	broken.Prior = &Prior{}
	_, err = broken.Marshal()
	require.Error(t, err)
}

func TestEvidence_EnvelopeBindsFileNameToContent(t *testing.T) {
	subject := artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: digestA, MediaType: artifact.MediaTypeConfig, Size: 12}
	payload := &artifact.EvidencePayload{Format: artifact.EvidenceSigstoreBundle, MediaType: "application/json", Data: []byte(`{"bundle":true}`)}
	evidence, err := NewEvidence(subject, payload)
	require.NoError(t, err)
	assert.Equal(t, digest.FromBytes(payload.Data), evidence.Digest())

	raw, err := evidence.Marshal()
	require.NoError(t, err)
	decoded, err := DecodeEvidence(raw, evidence.Digest())
	require.NoError(t, err)
	assert.Equal(t, payload.Data, decoded.Data)

	blob := decoded.Blob("loc")
	assert.Equal(t, subject, blob.Evidence.Subject)
	assert.Equal(t, evidence.Digest(), blob.Evidence.Descriptor.Digest)
	assert.EqualValues(t, len(payload.Data), blob.Evidence.Descriptor.Size)
	assert.Equal(t, "loc", blob.Evidence.Locator)
	blob.Data[0] = 'X'
	assert.Equal(t, byte('{'), decoded.Data[0], "Blob copies the bytes")

	_, err = DecodeEvidence(raw, digestB)
	require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed, "a file whose name disagrees with its content is rejected")

	for _, bad := range []*artifact.EvidencePayload{
		{Format: "", MediaType: "x", Data: []byte("x")},
		{Format: artifact.EvidenceSigstoreBundle, MediaType: "", Data: []byte("x")},
		{Format: artifact.EvidenceSigstoreBundle, MediaType: "x", Data: nil},
		{Format: artifact.EvidenceSigstoreBundle, MediaType: "x", Data: make([]byte, artifact.MaxEvidenceSize+1)},
	} {
		_, err := NewEvidence(subject, bad)
		require.Error(t, err)
	}
	_, err = NewEvidence(artifact.SignatureSubject{Kind: "bogus", Digest: digestA}, payload)
	require.Error(t, err)
}

func TestAdmit(t *testing.T) {
	completed := NewPendingReference("team/app", "1.0.0", "op-0", digestA, nil)
	completed = completed.Completed(digestB)
	pendingFirst := NewPendingReference("team/app", "1.0.0", "op-1", digestA, nil)
	pendingReplace := NewPendingReference("team/app", "1.0.0", "op-2", digestA, &completed)
	expectCurrent := &artifact.PinnedReference{ResolvedDigest: digestB}

	tests := []struct {
		name    string
		in      AdmitInput
		want    Admission
		outcome artifact.Outcome
		wantErr error
	}{
		{"create when absent", AdmitInput{IncomingRecord: digestC, IncomingIntent: digestA}, AdmitCreate, artifact.OutcomeCreated, nil},
		{"absent but a prior was expected", AdmitInput{IncomingRecord: digestC, ExpectedPrior: expectCurrent}, "", "", errUtils.ErrArtifactConflict},
		{"same record is unchanged", AdmitInput{Current: &completed, IncomingRecord: digestB, ExpectedPrior: expectCurrent}, AdmitUnchanged, artifact.OutcomeUnchanged, nil},
		{"same intent is unchanged", AdmitInput{Current: &completed, CurrentIntent: digestA, IncomingRecord: digestC, IncomingIntent: digestA, ExpectedPrior: expectCurrent}, AdmitUnchanged, artifact.OutcomeUnchanged, nil},
		{"unknown current intent is never unchanged", AdmitInput{Current: &completed, IncomingRecord: digestC, IncomingIntent: "", ExpectedPrior: expectCurrent}, "", "", errUtils.ErrArtifactConflict},
		{"different intent conflicts without force", AdmitInput{Current: &completed, CurrentIntent: digestC, IncomingRecord: digestA, IncomingIntent: digestA, ExpectedPrior: expectCurrent}, "", "", errUtils.ErrArtifactConflict},
		{"different intent replaces under force", AdmitInput{Current: &completed, CurrentIntent: digestC, IncomingRecord: digestA, IncomingIntent: digestA, ExpectedPrior: expectCurrent, Force: true}, AdmitReplace, artifact.OutcomeReplaced, nil},
		{"stale expected prior", AdmitInput{Current: &completed, IncomingRecord: digestC, ExpectedPrior: &artifact.PinnedReference{ResolvedDigest: digestC}, Force: true}, "", "", errUtils.ErrArtifactConflict},
		{"exists but writer expected none", AdmitInput{Current: &completed, IncomingRecord: digestC, Force: true}, "", "", errUtils.ErrArtifactConflict},
		{"same-intent pending first publication resumes", AdmitInput{Current: &pendingFirst, IncomingIntent: digestA}, AdmitResumeCreate, artifact.OutcomeCreated, nil},
		{"same-intent pending replacement resumes", AdmitInput{Current: &pendingReplace, IncomingIntent: digestA}, AdmitResumeReplace, artifact.OutcomeReplaced, nil},
		{"different-intent pending names the operation", AdmitInput{Current: &pendingFirst, IncomingIntent: digestC}, "", "", errUtils.ErrArtifactPublicationUnavailable},
		{"force never steals a different pending intent", AdmitInput{Current: &pendingReplace, IncomingIntent: digestC, Force: true}, "", "", errUtils.ErrArtifactPublicationUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Admit(&tt.in)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.outcome, got.Outcome())
			assert.Equal(t, tt.want == AdmitResumeCreate || tt.want == AdmitResumeReplace, got.Resuming())
		})
	}

	_, err := Admit(&AdmitInput{Current: &pendingFirst, IncomingIntent: digestC})
	require.ErrorContains(t, err, "op-1", "the recovery diagnostic includes the owning operation ID")
}
