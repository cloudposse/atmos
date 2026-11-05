package artifacttest

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/materialize"
	"github.com/cloudposse/atmos/pkg/artifact/produce"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Names and modes the suite uses for its fixtures.
const (
	appName       = "team/app"
	firstVersion  = "1.0.0"
	secondVersion = "2.0.0"
	filePerm      = 0o644
	execPerm      = 0o755
	pageSafety    = 100
)

// Harness configures [RunRepositoryConformance] for one backend.
type Harness struct {
	// New returns a fresh, empty repository. It is called once per subtest; the harness never
	// shares state between subtests. Register cleanup with t.Cleanup.
	New func(t *testing.T) artifact.Repository

	// Packaging is the payload packaging used for publications; empty uses the first packaging
	// the repository advertises.
	Packaging artifact.Packaging

	// Signer produces evidence the repository can store. Nil is allowed only for repositories that
	// advertise no evidence formats; the suite then asserts Sign fails with a capability error.
	Signer artifact.Signer

	// TrustedPolicy is satisfied by evidence the Signer produced. Required when Signer is set.
	TrustedPolicy artifact.VerificationPolicy

	// UntrustedPolicy is a policy that evidence from the Signer must NOT satisfy. Required when
	// Signer is set.
	UntrustedPolicy artifact.VerificationPolicy
}

// published is one artifact stored through the repository under test.
type published struct {
	prepared *artifact.PreparedArtifact
	result   *artifact.PublishResult
	ref      artifact.Ref
}

// RunRepositoryConformance runs the shared repository contract suite against h. Every backend
// calls it from its own tests; backend-specific differences are expressed through Capabilities,
// never by omitting an operation that claims success. Operations a repository does not support
// must return a capability error, which the suite asserts.
func RunRepositoryConformance(t *testing.T, h *Harness) {
	defer perf.Track(nil, "artifacttest.RunRepositoryConformance")()

	t.Helper()
	suite := &conformance{h: *h}
	t.Run("CapabilitiesAreStatic", suite.capabilities)
	t.Run("MultipleNamesAndVersions", suite.multipleNamesAndVersions)
	t.Run("PublishReadRoundTrip", suite.roundTrip)
	t.Run("MetadataAndDocumentsWithoutPayload", suite.metadataAndDocuments)
	t.Run("DigestReferences", suite.digestReferences)
	t.Run("Enumeration", suite.enumeration)
	t.Run("ForceReplacementNeverServesNewBytesUnderOldDigest", suite.replacement)
	t.Run("ConditionalPublish", suite.conditionalPublish)
	t.Run("Delete", suite.deletion)
	t.Run("Pin", suite.pin)
	t.Run("SignVerify", suite.signVerify)
	t.Run("Lock", suite.lock)
	t.Run("ReadOnlyOperationsReturnCapabilityErrors", suite.capabilityErrors)
}

type conformance struct {
	h Harness
}

func (c *conformance) packaging(repo artifact.Repository) artifact.Packaging {
	if c.h.Packaging != "" {
		return c.h.Packaging
	}
	caps := repo.Capabilities()
	if len(caps.Packagings) > 0 {
		return caps.Packagings[0]
	}
	return artifact.PackagingNone
}

// spec names and fills one artifact the suite publishes.
type spec struct {
	name    string
	version string
	content string
}

// publish prepares a file package and publishes it. The spec's content varies the payload.
func (c *conformance) publish(t *testing.T, repo artifact.Repository, sp spec, options artifact.PublishOptions) published {
	t.Helper()
	src := t.TempDir()
	fixtures.WriteFile(t, src, "data.txt", sp.content, filePerm)
	fixtures.WriteFile(t, src, "bin/run.sh", fixtures.HelperScript, execPerm)
	fixtures.WriteFile(t, src, fixtures.ReservedDir+"/README.md", "readme for "+sp.name+"\n", filePerm)
	prepared, err := produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), artifact.PrepareRequest{
		Source: src, Name: sp.name, Version: sp.version, Packaging: c.packaging(repo),
		Metadata:    artifact.Metadata{Labels: map[string]string{"suite": "conformance"}},
		OperationID: "op-" + sp.name + "-" + sp.version, PreparedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = prepared.Close() })
	result, err := repo.Publish(context.Background(), prepared, options)
	require.NoError(t, err)
	return published{prepared: prepared, result: result, ref: artifact.Ref{Name: sp.name, Version: sp.version}}
}

