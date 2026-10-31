package artifact

import "context"

//go:generate mockgen -package artifact -destination mock_verifier.go github.com/cloudposse/atmos/pkg/artifact Verifier

// Verifier authenticates stored evidence against a policy. Repository adapters fetch evidence and
// invoke a Verifier; they may register additional native formats with the same result contract.
type Verifier interface {
	// Formats lists the evidence formats the verifier understands. Required evidence in any other
	// format fails closed.
	Formats() []EvidenceFormat

	// Verify checks subject binding, cryptographic validity, signer or key trust, and the
	// applicable log, certificate, and signing-time proofs. At least one valid permitted signature
	// must satisfy each required trust set; unrelated invalid attachments cannot override valid
	// matching evidence. A public-key signature never satisfies an identity/issuer requirement.
	Verify(ctx context.Context, subject SignatureSubject, evidence []EvidenceBlob, policy VerificationPolicy) (*VerificationResult, error)
}
