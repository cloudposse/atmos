//go:build darwin

package keystore

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"fmt"
	"io"
	"sync"
	"unsafe"
)

func init() {
	seBackendMu.Lock()
	defer seBackendMu.Unlock()
	seBackendFn = darwinBackend{}
}

// darwinBackend drives the Secure Enclave through Security.framework's C API, called with purego
// so the binary builds with CGO_ENABLED=0 and no Swift.
type darwinBackend struct{}

func (darwinBackend) generate(requirements []Requirement) ([]byte, handle, error) {
	api, err := loadAPI()
	if err != nil {
		return nil, nil, err
	}
	var rel releaser
	rel.api = api
	defer rel.release()

	// Without a requirement the key stays usable while the screen is locked; "unlocked" ties it to
	// an unlocked device. Only private-key usage is required: no biometry and no user presence.
	protection := api.accessibleAfterFirst
	for _, r := range requirements {
		if r == Unlocked {
			protection = api.accessibleWhenUnlocked
		}
	}
	var cfErr uintptr
	ac := rel.add(api.accessControlCreate(0, protection, accessPrivateKeyUsage, unsafe.Pointer(&cfErr)))
	if ac == 0 {
		return nil, nil, api.toError("SecAccessControlCreateWithFlags", cfErr)
	}
	privAttrs := rel.add(api.dict(api.attrIsPermanent, api.cfFalse, api.attrAccessControl, ac))
	bits := rel.add(api.number(256))
	params := rel.add(api.dict(
		api.attrKeyType, api.attrKeyTypeECSECPrime,
		api.attrKeySizeInBits, bits,
		api.attrTokenID, api.attrTokenIDSecureEnclave,
		api.privateKeyAttrs, privAttrs,
	))
	cfErr = 0
	ref := api.keyCreateRandom(params, unsafe.Pointer(&cfErr))
	if ref == 0 {
		return nil, nil, api.toError("SecKeyCreateRandomKey", cfErr)
	}
	k, err := newSEKey(api, ref)
	if err != nil {
		api.cfRelease(ref)
		return nil, nil, err
	}
	attrs := rel.add(api.keyCopyAttributes(ref))
	blobRef := api.cfDictionaryGet(attrs, api.tokenOIDKey) // Get rule: owned by attrs.
	if blobRef == 0 {
		_ = k.Close()
		return nil, nil, fmt.Errorf("%w: Secure Enclave key has no token object ID", ErrInvalidKey)
	}
	return api.bytesOf(blobRef), k, nil
}

func (darwinBackend) open(blob []byte) (handle, error) {
	api, err := loadAPI()
	if err != nil {
		return nil, err
	}
	return openSEKey(api, blob)
}

// openSEKey reloads a Secure Enclave key from its wrapped blob. Blobs made by CryptoKit's
// SecureEnclave.P256 dataRepresentation load unchanged.
func openSEKey(api *secAPI, blob []byte) (*seKey, error) {
	var rel releaser
	rel.api = api
	defer rel.release()

	empty := rel.add(api.data(nil))
	blobData := rel.add(api.data(blob))
	attrs := rel.add(api.dict(
		api.attrKeyType, api.attrKeyTypeECSECPrime,
		api.attrKeyClass, api.attrKeyClassPrivate,
		api.attrTokenID, api.attrTokenIDSecureEnclave,
		api.tokenOIDKey, blobData,
	))
	var cfErr uintptr
	ref := api.keyCreateWithData(empty, attrs, unsafe.Pointer(&cfErr))
	if ref == 0 {
		return nil, api.toError("SecKeyCreateWithData", cfErr)
	}
	k, err := newSEKey(api, ref)
	if err != nil {
		api.cfRelease(ref)
		return nil, err
	}
	return k, nil
}

// seKey is a live Secure Enclave key. It satisfies handle.
type seKey struct {
	api    *secAPI
	pub    *ecdsa.PublicKey
	mu     sync.RWMutex
	ref    uintptr
	closed bool
}

// newSEKey wraps ref, reading its public key. On success it owns ref; on error the caller keeps it.
func newSEKey(api *secAPI, ref uintptr) (*seKey, error) {
	var rel releaser
	rel.api = api
	defer rel.release()

	pubRef := rel.add(api.keyCopyPublicKey(ref))
	if pubRef == 0 {
		return nil, fmt.Errorf("%w: SecKeyCopyPublicKey returned no key", ErrInvalidKey)
	}
	var cfErr uintptr
	repr := rel.add(api.keyCopyExternalRep(pubRef, unsafe.Pointer(&cfErr)))
	if repr == 0 {
		return nil, api.toError("SecKeyCopyExternalRepresentation", cfErr)
	}
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), api.bytesOf(repr))
	if err != nil {
		return nil, fmt.Errorf("%w: parse Secure Enclave public key: %w", ErrInvalidKey, err)
	}
	return &seKey{api: api, ref: ref, pub: pub}, nil
}

func (k *seKey) Public() crypto.PublicKey { return k.pub }

// Sign signs a SHA-256 digest with the enclave key and returns an ASN.1 DER ECDSA signature. A
// locked screen on a key that requires "unlocked" surfaces as an *OSStatusError for which
// IsInteractionNotAllowed is true.
func (k *seKey) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if err := checkSHA256Digest(digest, opts); err != nil {
		return nil, err
	}
	k.mu.RLock()
	defer k.mu.RUnlock()
	if k.closed {
		return nil, ErrKeyClosed
	}
	var rel releaser
	rel.api = k.api
	defer rel.release()

	data := rel.add(k.api.data(digest))
	var cfErr uintptr
	sig := rel.add(k.api.keyCreateSignature(k.ref, k.api.algECDSADigestSHA256, data, unsafe.Pointer(&cfErr)))
	if sig == 0 {
		return nil, k.api.toError("SecKeyCreateSignature", cfErr)
	}
	return k.api.bytesOf(sig), nil
}

// Close releases the key reference. It is safe to call more than once.
func (k *seKey) Close() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.closed {
		k.closed = true
		k.api.cfRelease(k.ref)
	}
	return nil
}
