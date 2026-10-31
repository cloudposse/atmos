package errors

import "errors"

// Shared signature-verification sentinels used by pkg/signature. Adapters that predate the shared
// package translate these to their legacy sentinels so existing callers keep matching.
var (
	// ErrSignatureInvalid indicates signature evidence is malformed, untrusted, or does not match the subject.
	ErrSignatureInvalid = errors.New("signature does not satisfy the trust policy")

	// ErrSignatureConfigInvalid indicates the verification policy itself is unusable (for example an empty identity).
	ErrSignatureConfigInvalid = errors.New("invalid signature verification configuration")

	// ErrSignatureOffline indicates trust material cannot be fetched while verifying offline.
	ErrSignatureOffline = errors.New("signature trust material is unavailable offline")
)
