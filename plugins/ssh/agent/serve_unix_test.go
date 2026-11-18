//go:build !windows

package agent

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	xagent "golang.org/x/crypto/ssh/agent"

	"github.com/cloudposse/atmos/plugins/ssh/keystore"
)

func TestListReturnsOnlyTheIdentityKey(t *testing.T) {
	p := newPaths(t)
	pubA := createFileKey(t, p, "a")
	createFileKey(t, p, "b")
	h := startDaemon(t, p, nil, ident("a", AllowSigning), ident("b", AllowSigning))

	keys, err := h.client("a").List()
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Equal(t, pubA.Marshal(), keys[0].Blob)
	assert.Equal(t, "a", keys[0].Comment)
}

func TestSocketMode(t *testing.T) {
	p := newPaths(t)
	createFileKey(t, p, "a")
	h := startDaemon(t, p, nil, ident("a", AllowSigning))
	info, err := os.Stat(h.socket("a"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	assert.NotZero(t, info.Mode()&os.ModeSocket)
}

// signCase is one sign request and the outcome expected for each allow list.
type signCase struct {
	name    string
	payload func(pub ssh.PublicKey) []byte
	signing bool
	auth    bool
	// auditKind is the expected audit classification.
	auditKind Kind
}

func signCases() []signCase {
	return []signCase{
		{"git signature", func(ssh.PublicKey) []byte { return sshsigBlob("git") }, true, false, KindGitSignature},
		{"sshsig other namespace", func(ssh.PublicKey) []byte { return sshsigBlob("file") }, false, false, KindGitSignature},
		{"userauth for this key", func(pub ssh.PublicKey) []byte {
			return userauthBlob("session-session-session-session", "alice", "ssh-connection", pub)
		}, false, true, KindSSHAuthentication},
		{"userauth for another key", func(ssh.PublicKey) []byte {
			return userauthBlob("session-session-session-session", "alice", "ssh-connection", newPubForCase())
		}, false, false, KindSSHAuthentication},
		{"garbage", func(ssh.PublicKey) []byte { return []byte("just some bytes to sign") }, false, false, KindOther},
	}
}

func newPubForCase() ssh.PublicKey {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	k, _ := ssh.NewPublicKey(pub)
	return k
}

func TestSignRespectsAllowList(t *testing.T) {
	p := newPaths(t)
	pubS := createFileKey(t, p, "signonly")
	pubA := createFileKey(t, p, "authonly")
	h := startDaemon(t, p, nil, ident("signonly", AllowSigning), ident("authonly", AllowAuthentication))

	for _, tc := range signCases() {
		for _, who := range []struct {
			name   string
			pub    ssh.PublicKey
			expect bool
		}{{"signonly", pubS, tc.signing}, {"authonly", pubA, tc.auth}} {
			t.Run(tc.name+" via "+who.name, func(t *testing.T) {
				data := tc.payload(who.pub)
				sig, err := h.client(who.name).Sign(who.pub, data)
				entry := h.lastAudit()
				assert.Equal(t, who.name, entry.Identity)
				assert.Equal(t, OpSign, entry.Operation)
				assert.Equal(t, tc.auditKind, entry.Classification)
				if !who.expect {
					require.Error(t, err)
					assert.Equal(t, ResultRefused, entry.Result)
					assert.NotEmpty(t, entry.Reason)
					return
				}
				require.NoError(t, err)
				require.NoError(t, who.pub.Verify(data, sig))
				assert.Equal(t, ResultOK, entry.Result)
				assert.Equal(t, ssh.FingerprintSHA256(who.pub), entry.Fingerprint)
			})
		}
	}
}

func TestSignRefusesAnotherKey(t *testing.T) {
	p := newPaths(t)
	createFileKey(t, p, "a")
	h := startDaemon(t, p, nil, ident("a", AllowSigning))

	_, err := h.client("a").Sign(newPubForCase(), sshsigBlob("git"))
	require.Error(t, err)
	entry := h.lastAudit()
	assert.Equal(t, ResultRefused, entry.Result)
	assert.Contains(t, entry.Reason, "different public key")
}

func TestManagementRequestsAreRefused(t *testing.T) {
	p := newPaths(t)
	pub := createFileKey(t, p, "a")
	h := startDaemon(t, p, nil, ident("a", AllowSigning, AllowAuthentication))
	c := h.client("a")

	_, extra, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	require.Error(t, c.Add(xagent.AddedKey{PrivateKey: extra}))
	require.Error(t, c.Remove(pub))
	require.Error(t, c.RemoveAll())
	require.Error(t, c.Lock([]byte("pw")))
	require.Error(t, c.Unlock([]byte("pw")))
	_, err = c.Extension("query", nil)
	require.Error(t, err)

	keys, err := c.List()
	require.NoError(t, err)
	require.Len(t, keys, 1, "the key list is unchanged")

	for _, op := range []string{"add", "remove", "remove-all", "lock", "unlock"} {
		var found bool
		for _, e := range h.auditEntries() {
			if e.Operation == op {
				found = true
				assert.Equal(t, ResultRefused, e.Result)
			}
		}
		assert.True(t, found, "audit entry for %s", op)
	}
}

func TestAuditLogHasNoPayload(t *testing.T) {
	p := newPaths(t)
	pub := createFileKey(t, p, "a")
	h := startDaemon(t, p, nil, ident("a", AllowSigning, AllowAuthentication))
	c := h.client("a")

	marker := "UNIQUE-SESSION-MARKER-0123456789"
	data := userauthBlob(marker, "alice", "ssh-connection", pub)
	sig, err := c.Sign(pub, data)
	require.NoError(t, err)
	_, err = c.Sign(pub, []byte("PAYLOAD-MARKER-should-never-be-logged"))
	require.Error(t, err)

	raw, err := os.ReadFile(h.paths.AuditLogFile())
	require.NoError(t, err)
	for _, secret := range [][]byte{
		[]byte(marker), []byte("PAYLOAD-MARKER"), sig.Blob,
		[]byte(base64.StdEncoding.EncodeToString(sig.Blob)),
		[]byte(base64.StdEncoding.EncodeToString(data)),
	} {
		assert.False(t, bytes.Contains(raw, secret), "audit log leaks %q", secret)
	}

	var login AuditEntry
	for _, e := range h.auditEntries() {
		if e.Classification == KindSSHAuthentication {
			login = e
		}
	}
	assert.Equal(t, "alice", login.User)
	assert.Equal(t, "ssh-connection", login.Service)
	assert.Equal(t, ResultOK, login.Result)
}

func TestReloadAddsAndRemovesSockets(t *testing.T) {
	p := newPaths(t)
	createFileKey(t, p, "a")
	createFileKey(t, p, "b")
	h := startDaemon(t, p, nil, ident("a", AllowSigning))

	require.NoError(t, UpsertIdentity(p.AgentConfigFile(), ident("b", AllowSigning)))
	require.Eventually(t, func() bool {
		return CheckSocket(context.Background(), h.socket("b"), nil) == nil
	}, 5*time.Second, 25*time.Millisecond, "new socket comes up")

	// Hold a connection to a, then remove b: a must keep working on the same connection.
	ca := h.client("a")
	_, err := RemoveIdentity(p.AgentConfigFile(), "b")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err := os.Stat(h.socket("b"))
		return os.IsNotExist(err)
	}, 5*time.Second, 25*time.Millisecond, "removed socket goes away")

	keys, err := ca.List()
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.NoError(t, CheckSocket(context.Background(), h.socket("a"), keys[0]))
}

