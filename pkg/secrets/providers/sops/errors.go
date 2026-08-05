package sops

import "errors"

// SOPS-specific sentinel errors. They live with the SOPS provider (not in the shared providers
// package) because only this backend produces them; callers assert on them via errors.Is.
var (
	// ErrSopsFilePathTemplate indicates the SOPS `spec.file` Go template could not be rendered.
	ErrSopsFilePathTemplate = errors.New("failed to render SOPS file path template")
	// ErrSopsDecrypt indicates the SOPS file could not be decrypted in-process.
	ErrSopsDecrypt = errors.New("failed to decrypt SOPS file")
	// ErrSopsMacMismatch indicates the SOPS file MAC did not match the computed MAC.
	ErrSopsMacMismatch = errors.New("SOPS MAC mismatch")
	// ErrSopsEncrypt indicates the SOPS file could not be encrypted in-process.
	ErrSopsEncrypt = errors.New("failed to encrypt SOPS file")
	// ErrSopsRecipients indicates encryption recipients could not be resolved for a fresh file.
	ErrSopsRecipients = errors.New("failed to resolve SOPS recipients (set `spec.age_recipients` or add a matching .sops.yaml creation rule)")
	// ErrSopsAgeKeyFile indicates the configured `spec.age_key_file` could not be read or parsed.
	ErrSopsAgeKeyFile = errors.New("failed to load SOPS age key file (`spec.age_key_file`)")
	// ErrSopsAgeKey indicates the inline `spec.age_key` material could not be parsed.
	ErrSopsAgeKey = errors.New("failed to parse SOPS age key (`spec.age_key`)")
	// ErrSecretFileNotFound indicates the referenced SOPS file does not exist.
	ErrSecretFileNotFound = errors.New("SOPS file not found")
	// ErrSecretNotInitialized indicates the secret key is absent from its backend.
	ErrSecretNotInitialized = errors.New("secret is not initialized in its backend")
)
