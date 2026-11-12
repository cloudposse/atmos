//go:build !darwin

package keystore

// No Secure Enclave backend exists off macOS. seBackendFn stays nil, so the secure-enclave store
// registers but every operation returns ErrUnsupportedPlatform.
