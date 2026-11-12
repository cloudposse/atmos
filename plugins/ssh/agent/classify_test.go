package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func wireString(b []byte, s string) []byte {
	n := len(s)
	b = append(b, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	return append(b, s...)
}

func sshsigBlob(namespace string) []byte {
	b := []byte("SSHSIG")
	b = wireString(b, namespace)
	b = wireString(b, "")
	b = wireString(b, "sha512")
	return wireString(b, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
}

func userauthBlob(session, user, service string, pub ssh.PublicKey) []byte {
	b := wireString(nil, session)
	b = append(b, 50)
	b = wireString(b, user)
	b = wireString(b, service)
	b = wireString(b, "publickey")
	b = append(b, 1)
	b = wireString(b, pub.Type())
	return wireString(b, string(pub.Marshal()))
}

func newPub(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sshPub, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return sshPub
}

func TestClassify(t *testing.T) {
	pub := newPub(t)
	auth := userauthBlob("session-id-session-id-session-id", "git", "ssh-connection", pub)

	tests := []struct {
		name string
		data []byte
		want Request
	}{
		{"git signature", sshsigBlob("git"), Request{Kind: KindGitSignature, Namespace: "git"}},
		{"other namespace", sshsigBlob("file"), Request{Kind: KindGitSignature, Namespace: "file"}},
		{"empty namespace", sshsigBlob(""), Request{Kind: KindGitSignature, Namespace: ""}},
		{"truncated sshsig", sshsigBlob("git")[:20], Request{Kind: KindOther}},
		{"sshsig with trailing bytes", append(sshsigBlob("git"), 0), Request{Kind: KindOther}},
		{"magic only", []byte("SSHSIG"), Request{Kind: KindOther}},
		{"empty", nil, Request{Kind: KindOther}},
		{"garbage", []byte("hello, world, this is not a signature request"), Request{Kind: KindOther}},
		{"userauth", auth, Request{Kind: KindSSHAuthentication, User: "git", Service: "ssh-connection", PublicKey: pub}},
		{"userauth with trailing bytes", append(append([]byte{}, auth...), 0), Request{Kind: KindOther}},
		{"userauth truncated", auth[:len(auth)-3], Request{Kind: KindOther}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.data)
			assert.Equal(t, tt.want.Kind, got.Kind)
			assert.Equal(t, tt.want.Namespace, got.Namespace)
			assert.Equal(t, tt.want.User, got.User)
			assert.Equal(t, tt.want.Service, got.Service)
			if tt.want.PublicKey != nil {
				require.NotNil(t, got.PublicKey)
				assert.Equal(t, tt.want.PublicKey.Marshal(), got.PublicKey.Marshal())
			}
		})
	}
}

func TestClassifyUserauthRequiresExactShape(t *testing.T) {
	pub := newPub(t)

	t.Run("wrong message number", func(t *testing.T) {
		b := wireString(nil, "sessionsessionsession")
		b = append(b, 51)
		b = wireString(b, "git")
		b = wireString(b, "ssh-connection")
		b = wireString(b, "publickey")
		b = append(b, 1)
		b = wireString(b, pub.Type())
		b = wireString(b, string(pub.Marshal()))
		assert.Equal(t, KindOther, Classify(b).Kind)
	})
	t.Run("wrong method", func(t *testing.T) {
		b := wireString(nil, "sessionsessionsession")
		b = append(b, 50)
		b = wireString(b, "git")
		b = wireString(b, "ssh-connection")
		b = wireString(b, "password")
		b = append(b, 1)
		b = wireString(b, pub.Type())
		b = wireString(b, string(pub.Marshal()))
		assert.Equal(t, KindOther, Classify(b).Kind)
	})
	t.Run("not a signature query", func(t *testing.T) {
		b := wireString(nil, "sessionsessionsession")
		b = append(b, 50)
		b = wireString(b, "git")
		b = wireString(b, "ssh-connection")
		b = wireString(b, "publickey")
		b = append(b, 0)
		b = wireString(b, pub.Type())
		b = wireString(b, string(pub.Marshal()))
		assert.Equal(t, KindOther, Classify(b).Kind)
	})
	t.Run("unparseable key blob", func(t *testing.T) {
		b := wireString(nil, "sessionsessionsession")
		b = append(b, 50)
		b = wireString(b, "git")
		b = wireString(b, "ssh-connection")
		b = wireString(b, "publickey")
		b = append(b, 1)
		b = wireString(b, "ssh-ed25519")
		b = wireString(b, "not a key")
		assert.Equal(t, KindOther, Classify(b).Kind)
	})
}
