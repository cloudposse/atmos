// Package keystore creates, stores, and loads the private keys behind Atmos SSH identities.
//
// A key store decides where private key material lives and which Requirement terms it can
// enforce. The "file" store keeps an ordinary OpenSSH key on disk. The "secure-enclave" store
// keeps a P-256 key inside Apple's Secure Enclave and persists only a wrapped blob that is useless
// without that machine's hardware.
package keystore

import (
	"context"
	"crypto"
	"errors"
	"io"
	"time"

	"golang.org/x/crypto/ssh"
)

// Requirement is a condition that must hold for a key to be used. The key store enforces it, not
// the caller.
type Requirement string

const (
	// Unlocked requires the user's screen to be unlocked while the key is used.
	Unlocked Requirement = "unlocked"
)

// Key store kind names.
const (
	// KindFile is the kind name of the ordinary OpenSSH private key file store.
	KindFile = "file"
	// KindSecureEnclave is the kind name of the Apple Secure Enclave store.
	KindSecureEnclave = "secure-enclave"
)

// Sentinel errors. Wrap them with fmt.Errorf("%w ...") and test with errors.Is.
var (
	// ErrUnsupportedPlatform means the key store is not available on this operating system.
	ErrUnsupportedPlatform = errors.New("key store is not supported on this platform")
	// ErrUnsupportedRequirement means the key store cannot enforce a requirement.
	ErrUnsupportedRequirement = errors.New("key store cannot enforce requirement")
	// ErrKeyExists means a key with that name already exists.
	ErrKeyExists = errors.New("key already exists")
	// ErrKeyNotFound means no key with that name exists.
	ErrKeyNotFound = errors.New("key not found")
	// ErrRequirementMismatch means a key was created with different requirements than asked for.
	ErrRequirementMismatch = errors.New("key requirements do not match")
	// ErrUnknownKind means no key store is registered under that kind name.
	ErrUnknownKind = errors.New("unknown key store kind")
	// ErrInvalidName means a key name is empty or contains unsafe characters.
	ErrInvalidName = errors.New("invalid key name")
	// ErrInvalidKey means stored key material or its metadata is missing, corrupt, or inconsistent.
	ErrInvalidKey = errors.New("invalid stored key")
	// ErrInvalidDigest means a signing request did not carry a SHA-256 digest.
	ErrInvalidDigest = errors.New("signing requires a SHA-256 digest")
	// ErrKeyClosed means the key was used after Close.
	ErrKeyClosed = errors.New("key is closed")
)

// Info describes a stored key without touching its private material.
type Info struct {
	// Name is the key name within its store.
	Name string
	// Store is the kind name of the store that holds the key.
	Store string
	// Requirements are the requirements the key was created with, sorted and de-duplicated.
	Requirements []Requirement
	// Created is when the key was created, in UTC.
	Created time.Time
	// PublicKey is the key's OpenSSH public key.
	PublicKey ssh.PublicKey
}

// Key is a private key that signs. Callers must Close it when done.
type Key interface {
	crypto.Signer
	io.Closer
	// Name is the key name within its store.
	Name() string
	// SSHPublicKey is the key's OpenSSH public key.
	SSHPublicKey() ssh.PublicKey
	// Requirements are the requirements the key was created with.
	Requirements() []Requirement
	// Info returns the key's metadata.
	Info() Info
}

// Store creates and holds keys of one kind.
type Store interface {
	// Kind is the name the store is registered under.
	Kind() string
	// Supports reports whether the store can enforce the requirement.
	Supports(Requirement) bool
	// Create generates a new key. It returns ErrKeyExists if the name is taken and
	// ErrUnsupportedRequirement if the store cannot enforce a requirement.
	Create(ctx context.Context, name string, requirements []Requirement) (Key, error)
	// Load opens an existing key. It returns ErrKeyNotFound if there is none.
	Load(ctx context.Context, name string) (Key, error)
	// Delete removes a key. It returns ErrKeyNotFound if there is none.
	Delete(ctx context.Context, name string) error
	// List returns the metadata of every key in the store, sorted by name.
	List(ctx context.Context) ([]Info, error)
}