func (c *conformance) requireWritable(t *testing.T, repo artifact.Repository) {
	t.Helper()
	if !repo.Capabilities().Write {
		t.Skip("the repository is read-only; capability errors are asserted by the dedicated subtest")
	}
}

func (c *conformance) capabilities(t *testing.T) {
	repo := c.h.New(t)
	first, second := repo.Capabilities(), repo.Capabilities()
	assert.Equal(t, first, second, "Capabilities must be stable and need no network access")
	assert.NotEmpty(t, first.LockScopes, "every repository can coordinate at least host-scope writes")
}

func (c *conformance) multipleNamesAndVersions(t *testing.T) {
	repo := c.h.New(t)
	c.requireWritable(t, repo)
	items := []struct{ name, version string }{{appName, "1.0.0"}, {appName, "2.0.0"}, {"other/tool", "1.0.0"}}
	var all []published
	for _, item := range items {
		all = append(all, c.publish(t, repo, spec{name: item.name, version: item.version, content: item.name + item.version}, artifact.PublishOptions{}))
	}
	seen := map[digest.Digest]bool{}
	for _, item := range all {
		pinned, err := repo.Resolve(context.Background(), item.ref)
		require.NoError(t, err)
		inspection, err := repo.Inspect(context.Background(), pinned)
		require.NoError(t, err)
		require.NotNil(t, inspection.Record)
		assert.Equal(t, item.ref.Name, inspection.Record.Name)
		assert.Equal(t, item.ref.Version, inspection.Record.Version)
		assert.False(t, seen[pinned.ResolvedDigest], "distinct artifacts resolve to distinct digests")
		seen[pinned.ResolvedDigest] = true
	}
}

func (c *conformance) roundTrip(t *testing.T) {
	repo := c.h.New(t)
	c.requireWritable(t, repo)
	item := c.publish(t, repo, spec{name: appName, version: firstVersion, content: "round trip"}, artifact.PublishOptions{})
	pinned, err := repo.Resolve(context.Background(), item.ref)
	require.NoError(t, err)
	content, err := repo.Read(context.Background(), pinned)
	require.NoError(t, err)
	defer content.Close()
	require.NotNil(t, content.Record)

	out := filepath.Join(t.TempDir(), "out")
	require.NoError(t, materialize.Materialize(context.Background(), content, out))
	state, err := materialize.InspectDestination(out, content.Record)
	require.NoError(t, err)
	assert.Equal(t, materialize.DestinationMatches, state, "pulled content matches the inventory, modes included")
}

func (c *conformance) metadataAndDocuments(t *testing.T) {
	repo := c.h.New(t)
	c.requireWritable(t, repo)
	item := c.publish(t, repo, spec{name: appName, version: firstVersion, content: "docs"}, artifact.PublishOptions{})
	pinned, err := repo.Resolve(context.Background(), item.ref)
	require.NoError(t, err)

	stream, err := repo.OpenDocument(context.Background(), pinned, artifact.DocumentReadme)
	require.NoError(t, err)
	data, err := io.ReadAll(stream.Reader)
	require.NoError(t, err)
	require.NoError(t, stream.Reader.Close())
	assert.Equal(t, "readme for team/app\n", string(data))
	assert.Equal(t, digest.FromBytes(data), stream.Document.Digest)

	_, err = repo.OpenDocument(context.Background(), pinned, artifact.DocumentChangelog)
	require.ErrorIs(t, err, errUtils.ErrArtifactDocumentMissing, "a missing optional document is a typed error")
}

