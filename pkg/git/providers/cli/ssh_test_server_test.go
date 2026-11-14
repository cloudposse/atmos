package cli

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

type sshTestCall struct {
	user    string
	command string
	at      time.Time
}

type sshTestAction struct {
	message    string
	disconnect bool
}

// sshGitServer accepts only the generated client key on loopback and only the
// two Git pack commands for one fixture repository. It never executes a shell.
type sshGitServer struct {
	listener    net.Listener
	config      *ssh.ServerConfig
	env         []string
	repo        string
	cancel      context.CancelFunc
	ctx         context.Context
	workers     sync.WaitGroup
	mu          sync.Mutex
	connections map[net.Conn]struct{}
	calls       []sshTestCall
	inject      func(sshTestCall, int) sshTestAction
}

func newSSHGitServer(t *testing.T, env []string, repo string, inject func(sshTestCall, int) sshTestAction) *sshGitServer {
	t.Helper()
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		t.Skip("OpenSSH client unavailable")
	}
	dir := t.TempDir()
	host := sshTestSigner(t)
	client := sshTestSigner(t)
	key, err := ssh.MarshalPrivateKey(client.key, "fixture")
	require.NoError(t, err)
	identity := filepath.Join(dir, "identity")
	require.NoError(t, os.WriteFile(identity, pem.EncodeToMemory(key), 0o600))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	s := &sshGitServer{listener: listener, env: env, repo: repo, ctx: ctx, cancel: cancel, connections: map[net.Conn]struct{}{}, inject: inject}
	s.config = &ssh.ServerConfig{PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if meta.User() == "denied" || !bytes.Equal(key.Marshal(), client.signer.PublicKey().Marshal()) {
			return nil, errors.New("fixture key rejected")
		}
		return nil, nil
	}}
	s.config.AddHostKey(host.signer)
	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)
	knownHosts := filepath.Join(dir, "known_hosts")
	require.NoError(t, os.WriteFile(knownHosts, []byte("[127.0.0.1]:"+port+" "+string(ssh.MarshalAuthorizedKey(host.signer.PublicKey()))), 0o600))
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
	command := quote(sshPath) + " -F /dev/null -o BatchMode=yes -o IdentitiesOnly=yes -o IdentityAgent=none -o StrictHostKeyChecking=yes -o GlobalKnownHostsFile=/dev/null -o UserKnownHostsFile=" + quote(knownHosts) + " -o ConnectTimeout=5 -i " + quote(identity)
	s.env = append(append([]string{}, env...), "GIT_SSH_COMMAND="+command, "GIT_SSH_VARIANT=ssh")
	s.workers.Add(1)
	go s.accept()
	t.Cleanup(s.close)
	return s
}

type sshFixtureKey struct {
	key    ed25519.PrivateKey
	signer ssh.Signer
}

func sshTestSigner(t *testing.T) sshFixtureKey {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)
	return sshFixtureKey{key: key, signer: signer}
}

func (s *sshGitServer) remote(user string) string {
	return "ssh://" + user + "@" + s.listener.Addr().String() + "/repo.git"
}

func (s *sshGitServer) accept() {
	defer s.workers.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		if s.ctx.Err() != nil {
			s.mu.Unlock()
			_ = conn.Close()
			return
		}
		s.connections[conn] = struct{}{}
		s.mu.Unlock()
		s.workers.Add(1)
		go s.serve(conn)
	}
}

func (s *sshGitServer) serve(conn net.Conn) {
	defer s.workers.Done()
	defer func() { _ = conn.Close(); s.mu.Lock(); delete(s.connections, conn); s.mu.Unlock() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	server, channels, requests, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		return
	}
	defer server.Close()
	_ = conn.SetDeadline(time.Time{})
	// Global requests do not carry Git data. Track every helper through shutdown.
	s.workers.Add(1)
	go func() { defer s.workers.Done(); ssh.DiscardRequests(requests) }()
	for incoming := range channels {
		if incoming.ChannelType() != "session" {
			_ = incoming.Reject(ssh.UnknownChannelType, "session required")
			continue
		}
		channel, requests, err := incoming.Accept()
		if err != nil {
			continue
		}
		s.session(server.User(), channel, requests)
	}
}

func (s *sshGitServer) session(user string, channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()
	for request := range requests {
		if request.Type != "exec" {
			_ = request.Reply(false, nil)
			continue
		}
		var payload struct{ Command string }
		if ssh.Unmarshal(request.Payload, &payload) != nil {
			_ = request.Reply(false, nil)
			return
		}
		command := ""
		switch payload.Command {
		case "git-receive-pack '/repo.git'":
			command = "receive-pack"
		case "git-upload-pack '/repo.git'":
			command = "upload-pack"
		default:
			_ = request.Reply(false, nil)
			return
		}
		_ = request.Reply(true, nil)
		s.execute(user, command, channel)
		return
	}
}

func (s *sshGitServer) execute(user, command string, channel ssh.Channel) {
	call := sshTestCall{user: user, command: command, at: time.Now()}
	s.mu.Lock()
	ordinal := 0
	for _, previous := range s.calls {
		if previous.user == user && previous.command == command {
			ordinal++
		}
	}
	s.calls = append(s.calls, call)
	s.mu.Unlock()
	action := sshTestAction{}
	if s.inject != nil {
		action = s.inject(call, ordinal)
	}
	if action.disconnect {
		return
	}
	status := uint32(0)
	if action.message != "" {
		_, _ = io.WriteString(channel.Stderr(), action.message+"\n")
		status = 1
	} else {
		cmd := exec.CommandContext(s.ctx, "git", command, s.repo)
		cmd.Env = s.env
		cmd.Stdin = channel
		cmd.Stdout = channel
		cmd.Stderr = channel.Stderr()
		cmd.WaitDelay = time.Second
		if err := cmd.Run(); err != nil {
			status = 1
		}
	}
	_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
}

func (s *sshGitServer) snapshot() []sshTestCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]sshTestCall{}, s.calls...)
}

func (s *sshGitServer) close() {
	s.cancel()
	_ = s.listener.Close()
	s.mu.Lock()
	for conn := range s.connections {
		_ = conn.Close()
	}
	s.mu.Unlock()
	s.workers.Wait()
}

func sshPushFixture(t *testing.T, env []string) (string, string) {
	t.Helper()
	requireGitBinary(t)
	bare := filepath.Join(t.TempDir(), "repo.git")
	gitRun(t, env, "", "init", "--bare", bare)
	seed := t.TempDir()
	gitRun(t, env, seed, "init")
	writeFile(t, filepath.Join(seed, "manifest.yaml"), "image: v1\n")
	gitRun(t, env, seed, "add", ".")
	gitRun(t, env, seed, "commit", "-m", "seed")
	gitRun(t, env, seed, "push", bare, "main")
	return bare, seed
}

func sshPublisherFixture(t *testing.T, env []string, seed string, id int) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, env, "", "clone", seed, dir)
	writeFile(t, filepath.Join(dir, fmt.Sprintf("app-%d.yaml", id)), "image: v2\n")
	gitRun(t, env, dir, "add", ".")
	gitRun(t, env, dir, "commit", "-m", fmt.Sprintf("publish app %d", id))
	return dir
}