func TestReloadOnSIGHUP(t *testing.T) {
	p := newPaths(t)
	createFileKey(t, p, "a")
	createFileKey(t, p, "b")
	h := startDaemon(t, p, func(o *Options) { o.PollInterval = time.Hour }, ident("a", AllowSigning))

	require.NoError(t, UpsertIdentity(p.AgentConfigFile(), ident("b", AllowSigning)))
	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGHUP))
	require.Eventually(t, func() bool {
		return CheckSocket(context.Background(), h.socket("b"), nil) == nil
	}, 5*time.Second, 25*time.Millisecond)
}

func TestInvalidConfigKeepsRunningSockets(t *testing.T) {
	p := newPaths(t)
	createFileKey(t, p, "a")
	h := startDaemon(t, p, nil, ident("a", AllowSigning))

	require.NoError(t, os.WriteFile(p.AgentConfigFile(), []byte("{broken"), 0o600))
	time.Sleep(200 * time.Millisecond)
	require.NoError(t, CheckSocket(context.Background(), h.socket("a"), nil))
}

func TestChangedIdentityIsRestarted(t *testing.T) {
	p := newPaths(t)
	pub := createFileKey(t, p, "a")
	h := startDaemon(t, p, nil, ident("a", AllowAuthentication))

	_, err := h.client("a").Sign(pub, sshsigBlob("git"))
	require.Error(t, err)

	require.NoError(t, UpsertIdentity(p.AgentConfigFile(), ident("a", AllowSigning)))
	require.Eventually(t, func() bool {
		conn, err := net.Dial("unix", h.socket("a"))
		if err != nil {
			return false
		}
		defer conn.Close()
		_, err = xagent.NewClient(conn).Sign(pub, sshsigBlob("git"))
		return err == nil
	}, 5*time.Second, 25*time.Millisecond)
}