func (c *conformance) digestReferences(t *testing.T) {
	repo := c.h.New(t)
	c.requireWritable(t, repo)
	item := c.publish(t, repo, spec{name: appName, version: firstVersion, content: "digest"}, artifact.PublishOptions{})
	byVersion, err := repo.Resolve(context.Background(), item.ref)
	require.NoError(t, err)

	byDigest, err := repo.Resolve(context.Background(), artifact.Ref{Name: item.ref.Name, Digest: byVersion.ResolvedDigest})
	require.NoError(t, err)
	assert.Equal(t, byVersion.ResolvedDigest, byDigest.ResolvedDigest)

	_, err = repo.Resolve(context.Background(), artifact.Ref{Name: "other/name", Digest: byVersion.ResolvedDigest})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "a digest read checks the Record's embedded name")

	_, err = repo.Resolve(context.Background(), artifact.Ref{Name: item.ref.Name, Version: "9.9.9"})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
}

func (c *conformance) enumeration(t *testing.T) {
	repo := c.h.New(t)
	c.requireWritable(t, repo)
	caps := repo.Capabilities()
	for _, item := range []struct{ name, version string }{{"b/app", "1"}, {"a/app", "2"}, {"a/app", "1"}} {
		c.publish(t, repo, spec{name: item.name, version: item.version, content: item.name + item.version}, artifact.PublishOptions{})
	}
	if !caps.EnumerateCollection {
		_, err := repo.List(context.Background(), artifact.ListQuery{})
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
		return
	}
	var names []string
	cursor := ""
	for pages := 0; pages < pageSafety; pages++ {
		page, err := repo.List(context.Background(), artifact.ListQuery{Cursor: cursor})
		require.NoError(t, err)
		for index := range page.Entries {
			names = append(names, page.Entries[index].Name+":"+page.Entries[index].Version)
		}
		if page.NextCursor == "" {
			break
		}
		require.NotEqual(t, cursor, page.NextCursor, "the cursor must advance")
		cursor = page.NextCursor
	}
	assert.Equal(t, []string{"a/app:1", "a/app:2", "b/app:1"}, names, "lexical order, every entry exactly once")

	if caps.EnumerateNamed {
		page, err := repo.List(context.Background(), artifact.ListQuery{Name: "a/app", PageSize: pageSafety})
		require.NoError(t, err)
		require.Len(t, page.Entries, 2)
		assert.Equal(t, "1", page.Entries[0].Version)
		assert.Equal(t, "2", page.Entries[1].Version)
	}
}

func (c *conformance) replacement(t *testing.T) {
	repo := c.h.New(t)
	c.requireWritable(t, repo)
	first := c.publish(t, repo, spec{name: appName, version: firstVersion, content: "first"}, artifact.PublishOptions{})
	oldPinned, err := repo.Resolve(context.Background(), first.ref)
	require.NoError(t, err)

	second := c.publishForce(t, repo, first.ref, "second", &oldPinned)
	newPinned, err := repo.Resolve(context.Background(), first.ref)
	require.NoError(t, err)
	assert.NotEqual(t, oldPinned.ResolvedDigest, newPinned.ResolvedDigest)
	assert.Equal(t, artifact.OutcomeReplaced, second.result.Outcome)

	content, err := repo.Read(context.Background(), oldPinned)
	if err != nil {
		assert.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "retired bytes report not found")
		return
	}
	defer content.Close()
	require.NotNil(t, content.Record)
	recordDigest, err := content.Record.RecordDigest()
	require.NoError(t, err)
	assert.Equal(t, oldPinned.RecordDigest, recordDigest, "an old digest never serves the replacement's record")
}

func (c *conformance) publishForce(t *testing.T, repo artifact.Repository, ref artifact.Ref, content string, prior *artifact.PinnedReference) published {
	return c.publish(t, repo, spec{name: ref.Name, version: ref.Version, content: content}, artifact.PublishOptions{Force: true, ExpectedPrior: prior})
}

