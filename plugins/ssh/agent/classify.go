package agent

import (
	"bytes"

	"golang.org/x/crypto/ssh"
)

// Kind classifies a sign request by what its payload is.
type Kind string

const (
	// KindGitSignature is an SSHSIG blob (OpenSSH PROTOCOL.sshsig).
	KindGitSignature Kind = "git-signature"
	// KindSSHAuthentication is an SSH userauth request (RFC 4252 section 7).
	KindSSHAuthentication Kind = "ssh-authentication"
	// KindOther is anything else.
	KindOther Kind = "other"
)

// sshsigMagic starts every SSHSIG signed-data blob.
const sshsigMagic = "SSHSIG"

// gitNamespace is the SSHSIG namespace Git signs under.
const gitNamespace = "git"

// msgUserauthRequest is SSH_MSG_USERAUTH_REQUEST.
const msgUserauthRequest = 50

// Request is what Classify learned about a sign request payload. It holds no payload bytes.
type Request struct {
	// Kind is the classification.
	Kind Kind
	// Namespace is the SSHSIG namespace, for KindGitSignature.
	Namespace string
	// User and Service are the userauth user name and service, for KindSSHAuthentication.
	User    string
	Service string
	// PublicKey is the key the userauth request names, for KindSSHAuthentication.
	PublicKey ssh.PublicKey
}

// Classify decides what a sign request payload is. A payload that does not parse completely as one
// of the two known formats is KindOther.
func Classify(data []byte) Request {
	if bytes.HasPrefix(data, []byte(sshsigMagic)) {
		if ns, ok := parseSSHSIG(data[len(sshsigMagic):]); ok {
			return Request{Kind: KindGitSignature, Namespace: ns}
		}
		return Request{Kind: KindOther}
	}
	if user, service, pub, ok := parseUserauth(data); ok {
		return Request{Kind: KindSSHAuthentication, User: user, Service: service, PublicKey: pub}
	}
	return Request{Kind: KindOther}
}

// reader consumes SSH wire-format fields and remembers the first failure.
type reader struct {
	b  []byte
	ok bool
}

func newReader(b []byte) *reader { return &reader{b: b, ok: true} }

func (r *reader) str() []byte {
	if !r.ok || len(r.b) < 4 {
		r.ok = false
		return nil
	}
	n := uint64(r.b[0])<<24 | uint64(r.b[1])<<16 | uint64(r.b[2])<<8 | uint64(r.b[3])
	if n > uint64(len(r.b)-4) {
		r.ok = false
		return nil
	}
	out := r.b[4 : 4+n]
	r.b = r.b[4+n:]
	return out
}

func (r *reader) byte() byte {
	if !r.ok || len(r.b) < 1 {
		r.ok = false
		return 0
	}
	v := r.b[0]
	r.b = r.b[1:]
	return v
}

func (r *reader) done() bool { return r.ok && len(r.b) == 0 }

// parseSSHSIG parses what follows the magic: namespace, reserved, hash_algorithm, H(message). The
// blob must end there.
func parseSSHSIG(rest []byte) (namespace string, ok bool) {
	r := newReader(rest)
	ns := r.str()
	_ = r.str() // reserved.
	hashAlg := r.str()
	digest := r.str()
	if !r.done() || len(hashAlg) == 0 || len(digest) == 0 {
		return "", false
	}
	return string(ns), true
}

// parseUserauth parses a userauth request signature payload (RFC 4252 section 7):
// string session_id, byte 50, string user, string service, string "publickey", boolean TRUE,
// string algorithm, string public_key. The payload must end there.
func parseUserauth(data []byte) (user, service string, pub ssh.PublicKey, ok bool) {
	r := newReader(data)
	session := r.str()
	msg := r.byte()
	u := r.str()
	svc := r.str()
	method := r.str()
	hasSig := r.byte()
	alg := r.str()
	blob := r.str()
	if !r.done() || len(session) == 0 || msg != msgUserauthRequest ||
		string(method) != "publickey" || hasSig != 1 || len(alg) == 0 {
		return "", "", nil, false
	}
	key, err := ssh.ParsePublicKey(blob)
	if err != nil {
		return "", "", nil, false
	}
	return string(u), string(svc), key, true
}
