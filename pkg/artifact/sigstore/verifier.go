package sigstore

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"os"

	"github.com/opencontainers/go-digest"
	"github.com/sigstore/sigstore-go/pkg/root"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/signature"
)

// maxDiagnostics bounds the diagnostics one verification reports.
const maxDiagnostics = 16

// RootLoader supplies independently provisioned trust material. In offline mode it may use only
// cached trust and must fail otherwise.
type RootLoader func(ctx context.Context, offline bool) (root.TrustedMaterial, error)

// Verifier implements [artifact.Verifier] for Sigstore bundle evidence.
type Verifier struct {
	loadRoot RootLoader
	readFile func(path string) ([]byte, error)
	keys     map[string]crypto.PublicKey
}

var _ artifact.Verifier = (*Verifier)(nil)

// VerifierOption configures a [Verifier].
type VerifierOption func(*Verifier)

// WithRootLoader injects the trust-root loader. The default is [signature.LoadPublicRoot], the
// TUF-authenticated Sigstore public-good root.
func WithRootLoader(loader func(ctx context.Context, offline bool) (root.TrustedMaterial, error)) VerifierOption {
	defer perf.Track(nil, "sigstore.WithRootLoader")()

	return func(v *Verifier) { v.loadRoot = loader }
}

// WithTrustedKey registers an operator-provisioned public key. A trust set refers to it by naming
// hint in its Keys list, exactly like a key file path. The bundle may identify the key by hint or
// by the SHA-256 fingerprint of its PKIX encoding.
func WithTrustedKey(hint string, key crypto.PublicKey) VerifierOption {
	defer perf.Track(nil, "sigstore.WithTrustedKey")()

	return func(v *Verifier) { v.keys[hint] = key }
}

// WithKeyFileReader injects how trusted key files named by a trust set are read. The default
// reads the operating system file.
func WithKeyFileReader(read func(path string) ([]byte, error)) VerifierOption {
	defer perf.Track(nil, "sigstore.WithKeyFileReader")()

	return func(v *Verifier) { v.readFile = read }
}

// NewVerifier returns a Verifier.
func NewVerifier(opts ...VerifierOption) *Verifier {
	defer perf.Track(nil, "sigstore.NewVerifier")()

	v := &Verifier{loadRoot: signature.LoadPublicRoot, readFile: os.ReadFile, keys: map[string]crypto.PublicKey{}}
	for _, opt := range opts {
		if opt != nil {
			opt(v)
		}
	}
	if v.loadRoot == nil {
		v.loadRoot = signature.LoadPublicRoot
	}
	if v.readFile == nil {
		v.readFile = os.ReadFile
	}
	return v
}

// Formats implements [artifact.Verifier].
func (v *Verifier) Formats() []artifact.EvidenceFormat {
	defer perf.Track(nil, "sigstore.Verifier.Formats")()

	return []artifact.EvidenceFormat{artifact.EvidenceSigstoreBundle, artifact.EvidencePublicKeyBundle}
}

// session holds the state of one Verify call.
type session struct {
	verifier *Verifier
	subject  artifact.SignatureSubject
	policy   artifact.VerificationPolicy
	digests  []string
	loader   RootLoader
	result   *artifact.VerificationResult
	dropped  int
}

// candidate is evidence that passed the structural screen.
type candidate struct {
	blob   *artifact.EvidenceBlob
	format artifact.EvidenceFormat
	digest digest.Digest
}

// Verify implements [artifact.Verifier]. Every trust set must be satisfied by at least one valid
// evidence item. Invalid, unrelated, or unsupported evidence is reported (bounded) and ignored;
// it can never override matching valid evidence, and a public-key bundle never satisfies an
// identity requirement.
func (v *Verifier) Verify(ctx context.Context, subject artifact.SignatureSubject, evidence []artifact.EvidenceBlob, policy artifact.VerificationPolicy) (*artifact.VerificationResult, error) {
	defer perf.Track(nil, "sigstore.Verifier.Verify")()

	result := &artifact.VerificationResult{Subject: subject, Status: artifact.VerificationFailed}
	if fingerprint, err := policy.Fingerprint(); err == nil {
		result.PolicyFingerprint = fingerprint
	}
	if !policy.HasTrustConstraints() {
		result.Status = artifact.VerificationUnchecked
		return result, nil
	}
	s := &session{verifier: v, subject: subject, policy: policy, result: result}
	s.digests = []string{subject.Digest.String(), digest.FromBytes(SubjectStatement(subject)).String()}
	s.loader = memoizeLoader(v.loadRoot)
	if err := ctx.Err(); err != nil {
		cancelled := errors.Join(errUtils.ErrArtifactVerificationFailed, err)
		return s.fail(), cancelled
	}
	candidates := s.screen(evidence)
	return s.evaluate(ctx, candidates)
}

