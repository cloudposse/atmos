package keystore

import (
	"bytes"
	"crypto"
	"fmt"
	"io"

	"golang.org/x/crypto/ssh"
)

// handle is the live private key a store opens: something that signs and can be released.
type handle interface {
	crypto.Signer
	io.Closer
}

// storedKey adapts a handle plus its metadata to the Key interface.
type storedKey struct {
	handle
	info Info
}

// newStoredKey builds a Key, checking that the handle's public key is the one recorded in the
// metadata so a swapped key file or sidecar is caught.
func newStoredKey(h handle, info Info) (Key, error) {
	pub, err := ssh.NewPublicKey(h.Public())
	if err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("%w: %q public key: %w", ErrInvalidKey, info.Name, err)
	}
	if info.PublicKey != nil && !bytes.Equal(pub.Marshal(), info.PublicKey.Marshal()) {
		_ = h.Close()
		return nil, fmt.Errorf("%w: %q key does not match its metadata", ErrInvalidKey, info.Name)
	}
	info.PublicKey = pub
	return &storedKey{handle: h, info: info}, nil
}

func (k *storedKey) Name() string                { return k.info.Name }
func (k *storedKey) SSHPublicKey() ssh.PublicKey { return k.info.PublicKey }
func (k *storedKey) Requirements() []Requirement {
	return append([]Requirement(nil), k.info.Requirements...)
}
func (k *storedKey) Info() Info { return k.info }
