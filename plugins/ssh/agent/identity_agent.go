package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/cloudposse/atmos/plugins/ssh/keystore"
)

// Sentinel errors for refused requests. The SSH agent protocol carries no detail to the client, so
// these reach the caller only as a generic agent failure; the audit log records the reason.
var (
	// ErrRefused means the agent refuses the request by policy.
	ErrRefused = errors.New("request refused")
	// ErrUnknownKey means the request names a key this socket does not serve.
	ErrUnknownKey = errors.New("key is not served by this socket")
)

// Reasons recorded in the audit log.
const (
	reasonNotAllowed     = "not allowed by the identity's allow list"
	reasonUnclassified   = "payload is neither a Git signature nor an SSH login"
	reasonKeyMismatch    = "request names a different public key"
	reasonLoginKey       = "login request names a different public key"
	reasonManagement     = "key management is not supported"
	reasonScreenLocked   = "key store refused: screen is locked"
	reasonKeyUnavailable = "key could not be opened"
)

// identityAgent is the agent.ExtendedAgent behind one identity's socket. It serves exactly one key
// and signs only what the identity's allow list permits.
type identityAgent struct {
	id    Identity
	store keystore.Store
	audit *AuditLog
	// onAuditError receives audit write failures.
	onAuditError func(error)
}

var _ agent.ExtendedAgent = (*identityAgent)(nil)

func (a *identityAgent) record(e AuditEntry) {
	e.Identity = a.id.Name
	if err := a.audit.Record(e); err != nil && a.onAuditError != nil {
		a.onAuditError(err)
	}
}

// open loads the identity's key. The caller must Close it.
func (a *identityAgent) open() (keystore.Key, error) {
	key, err := a.store.Load(context.Background(), a.id.Key)
	if err != nil {
		return nil, fmt.Errorf("open key %q in store %q: %w", a.id.Key, a.id.Store, err)
	}
	return key, nil
}

// List returns exactly the identity's key.
func (a *identityAgent) List() ([]*agent.Key, error) {
	key, err := a.open()
	if err != nil {
		a.record(AuditEntry{Operation: OpList, Result: ResultError, Reason: reasonKeyUnavailable})
		return nil, err
	}
	defer key.Close()
	pub := key.SSHPublicKey()
	a.record(AuditEntry{Operation: OpList, Fingerprint: ssh.FingerprintSHA256(pub), Result: ResultOK})
	return []*agent.Key{{Format: pub.Type(), Blob: pub.Marshal(), Comment: a.id.Name}}, nil
}

// Sign signs data with the identity's key when the request is permitted.
func (a *identityAgent) Sign(pub ssh.PublicKey, data []byte) (*ssh.Signature, error) {
	return a.SignWithFlags(pub, data, 0)
}

// SignWithFlags is Sign with agent signature flags.
func (a *identityAgent) SignWithFlags(pub ssh.PublicKey, data []byte, flags agent.SignatureFlags) (*ssh.Signature, error) {
	req := Classify(data)
	entry := AuditEntry{
		Operation: OpSign, Classification: req.Kind,
		Namespace: req.Namespace, User: req.User, Service: req.Service,
	}
	key, err := a.open()
	if err != nil {
		entry.Result, entry.Reason = ResultError, reasonKeyUnavailable
		a.record(entry)
		return nil, err
	}
	defer key.Close()
	own := key.SSHPublicKey()
	entry.Fingerprint = ssh.FingerprintSHA256(own)

	if reason := a.refusal(req, pub, own); reason != "" {
		entry.Result, entry.Reason = ResultRefused, reason
		a.record(entry)
		return nil, fmt.Errorf("%w: %s", ErrRefused, reason)
	}
	sig, err := signWith(key, data, flags)
	if err != nil {
		entry.Result = ResultError
		if keystore.IsInteractionNotAllowed(err) {
			entry.ScreenLocked, entry.Reason = true, reasonScreenLocked
		}
		a.record(entry)
		return nil, err
	}
	entry.Result = ResultOK
	a.record(entry)
	return sig, nil
}

// refusal returns why the request must be refused, or "" when it is permitted.
func (a *identityAgent) refusal(req Request, asked, own ssh.PublicKey) string {
	if !samePublicKey(asked, own) {
		return reasonKeyMismatch
	}
	switch req.Kind {
	case KindGitSignature:
		if !a.id.Allows(AllowSigning) || req.Namespace != gitNamespace {
			return reasonNotAllowed
		}
	case KindSSHAuthentication:
		if !a.id.Allows(AllowAuthentication) {
			return reasonNotAllowed
		}
		if !samePublicKey(req.PublicKey, own) {
			return reasonLoginKey
		}
	default:
		return reasonUnclassified
	}
	return ""
}

func samePublicKey(a, b ssh.PublicKey) bool {
	return a != nil && b != nil && bytes.Equal(a.Marshal(), b.Marshal())
}

// signWith signs data with a store key. The key types the stores produce (ed25519 and ECDSA P-256)
// have a single signature algorithm, so the agent flags, which only select RSA variants, do not
// apply.
func signWith(key keystore.Key, data []byte, _ agent.SignatureFlags) (*ssh.Signature, error) {
	signer, err := ssh.NewSignerFromSigner(key)
	if err != nil {
		return nil, fmt.Errorf("create signer: %w", err)
	}
	sig, err := signer.Sign(rand.Reader, data)
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}
	return sig, nil
}

func (a *identityAgent) refuseManagement(op string) error {
	a.record(AuditEntry{Operation: op, Result: ResultRefused, Reason: reasonManagement})
	return fmt.Errorf("%w: %s", ErrRefused, reasonManagement)
}

// Add is refused: keys are managed through the plugin, never through the agent socket.
func (a *identityAgent) Add(agent.AddedKey) error { return a.refuseManagement("add") }

// Remove is refused.
func (a *identityAgent) Remove(ssh.PublicKey) error { return a.refuseManagement("remove") }

// RemoveAll is refused.
func (a *identityAgent) RemoveAll() error { return a.refuseManagement("remove-all") }

// Lock is refused.
func (a *identityAgent) Lock([]byte) error { return a.refuseManagement("lock") }

// Unlock is refused.
func (a *identityAgent) Unlock([]byte) error { return a.refuseManagement("unlock") }

// Signers is refused: the agent never hands out in-process signers.
func (a *identityAgent) Signers() ([]ssh.Signer, error) {
	return nil, a.refuseManagement("signers")
}

// Extension is refused.
func (a *identityAgent) Extension(string, []byte) ([]byte, error) {
	a.record(AuditEntry{Operation: "extension", Result: ResultRefused, Reason: reasonManagement})
	return nil, agent.ErrExtensionUnsupported
}