func TestStateFileLifecycle(t *testing.T) {
	p := newPaths(t)
	createFileKey(t, p, "a")
	h := startDaemon(t, p, func(o *Options) { o.Version = "9.9.9" }, ident("a", AllowSigning))

	state, err := ReadState(p.StateFile())
	require.NoError(t, err)
	assert.Equal(t, os.Getpid(), state.PID)
	assert.Equal(t, "9.9.9", state.Version)
	assert.WithinDuration(t, time.Now(), state.Started, time.Minute)

	h.Stop()
	_, err = ReadState(p.StateFile())
	require.ErrorIs(t, err, ErrNoState)
	_, err = os.Stat(h.socket("a"))
	require.ErrorIs(t, err, os.ErrNotExist, "socket removed on exit")
}

func TestStaleSocketIsReplaced(t *testing.T) {
	p := newPaths(t)
	createFileKey(t, p, "a")
	sock := filepath.Join(p.SocketsDir(), "a.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	require.NoError(t, ln.Close())
	_, err = os.Stat(sock)
	require.NoError(t, err, "stale socket file exists")

	h := startDaemon(t, p, nil, ident("a", AllowSigning))
	require.NoError(t, CheckSocket(context.Background(), h.socket("a"), nil))
}

func TestLiveSocketIsNotStolen(t *testing.T) {
	p := newPaths(t)
	sock := filepath.Join(p.SocketsDir(), "a.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer ln.Close()

	require.ErrorIs(t, removeStaleSocket(sock), ErrSocketInUse)
	_, err = os.Stat(sock)
	require.NoError(t, err)
}

func TestRemoveStaleSocketRefusesRegularFile(t *testing.T) {
	path := filepath.Join(shortDir(t), "x.sock")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o600))
	require.Error(t, removeStaleSocket(path))
	_, err := os.Stat(path)
	require.NoError(t, err, "a regular file is left alone")
}

func TestOtherUsersAreRefused(t *testing.T) {
	p := newPaths(t)
	createFileKey(t, p, "a")
	h := startDaemon(t, p, func(o *Options) {
		o.peer = func(*net.UnixConn) (uint32, error) { return uint32(os.Getuid()) + 1, nil }
	}, ident("a", AllowSigning))

	conn, err := net.Dial("unix", h.socket("a"))
	require.NoError(t, err)
	defer conn.Close()
	_, err = xagent.NewClient(conn).List()
	require.Error(t, err, "connection is closed without an answer")

	require.Eventually(t, func() bool {
		for _, e := range h.auditEntries() {
			if e.Operation == OpConnect && e.Result == ResultRefused {
				return true
			}
		}
		return false
	}, 5*time.Second, 25*time.Millisecond)
}

func TestVerifyPeer(t *testing.T) {
	require.NoError(t, verifyPeer(501, 501))
	require.ErrorIs(t, verifyPeer(0, 501), ErrWrongUser)
}

func TestLockedScreenIsReportedAndFails(t *testing.T) {
	p := newPaths(t)
	h := startDaemon(t, p, func(o *Options) {
		o.OpenStore = func(string, string) (keystore.Store, error) { return newLockedStore(t), nil }
	}, Identity{Name: "locked", Store: "fake", Key: "k", Allow: []Allow{AllowSigning}})

	c := h.client("locked")
	keys, err := c.List()
	require.NoError(t, err)
	require.Len(t, keys, 1)
	pub, err := ssh.ParsePublicKey(keys[0].Blob)
	require.NoError(t, err)

	_, err = c.Sign(pub, sshsigBlob("git"))
	require.Error(t, err, "the agent reports a failure")

	entry := h.lastAudit()
	assert.Equal(t, OpSign, entry.Operation)
	assert.Equal(t, ResultError, entry.Result)
	assert.True(t, entry.ScreenLocked)
	assert.Equal(t, KindGitSignature, entry.Classification)
}

func TestOtherSignErrorIsNotScreenLocked(t *testing.T) {
	p := newPaths(t)
	h := startDaemon(t, p, func(o *Options) {
		o.OpenStore = func(string, string) (keystore.Store, error) {
			s := newLockedStore(t)
			s.signErr = os.ErrDeadlineExceeded
			return s, nil
		}
	}, Identity{Name: "broken", Store: "fake", Key: "k", Allow: []Allow{AllowSigning}})

	c := h.client("broken")
	keys, err := c.List()
	require.NoError(t, err)
	pub, err := ssh.ParsePublicKey(keys[0].Blob)
	require.NoError(t, err)
	_, err = c.Sign(pub, sshsigBlob("git"))
	require.Error(t, err)
	entry := h.lastAudit()
	assert.Equal(t, ResultError, entry.Result)
	assert.False(t, entry.ScreenLocked)
}