// evaluate checks every trust set against the candidates.
func (s *session) evaluate(ctx context.Context, candidates []candidate) (*artifact.VerificationResult, error) {
	var claims []artifact.SignerClaim
	verified := map[int]bool{}
	for index := range s.policy.TrustSets {
		set := &s.policy.TrustSets[index]
		if len(set.Identities) == 0 && len(set.Keys) == 0 {
			continue
		}
		found, err := s.satisfy(ctx, set, candidates)
		if err != nil {
			wrapped := errors.Join(errUtils.ErrArtifactVerificationFailed, err)
			return s.fail(), wrapped
		}
		claim, matched := found.claim, found.index
		if matched < 0 {
			s.add(artifact.SeverityError, "no-permitted-signature", fmt.Sprintf("no valid permitted signature satisfies trust set %d", index+1))
			for _, failure := range found.failures {
				s.add(artifact.SeverityWarning, "verification-failed", failure)
			}
			wrapped := fmt.Errorf("%w: no valid permitted signature for %s", errUtils.ErrArtifactVerificationFailed, s.subject.Digest)
			return s.fail(), wrapped
		}
		verified[matched] = true
		if !containsClaim(claims, claim) {
			claims = append(claims, claim)
		}
	}
	for index := range candidates {
		if verified[index] {
			s.result.EvidenceDigests = append(s.result.EvidenceDigests, candidates[index].digest)
		}
	}
	s.result.Claims = claims
	s.result.Status = artifact.VerificationVerified
	s.flush()
	return s.result, nil
}

// satisfaction is the outcome of checking one trust set: the matched candidate index (-1 when none
// matched), its claim, and the reasons other candidates failed.
type satisfaction struct {
	claim    artifact.SignerClaim
	index    int
	failures []string
}

// satisfy looks for one candidate that satisfies the set. It returns a non-nil error only for
// cancellation.
func (s *session) satisfy(ctx context.Context, set *artifact.TrustSet, candidates []candidate) (satisfaction, error) {
	var keys *keySet
	var keyErr error
	if len(set.Keys) > 0 {
		keys, keyErr = s.verifier.resolveKeys(set.Keys)
	}
	var failures []string
	if keyErr != nil {
		failures = append(failures, keyErr.Error())
	}
	for index := range candidates {
		claim, err := s.authenticate(ctx, set, keys, &candidates[index])
		if err == nil {
			return satisfaction{claim: claim, index: index}, nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return satisfaction{index: -1}, ctxErr
		}
		if !errors.Is(err, errNotApplicable) {
			failures = append(failures, err.Error())
		}
	}
	return satisfaction{index: -1, failures: failures}, nil
}

// errNotApplicable marks evidence that cannot count toward a trust set at all, for example a
// public-key bundle offered to an identity-only set.
var errNotApplicable = errors.New("evidence does not apply to this trust set")

// authenticate verifies one candidate against one trust set through the verifier that matches the
// evidence format. Identities are checked only for certificate bundles and keys only for
// public-key bundles.
func (s *session) authenticate(ctx context.Context, set *artifact.TrustSet, keys *keySet, c *candidate) (artifact.SignerClaim, error) {
	switch c.format {
	case artifact.EvidenceSigstoreBundle:
		if len(set.Identities) == 0 {
			return artifact.SignerClaim{}, errNotApplicable
		}
		return s.verifyKeyless(ctx, set.Identities, c)
	case artifact.EvidencePublicKeyBundle:
		if keys == nil || len(keys.keys) == 0 {
			return artifact.SignerClaim{}, errNotApplicable
		}
		return s.verifyKey(ctx, keys, c)
	default:
		return artifact.SignerClaim{}, errNotApplicable
	}
}

func (s *session) verifyKeyless(ctx context.Context, trusted []artifact.TrustedIdentity, c *candidate) (artifact.SignerClaim, error) {
	identities := make([]signature.Identity, 0, len(trusted))
	for _, identity := range trusted {
		identities = append(identities, signature.Identity{Identity: identity.Identity, Issuer: identity.Issuer})
	}
	verifier := signature.Verifier{LoadRoot: s.loader}
	var failure error
	for _, subjectDigest := range s.digests {
		result, err := verifier.Verify(ctx, c.blob.Data, subjectDigest, identities, s.policy.Offline)
		if err == nil {
			return artifact.SignerClaim{Identity: result.Identity, Issuer: result.Issuer}, nil
		}
		if ctx.Err() != nil {
			return artifact.SignerClaim{}, err
		}
		failure = err
	}
	return artifact.SignerClaim{}, fmt.Errorf("%s: %w", c.digest, failure)
}

func (s *session) verifyKey(ctx context.Context, keys *keySet, c *candidate) (artifact.SignerClaim, error) {
	verifier := signature.KeyVerifier{Keys: keys.trustedKeys(), LoadRoot: s.loader}
	var failure error
	for _, subjectDigest := range s.digests {
		result, err := verifier.Verify(ctx, c.blob.Data, subjectDigest, s.policy.Offline)
		if err == nil {
			return artifact.SignerClaim{KeyID: keys.fingerprintFor(result.Hint)}, nil
		}
		if ctx.Err() != nil {
			return artifact.SignerClaim{}, err
		}
		failure = err
	}
	return artifact.SignerClaim{}, fmt.Errorf("%s: %w", c.digest, failure)
}

