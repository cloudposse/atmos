package command

import (
	"crypto/sha512"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
	sshagent "golang.org/x/crypto/ssh/agent"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
)

const (
	// defaultSoakInterval is the pause between attempts of a soak run.
	defaultSoakInterval = 30 * time.Second
	// attemptTimeout bounds one connect, list, and sign.
	attemptTimeout = 30 * time.Second
	// auditLinesRead is how much of the audit log is searched for the reason of a refusal.
	auditLinesRead = 50
	// signNamespace is the SSHSIG namespace Git signs under.
	signNamespace = "git"
)

// Results of one attempt.
const (
	resultOK          = "ok"
	resultRefused     = "refused"
	resultLocked      = "locked"
	resultUnreachable = "unreachable"
	resultError       = "error"
)

// ErrTestFailed means at least one attempt did not sign and verify.
var ErrTestFailed = errors.New("signing test failed")

type attempt struct {
	result string
	detail string
}

// test signs a Git payload through an identity's socket and verifies the signature. With --soak it
// repeats every --interval for that long, which is how locked-screen behavior is checked by hand:
// start it, lock the screen, wait, unlock, and read the lines.
func (c *call) test(words []string) error {
	if err := c.flags.only("soak", "interval"); err != nil {
		return err
	}
	if err := exactly(words, 1, "test <identity> [--soak <duration>] [--interval <duration>]"); err != nil {
		return err
	}
	soak, err := c.flags.duration("soak", 0)
	if err != nil {
		return err
	}
	interval, err := c.flags.duration("interval", defaultSoakInterval)
	if err != nil {
		return err
	}
	if soak > 0 && interval <= 0 {
		return usagef("--interval must be longer than zero")
	}
	socket, err := c.paths.SocketPath(words[0])
	if err != nil {
		return err
	}
	tally, runErr := c.soak(words[0], socket, soak, interval)
	if soak > 0 {
		c.printf("Summary: ok=%d refused=%d locked=%d unreachable=%d error=%d\n",
			tally[resultOK], tally[resultRefused], tally[resultLocked], tally[resultUnreachable], tally[resultError])
	}
	if runErr != nil {
		return runErr
	}
	total := 0
	for _, n := range tally {
		total += n
	}
	if failed := total - tally[resultOK]; failed > 0 {
		return fmt.Errorf("%w: %d of %d attempts did not succeed", ErrTestFailed, failed, total)
	}
	return nil
}

// soak runs attempts until the duration has passed. A zero duration runs once.
func (c *call) soak(identity, socket string, soak, interval time.Duration) (map[string]int, error) {
	now := c.svc.deps.Now
	deadline := now().Add(soak)
	tally := map[string]int{}
	for {
		a := c.attempt(identity, socket)
		tally[a.result]++
		c.printf("%s  %-11s  %s  %s\n", now().UTC().Format(time.RFC3339), a.result, identity, a.detail)
		remaining := deadline.Sub(now())
		if soak == 0 || remaining <= 0 {
			return tally, nil
		}
		if err := c.svc.deps.Sleep(c.ctx, min(interval, remaining)); err != nil {
			return tally, err
		}
	}
}

// attempt connects to the socket, signs an SSHSIG payload for Git, and verifies the signature.
func (c *call) attempt(identity, socket string) attempt {
	conn, err := c.svc.deps.Dial(c.ctx, socket)
	if err != nil {
		return attempt{resultUnreachable, err.Error()}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(attemptTimeout))
	client := sshagent.NewClient(conn)
	keys, err := client.List()
	if err != nil {
		return attempt{resultUnreachable, "list keys: " + err.Error()}
	}
	if len(keys) != 1 {
		return attempt{resultRefused, fmt.Sprintf("the socket lists %d keys, want 1", len(keys))}
	}
	payload := sshsigPayload([]byte(fmt.Sprintf("atmos ssh test %s\n", c.svc.deps.Now().UTC().Format(time.RFC3339Nano))))
	sig, err := client.Sign(keys[0], payload)
	if err != nil {
		return c.refusal(identity, err)
	}
	if err := keys[0].Verify(payload, sig); err != nil {
		return attempt{resultError, "the signature did not verify: " + err.Error()}
	}
	return attempt{resultOK, fmt.Sprintf("signed and verified a Git signature (%s %s)", keys[0].Type(), ssh.FingerprintSHA256(keys[0]))}
}

// refusal explains a failed sign request. The agent protocol carries no reason, so it is read from
// the audit log, where the daemon records the last request of the identity.
func (c *call) refusal(identity string, signErr error) attempt {
	entry, ok := lastSignEntry(c.paths.AuditLogFile(), identity)
	if !ok {
		return attempt{resultRefused, signErr.Error()}
	}
	if entry.ScreenLocked {
		return attempt{resultLocked, entry.Reason}
	}
	reason := entry.Reason
	if reason == "" {
		reason = signErr.Error()
	}
	return attempt{resultRefused, reason}
}

// lastSignEntry returns the identity's most recent sign request in the audit log.
func lastSignEntry(path, identity string) (agent.AuditEntry, bool) {
	lines, err := tailLines(path, auditLinesRead)
	if err != nil {
		return agent.AuditEntry{}, false
	}
	for i := len(lines) - 1; i >= 0; i-- {
		var e agent.AuditEntry
		if json.Unmarshal([]byte(lines[i]), &e) == nil && e.Identity == identity && e.Operation == agent.OpSign {
			return e, true
		}
	}
	return agent.AuditEntry{}, false
}

// sshsigPayload builds the data an SSHSIG signature covers (PROTOCOL.sshsig): the magic, the
// namespace, a reserved string, the hash algorithm, and the hash of the message.
func sshsigPayload(message []byte) []byte {
	digest := sha512.Sum512(message)
	out := []byte("SSHSIG")
	for _, field := range [][]byte{[]byte(signNamespace), nil, []byte("sha512"), digest[:]} {
		out = binary.BigEndian.AppendUint32(out, uint32(len(field)))
		out = append(out, field...)
	}
	return out
}
