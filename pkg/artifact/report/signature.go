package report

import (
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
)

// Signature statements. Signing and verifying are different claims, and a checksum is not publisher
// authentication, so each statement says what the status does and does not prove.
const (
	statementSigned = "Signature evidence was stored for this subject. Signing alone does not authenticate the signer: " +
		"verify the artifact against a trust policy to do that."
	statementVerified = "A valid signature from a signer permitted by the trust policy covers this subject, " +
		"so the publisher is authenticated."
	statementUnchecked = "No trust configuration was supplied, so no signer was authenticated and nothing is claimed. " +
		"A matching digest proves the content is intact, not who published it."
	statementFailed = "Required verification did not succeed. The artifact must not be trusted."
)

// signatureOf builds the signature section of a sign, verify, or policy-constrained inspect result.
// It returns nil for an operation that neither signed nor verified, so a report never implies a
// signature check that did not happen.
func signatureOf(action artifact.Action, result *invoke.Result) *Signature {
	switch {
	case result.Verification != nil:
		return verificationSignature(result)
	case action == artifact.ActionSign && (result.Subject != nil || len(result.Evidence) > 0):
		return signedSignature(result)
	default:
		return nil
	}
}

// signedSignature reports the evidence a sign operation stored. The status is "signed", never
// "verified": the evidence is untrusted until a verification applies a trust policy.
func signedSignature(result *invoke.Result) *Signature {
	signature := &Signature{Status: "signed", Statement: statementSigned, Evidence: evidenceOf(result.Evidence)}
	applySubject(signature, result.Subject)
	return signature
}

// verificationSignature reports an authenticated verification result: status, subject, policy,
// and the verified signer claims.
func verificationSignature(result *invoke.Result) *Signature {
	verification := result.Verification
	signature := &Signature{Status: string(verification.Status), Claims: claimsOf(verification.Claims)}
	switch verification.Status {
	case artifact.VerificationVerified:
		signature.Statement = statementVerified
	case artifact.VerificationUnchecked:
		signature.Statement = statementUnchecked
	default:
		signature.Statement = statementFailed
	}
	subject := verification.Subject
	applySubject(signature, &subject)
	if verification.PolicyFingerprint != "" {
		signature.Policy = code(shortDigestText(verification.PolicyFingerprint))
	}
	for _, evidenceDigest := range verification.EvidenceDigests {
		signature.Evidence = append(signature.Evidence, Evidence{Digest: code(string(evidenceDigest))})
	}
	return signature
}

// applySubject copies the subject of a signature, which is the immutable thing it covers.
func applySubject(signature *Signature, subject *artifact.SignatureSubject) {
	if subject == nil || subject.Digest == "" {
		return
	}
	signature.SubjectKind = plain(string(subject.Kind))
	signature.SubjectDigest = code(string(subject.Digest))
	signature.SubjectMediaType = code(subject.MediaType)
}

// claimsOf converts verified signer claims.
func claimsOf(claims []artifact.SignerClaim) []Claim {
	converted := make([]Claim, 0, len(claims))
	for index := range claims {
		converted = append(converted, Claim{
			Identity: code(claims[index].Identity),
			Issuer:   code(claims[index].Issuer),
			KeyID:    code(claims[index].KeyID),
		})
	}
	if len(converted) == 0 {
		return nil
	}
	return converted
}

// evidenceOf converts stored evidence descriptors.
func evidenceOf(evidence []artifact.SignatureEvidence) []Evidence {
	converted := make([]Evidence, 0, len(evidence))
	for index := range evidence {
		converted = append(converted, Evidence{
			Format: plain(string(evidence[index].Format)),
			Digest: code(string(evidence[index].Descriptor.Digest)),
		})
	}
	if len(converted) == 0 {
		return nil
	}
	return converted
}

// shortDigestText shortens a policy fingerprint, which is a digest-shaped string.
func shortDigestText(value string) string {
	const length = 19
	if len(value) <= length {
		return value
	}
	return value[:length] + ellipsis
}