// resolveKeys loads the keys a trust set names: a registered programmatic key by name, otherwise a
// PEM public-key file. Any unresolvable key is an error, so a typo never silently narrows trust
// into acceptance of nothing while hiding why. Keys that do resolve are still returned.
func (v *Verifier) resolveKeys(refs []string) (*keySet, error) {
	set := &keySet{}
	var errs []error
	for _, ref := range refs {
		if key, ok := v.keys[ref]; ok {
			resolved, err := programmaticKey(ref, key)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			set.keys = append(set.keys, resolved)
			continue
		}
		raw, err := v.readFile(ref)
		if err != nil {
			errs = append(errs, fmt.Errorf("%w: read trusted key %q: %w", errUtils.ErrArtifactConfigInvalid, ref, err))
			continue
		}
		resolved, err := parsePublicKeyFile(ref, raw)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		set.keys = append(set.keys, resolved)
	}
	return set, errors.Join(errs...)
}

// screen applies the structural checks and reports what it ignores.
func (s *session) screen(evidence []artifact.EvidenceBlob) []candidate {
	var out []candidate
	for index := range evidence {
		blob := &evidence[index]
		format, ok := s.screenOne(blob)
		if !ok {
			continue
		}
		out = append(out, candidate{blob: blob, format: format, digest: evidenceDigest(blob)})
	}
	return out
}

func evidenceDigest(blob *artifact.EvidenceBlob) digest.Digest {
	if blob.Evidence.Descriptor.Digest != "" {
		return blob.Evidence.Descriptor.Digest
	}
	return digest.FromBytes(blob.Data)
}

func (s *session) screenOne(blob *artifact.EvidenceBlob) (artifact.EvidenceFormat, bool) {
	held := blob.Evidence
	switch {
	case held.Subject.Kind != s.subject.Kind || held.Subject.Digest != s.subject.Digest:
		s.add(artifact.SeverityWarning, "subject-mismatch", "evidence bound to a different subject was ignored")
	case held.Format != artifact.EvidenceSigstoreBundle && held.Format != artifact.EvidencePublicKeyBundle:
		s.add(artifact.SeverityWarning, "unsupported-evidence", fmt.Sprintf("evidence in unsupported format %q was ignored", held.Format))
	case held.Descriptor.Digest != "" && held.Descriptor.Digest != digest.FromBytes(blob.Data):
		s.add(artifact.SeverityWarning, "malformed-evidence", "evidence bytes do not match their descriptor and were ignored")
	default:
		return s.classify(blob)
	}
	return "", false
}

func (s *session) classify(blob *artifact.EvidenceBlob) (artifact.EvidenceFormat, bool) {
	sniffed, _, err := EvidenceFormatOf(blob.Data)
	if err != nil {
		s.add(artifact.SeverityWarning, "malformed-evidence", "malformed evidence was ignored: "+err.Error())
		return "", false
	}
	if sniffed != blob.Evidence.Format {
		s.add(artifact.SeverityWarning, "format-mismatch", fmt.Sprintf("evidence declared as %q carries %q verification material and was ignored", blob.Evidence.Format, sniffed))
		return "", false
	}
	return sniffed, true
}

// add records a bounded diagnostic.
func (s *session) add(severity artifact.Severity, code, message string) {
	if len(s.result.Diagnostics) >= maxDiagnostics {
		s.dropped++
		return
	}
	s.result.Diagnostics = append(s.result.Diagnostics, artifact.Diagnostic{Severity: severity, Code: code, Message: message})
}

// flush appends the truncation summary when diagnostics were dropped.
func (s *session) flush() {
	if s.dropped > 0 {
		s.result.Diagnostics = append(s.result.Diagnostics, artifact.Diagnostic{
			Severity: artifact.SeverityWarning, Code: "diagnostics-truncated",
			Message: fmt.Sprintf("%d further diagnostics were omitted", s.dropped),
		})
		s.dropped = 0
	}
}

// fail finalizes a failed result.
func (s *session) fail() *artifact.VerificationResult {
	s.result.Status = artifact.VerificationFailed
	s.result.Claims = nil
	s.result.EvidenceDigests = nil
	s.flush()
	return s.result
}

func containsClaim(claims []artifact.SignerClaim, claim artifact.SignerClaim) bool {
	for _, have := range claims {
		if have == claim {
			return true
		}
	}
	return false
}

// memoizeLoader loads trust material at most once per offline mode during one verification.
func memoizeLoader(load RootLoader) RootLoader {
	type loaded struct {
		material root.TrustedMaterial
		err      error
	}
	cache := map[bool]*loaded{}
	return func(ctx context.Context, offline bool) (root.TrustedMaterial, error) {
		if hit, ok := cache[offline]; ok {
			return hit.material, hit.err
		}
		material, err := load(ctx, offline)
		if ctx.Err() == nil {
			cache[offline] = &loaded{material: material, err: err}
		}
		return material, err
	}
}