func (c *conformance) conditionalPublish(t *testing.T) {
	repo := c.h.New(t)
	c.requireWritable(t, repo)
	if !repo.Capabilities().ConditionalUpdate {
		t.Skip("the repository does not advertise conditional updates")
	}
	ref := artifact.Ref{Name: appName, Version: "1.0.0"}
	c.publish(t, repo, spec{name: ref.Name, version: ref.Version, content: "base"}, artifact.PublishOptions{})

	stale := artifact.PinnedReference{Ref: ref, ResolvedDigest: digest.FromString("stale")}
	src := t.TempDir()
	fixtures.WriteFile(t, src, "data.txt", "changed", filePerm)
	prepared, err := produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), artifact.PrepareRequest{
		Source: src, Name: ref.Name, Version: ref.Version, Packaging: c.packaging(repo),
		OperationID: "op-stale", PreparedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	})
	require.NoError(t, err)
	defer prepared.Close()
	_, err = repo.Publish(context.Background(), prepared, artifact.PublishOptions{Force: true, ExpectedPrior: &stale})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "a stale expected-prior reference is a conflict, not a silent overwrite")

	_, err = repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "publishing over an existing reference expecting none is a conflict")
}

func (c *conformance) deletion(t *testing.T) {
	repo := c.h.New(t)
	c.requireWritable(t, repo)
	keep := c.publish(t, repo, spec{name: appName, version: firstVersion, content: "keep"}, artifact.PublishOptions{})
	drop := c.publish(t, repo, spec{name: appName, version: secondVersion, content: "drop"}, artifact.PublishOptions{})
	pinned, err := repo.Resolve(context.Background(), drop.ref)
	require.NoError(t, err)

	caps := repo.Capabilities()
	if caps.Delete == artifact.DeleteNone || caps.Delete == "" {
		_, err := repo.Delete(context.Background(), pinned, artifact.DeleteOptions{Scope: caps.Delete})
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
		return
	}
	result, err := repo.Delete(context.Background(), pinned, artifact.DeleteOptions{ExpectedCurrent: &pinned, Scope: caps.Delete})
	require.NoError(t, err)
	assert.NotEmpty(t, result.Deleted)
	_, err = repo.Resolve(context.Background(), drop.ref)
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, err = repo.Resolve(context.Background(), keep.ref)
	require.NoError(t, err, "deletion is scoped to the selected artifact")
}

func (c *conformance) pin(t *testing.T) {
	repo := c.h.New(t)
	c.requireWritable(t, repo)
	item := c.publish(t, repo, spec{name: appName, version: firstVersion, content: "pin"}, artifact.PublishOptions{})
	resolved, err := repo.Resolve(context.Background(), item.ref)
	require.NoError(t, err)
	pinned, err := repo.Pin(context.Background(), item.ref)
	require.NoError(t, err)
	assert.Equal(t, resolved.ResolvedDigest, pinned.ResolvedDigest)
	assert.Equal(t, resolved.RecordDigest, pinned.RecordDigest)

	_, err = repo.Pin(context.Background(), artifact.Ref{Name: "missing/artifact", Version: "1"})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
}

