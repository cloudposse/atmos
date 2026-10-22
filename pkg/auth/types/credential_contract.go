package types

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_credential_contract.go -package=$GOPACKAGE github.com/cloudposse/atmos/pkg/auth/types CredentialContractProvider,CredentialTransaction

// CredentialContract describes the versioned envelopes accepted and produced by
// an auth step. Empty Accepts denotes a source with no upstream credentials.
type CredentialContract struct{ Accepts, Produces []string }

// CredentialContractProvider exposes static metadata for validation before login.
// Legacy built-ins without this optional interface retain their existing checks.
type CredentialContractProvider interface{ CredentialContract() CredentialContract }
