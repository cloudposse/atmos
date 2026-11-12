package keystore

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"

	"golang.org/x/crypto/ssh"
)

func init() {
	Register(KindFile, func(dir string) (Store, error) { return NewFileStore(dir), nil })
}

// fileStore keeps each key as an ordinary, unencrypted OpenSSH ed25519 private key file. It is not
// hardware-protected and so cannot enforce any Requirement.
type fileStore struct {
	dirStore
}

// NewFileStore returns the "file" store rooted at dir.
func NewFileStore(dir string) Store {
	return &fileStore{dirStore{kind: KindFile, dir: dir}}
}

func (s *fileStore) Kind() string { return KindFile }

// Supports is always false: a file cannot stop its reader from using it.
func (s *fileStore) Supports(Requirement) bool { return false }

func (s *fileStore) Create(ctx context.Context, name string, requirements []Requirement) (Key, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validName(name); err != nil {
		return nil, err
	}
	reqs, err := Validate(s, requirements)
	if err != nil {
		return nil, err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("encode public key: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(crypto.PrivateKey(priv), name)
	if err != nil {
		return nil, fmt.Errorf("encode private key: %w", err)
	}
	rec := newRecord(KindFile, name, reqs, sshPub)
	if err := s.writeNew(rec, pem.EncodeToMemory(block)); err != nil {
		return nil, err
	}
	info, err := rec.info()
	if err != nil {
		return nil, err
	}
	return newStoredKey(fileHandle{priv}, info)
}

func (s *fileStore) Load(ctx context.Context, name string) (Key, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	material, rec, err := s.read(name)
	if err != nil {
		return nil, err
	}
	info, err := rec.info()
	if err != nil {
		return nil, err
	}
	raw, err := ssh.ParseRawPrivateKey(material)
	if err != nil {
		return nil, fmt.Errorf("%w: %q: %w", ErrInvalidKey, name, err)
	}
	priv, ok := raw.(*ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: %q is not an ed25519 key", ErrInvalidKey, name)
	}
	return newStoredKey(fileHandle{*priv}, info)
}

func (s *fileStore) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.remove(name)
}

func (s *fileStore) List(ctx context.Context) ([]Info, error) { return s.list(ctx) }

// fileHandle makes an ed25519 key satisfy handle. Releasing it is a no-op.
type fileHandle struct {
	ed25519.PrivateKey
}

func (h fileHandle) Public() crypto.PublicKey { return h.PrivateKey.Public() }
func (h fileHandle) Close() error             { return nil }