func (c *conformance) signVerify(t *testing.T) {
	repo := c.h.New(t)
	c.requireWritable(t, repo)
	item := c.publish(t, repo, spec{name: appName, version: firstVersion, content: "sign"}, artifact.PublishOptions{})
	pinned, err := repo.Resolve(context.Background(), item.ref)
	require.NoError(t, err)
	inspection, err := repo.Inspect(context.Background(), pinned)
	require.NoError(t, err)
	canonical, err := inspection.Record.Canonical()
	require.NoError(t, err)
	subject := artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: digest.FromBytes(canonical), MediaType: artifact.MediaTypeConfig, Size: int64(len(canonical))}

	caps := repo.Capabilities()
	if c.h.Signer == nil {
		require.Empty(t, caps.EvidenceFormats, "a repository that stores evidence needs a Signer in the harness")
		_, err := repo.Sign(context.Background(), subject, &unsupportedSigner{}, artifact.SignOptions{})
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
		return
	}
	require.NotEmpty(t, c.h.TrustedPolicy.TrustSets, "TrustedPolicy is required with a Signer")

	unsigned, err := repo.Verify(context.Background(), subject, c.h.TrustedPolicy)
	require.Error(t, err, "no evidence never verifies")
	if unsigned != nil {
		assert.NotEqual(t, artifact.VerificationVerified, unsigned.Status)
	}

	evidence, err := repo.Sign(context.Background(), subject, c.h.Signer, artifact.SignOptions{OperationID: "sign-op"})
	require.NoError(t, err)
	require.NotEmpty(t, evidence)
	assert.Equal(t, subject.Digest, evidence[0].Subject.Digest)

	verified, err := repo.Verify(context.Background(), subject, c.h.TrustedPolicy)
	require.NoError(t, err)
	assert.Equal(t, artifact.VerificationVerified, verified.Status)
	assert.Equal(t, subject.Digest, verified.Subject.Digest)
	assert.NotEmpty(t, verified.EvidenceDigests)

	rejected, err := repo.Verify(context.Background(), subject, c.h.UntrustedPolicy)
	require.Error(t, err, "evidence from an untrusted signer never satisfies the policy")
	if rejected != nil {
		assert.NotEqual(t, artifact.VerificationVerified, rejected.Status)
	}

	other := subject
	other.Digest = digest.FromString("another subject")
	if mismatched, err := repo.Verify(context.Background(), other, c.h.TrustedPolicy); err == nil && mismatched != nil {
		assert.NotEqual(t, artifact.VerificationVerified, mismatched.Status, "evidence is bound to its subject digest")
	}
}

// unsupportedSigner is a signer for a format no repository under test advertises.
type unsupportedSigner struct{}

func (*unsupportedSigner) Formats() []artifact.EvidenceFormat {
	defer perf.Track(nil, "artifacttest.unsupportedSigner.Formats")()

	return []artifact.EvidenceFormat{"unsupported"}
}

func (*unsupportedSigner) Sign(context.Context, artifact.SignatureSubject) (*artifact.EvidencePayload, error) {
	defer perf.Track(nil, "artifacttest.unsupportedSigner.Sign")()

	return &artifact.EvidencePayload{Format: "unsupported", Data: []byte("x")}, nil
}

func (c *conformance) lock(t *testing.T) {
	repo := c.h.New(t)
	ctx := context.Background()
	request := artifact.LockRequest{Resource: "team/app:1.0.0", Owner: "owner-a", OperationID: "op-a", Required: artifact.LockScopeHost}
	first, err := repo.Lock(ctx, request)
	require.NoError(t, err)
	assert.Equal(t, request.Resource, first.Resource())
	assert.Equal(t, "owner-a", first.Owner())
	assert.NotEmpty(t, first.Token())
	assert.True(t, first.Scope().Satisfies(artifact.LockScopeHost))

	blocked, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	second := request
	second.Owner = "owner-b"
	handle, err := repo.Lock(blocked, second)
	require.Error(t, err, "a competing owner cannot take a held lock")
	assert.Nil(t, handle)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errUtils.ErrArtifactConflict), "waiting is bounded by the context or rejected as a conflict: %v", err)

	require.NoError(t, first.Release(ctx))
	again, err := repo.Lock(ctx, second)
	require.NoError(t, err, "a canceled acquisition leaked no ownership")
	require.NoError(t, again.Release(ctx))

	caps := repo.Capabilities()
	if !caps.HasLockScope(artifact.LockScopeRepository) {
		request.Required = artifact.LockScopeRepository
		_, err := repo.Lock(ctx, request)
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "an insufficient guarantee is an error, never a silent downgrade")
	}
}

func (c *conformance) capabilityErrors(t *testing.T) {
	repo := c.h.New(t)
	caps := repo.Capabilities()
	if caps.Write {
		t.Skip("the repository is writable; read-only behavior is not applicable")
	}
	src := t.TempDir()
	fixtures.WriteFile(t, src, "a.txt", "a", filePerm)
	prepared, err := produce.New(produce.WithStagingRoot(t.TempDir())).Prepare(context.Background(), artifact.PrepareRequest{
		Source: src, Name: "x/y", Version: "1", Packaging: c.packaging(repo), OperationID: "op", PreparedAt: time.Now(),
	})
	require.NoError(t, err)
	defer prepared.Close()
	_, err = repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
}
