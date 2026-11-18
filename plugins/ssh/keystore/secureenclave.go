package keystore

import (
	"context"
	"crypto"
	"fmt"
	"sync"

	"golang.org/x/crypto/ssh"
)

func init() {
	Register(KindSecureEnclave, func(dir string) (Store, error) { return NewSecureEnclaveStore(dir), nil })
}

// seBackend is the platform half of the Secure Enclave store. Only darwin provides one.
type seBackend interface {
	// generate creates an enclave key whose access class follows requirements and returns the
	// wrapped blob that reloads it.
	generate(requirements []Requirement) (blob []byte, h handle, err error)
	// open reloads a key from its wrapped blob.
	open(blob []byte) (handle, error)
}

var (
	seBackendMu sync.RWMutex
	seBackendFn seBackend // Set by the darwin build; nil elsewhere.
)

func currentSEBackend() (seBackend, error) {
	seBackendMu.RLock()
	defer seBackendMu.RUnlock()
	if seBackendFn == nil {
		return nil, fmt.Errorf("%w: secure-enclave needs macOS", ErrUnsupportedPlatform)
	}
	return seBackendFn, nil
}

// secureEnclaveStore keeps P-256 keys inside the Secure Enclave. Only a wrapped blob and a
// metadata sidecar touch the disk; nothing goes in the keychain.
type secureEnclaveStore struct {
	dirStore
}

// NewSecureEnclaveStore returns the "secure-enclave" store rooted at dir. On platforms other than
// macOS every operation returns ErrUnsupportedPlatform.
func NewSecureEnclaveStore(dir string) Store {
	return &secureEnclaveStore{dirStore{kind: KindSecureEnclave, dir: dir}}
}

func (s *secureEnclaveStore) Kind() string { return KindSecureEnclave }

// Supports reports true for Unlocked, and only where the Secure Enclave is reachable.
func (s *secureEnclaveStore) Supports(r Requirement) bool {
	if _, err := currentSEBackend(); err != nil {
		return false
	}
	return r == Unlocked
}

func (s *secureEnclaveStore) Create(ctx context.Context, name string, requirements []Requirement) (Key, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	backend, err := currentSEBackend()
	if err != nil {
		return nil, err
	}
	if err := validName(name); err != nil {
		return nil, err
	}
	reqs, err := Validate(s, requirements)
	if err != nil {
		return nil, err
	}
	blob, h, err := backend.generate(reqs)
	if err != nil {
		return nil, err
	}
	key, err := s.persist(name, reqs, blob, h)
	if err != nil {
		_ = h.Close()
		return nil, err
	}
	return key, nil
}

func (s *secureEnclaveStore) persist(name string, reqs []Requirement, blob []byte, h handle) (Key, error) {
	pub, err := sshPublicKey(h.Public())
	if err != nil {
		return nil, err
	}
	rec := newRecord(KindSecureEnclave, name, reqs, pub)
	if err := s.writeNew(rec, blob); err != nil {
		return nil, err
	}
	info, err := rec.info()
	if err != nil {
		return nil, err
	}
	return newStoredKey(h, info)
}

func (s *secureEnclaveStore) Load(ctx context.Context, name string) (Key, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	backend, err := currentSEBackend()
	if err != nil {
		return nil, err
	}
	blob, rec, err := s.read(name)
	if err != nil {
		return nil, err
	}
	info, err := rec.info()
	if err != nil {
		return nil, err
	}
	h, err := backend.open(blob)
	if err != nil {
		return nil, err
	}
	return newStoredKey(h, info)
}

func (s *secureEnclaveStore) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := currentSEBackend(); err != nil {
		return err
	}
	return s.remove(name)
}

func (s *secureEnclaveStore) List(ctx context.Context) ([]Info, error) {
	if _, err := currentSEBackend(); err != nil {
		return nil, err
	}
	return s.list(ctx)
}

// checkSHA256Digest is shared by backends: it checks that a signing request carries a SHA-256
// digest, which is what ecdsa-sha2-nistp256 signs.
func checkSHA256Digest(digest []byte, opts crypto.SignerOpts) error {
	if opts != nil && opts.HashFunc() != crypto.SHA256 {
		return fmt.Errorf("%w: got %v", ErrInvalidDigest, opts.HashFunc())
	}
	if len(digest) != crypto.SHA256.Size() {
		return fmt.Errorf("%w: got %d bytes", ErrInvalidDigest, len(digest))
	}
	return nil
}

func sshPublicKey(pub crypto.PublicKey) (ssh.PublicKey, error) {
	k, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("%w: public key: %w", ErrInvalidKey, err)
	}
	return k, nil
}