func TestKeysAreClosed(t *testing.T) {
	p := newPaths(t)
	store := newLockedStore(t)
	store.signErr = nil
	h := startDaemon(t, p, func(o *Options) {
		o.OpenStore = func(string, string) (keystore.Store, error) { return store, nil }
	}, Identity{Name: "k", Store: "fake", Key: "k", Allow: []Allow{AllowSigning}})

	c := h.client("k")
	keys, err := c.List()
	require.NoError(t, err)
	pub, err := ssh.ParsePublicKey(keys[0].Blob)
	require.NoError(t, err)
	_, err = c.Sign(pub, sshsigBlob("git"))
	require.NoError(t, err)
	_, err = c.Sign(pub, []byte("garbage"))
	require.Error(t, err)

	assert.Equal(t, store.opened(), store.closedCount(), "every opened key is closed")
	assert.Positive(t, store.opened())
}

func TestUnknownStoreFailsOnlyThatIdentity(t *testing.T) {
	p := newPaths(t)
	createFileKey(t, p, "good")
	h := startDaemon(t, p, nil,
		Identity{Name: "bad", Store: "no-such-store", Key: "k", Allow: []Allow{AllowSigning}},
		ident("good", AllowSigning))
	require.NoError(t, CheckSocket(context.Background(), h.socket("good"), nil))
	_, err := os.Stat(h.socket("bad"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCheckSocket(t *testing.T) {
	p := newPaths(t)
	pub := createFileKey(t, p, "a")
	other := createFileKey(t, p, "b")
	h := startDaemon(t, p, nil, ident("a", AllowSigning))

	require.NoError(t, CheckSocket(context.Background(), h.socket("a"), pub))
	require.ErrorIs(t, CheckSocket(context.Background(), h.socket("a"), other), ErrUnexpectedKeys)
	require.Error(t, CheckSocket(context.Background(), filepath.Join(p.SocketsDir(), "none.sock"), nil))
}

func TestSSHKeygenSigningEndToEnd(t *testing.T) {
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen is not on PATH; skipping the OpenSSH end-to-end check")
	}
	p := newPaths(t)
	pub := createFileKey(t, p, "e2e")
	h := startDaemon(t, p, nil, ident("e2e", AllowSigning))

	work := shortDir(t)
	message := filepath.Join(work, "commit.txt")
	require.NoError(t, os.WriteFile(message, []byte("tree 0123\nauthor Test\n\nsigned commit\n"), 0o600))
	pubFile := filepath.Join(work, "key.pub")
	require.NoError(t, os.WriteFile(pubFile, ssh.MarshalAuthorizedKey(pub), 0o600))
	allowed := filepath.Join(work, "allowed_signers")
	require.NoError(t, os.WriteFile(allowed,
		append([]byte(`test@example.com namespaces="git" `), ssh.MarshalAuthorizedKey(pub)...), 0o600))

	run := func(stdin []byte, args ...string) ([]byte, error) {
		cmd := exec.Command(keygen, args...)
		cmd.Env = []string{"SSH_AUTH_SOCK=" + h.socket("e2e"), "HOME=" + work}
		cmd.Dir = work
		if stdin != nil {
			cmd.Stdin = bytes.NewReader(stdin)
		}
		return cmd.CombinedOutput()
	}

	out, err := run(nil, "-Y", "sign", "-U", "-f", pubFile, "-n", "git", message)
	require.NoError(t, err, string(out))
	sigFile := message + ".sig"
	require.FileExists(t, sigFile)

	body, err := os.ReadFile(message)
	require.NoError(t, err)
	out, err = run(body, "-Y", "verify", "-f", allowed, "-I", "test@example.com", "-n", "git", "-s", sigFile)
	require.NoError(t, err, string(out))
	assert.Contains(t, string(out), "Good")

	// The same socket must refuse any other namespace.
	out, err = run(nil, "-Y", "sign", "-U", "-f", pubFile, "-n", "file", message)
	require.Error(t, err, string(out))

	var sawGit, sawRefusedFile bool
	for _, e := range h.auditEntries() {
		if e.Operation != OpSign {
			continue
		}
		sawGit = sawGit || (e.Namespace == "git" && e.Result == ResultOK)
		sawRefusedFile = sawRefusedFile || (e.Namespace == "file" && e.Result == ResultRefused)
	}
	assert.True(t, sawGit, "audit log records the git signature")
	assert.True(t, sawRefusedFile, "audit log records the refused namespace")
}

func TestServeRequiresPaths(t *testing.T) {
	require.ErrorIs(t, Serve(context.Background(), Options{}), ErrInvalidConfig)
}
