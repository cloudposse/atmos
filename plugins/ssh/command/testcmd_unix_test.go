//go:build !windows

package command_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/command"
	"github.com/cloudposse/atmos/plugins/ssh/internal/testfakes"
	"github.com/cloudposse/atmos/plugins/ssh/keystore"
)

// serve runs the real agent daemon on the rig's root until the test ends.
func (r *rig) serve(opener agent.StoreOpener) {
	r.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- agent.Serve(ctx, agent.Options{
			Paths: r.paths, Version: version, OpenStore: opener, PollInterval: 20 * time.Millisecond,
			Ready: func() { close(ready) },
		})
	}()
	select {
	case <-ready:
	case err := <-done:
		r.t.Fatalf("the agent stopped early: %v", err)
	case <-time.After(10 * time.Second):
		r.t.Fatal("the agent did not become ready")
	}
	r.t.Cleanup(func() {
		cancel()
		<-done
	})
}

func (r *rig) configureAndServe(allow []agent.Allow, opener agent.StoreOpener) {
	r.t.Helper()
	r.createKey("ops")
	r.configure(agent.Identity{Name: "ops", Store: keystore.KindFile, Key: "ops", Allow: allow})
	r.serve(opener)
}

func TestTestSignsAndVerifiesThroughTheIdentitysSocket(t *testing.T) {
	r := newRig(t)
	r.configureAndServe([]agent.Allow{agent.AllowSigning}, testfakes.FileOpener)

	code, out, errOut := r.run("test", "ops")

	require.Equal(t, command.ExitOK, code, errOut+out)
	assert.Contains(t, out, "2026-11-12T10:00:00Z  ok")
	assert.Contains(t, out, "ops  signed and verified a Git signature (ssh-ed25519 SHA256:")
	assert.NotContains(t, out, "Summary", "a single attempt prints no summary")
}

func TestAnAuthenticationOnlyIdentityCannotSignCommits(t *testing.T) {
	r := newRig(t)
	r.configureAndServe([]agent.Allow{agent.AllowAuthentication}, testfakes.FileOpener)

	code, out, errOut := r.run("test", "ops")

	assert.Equal(t, command.ExitFailure, code, "an authentication-only identity cannot sign commits")
	assert.Contains(t, out, "refused")
	assert.Contains(t, out, "ops  ", "the reason from the audit log follows")
	assert.Contains(t, errOut, "1 of 1 attempts did not succeed")
	assert.NotContains(t, out, "signed and verified")
}

func TestTestReportsALockedScreenFromTheAuditLog(t *testing.T) {
	r := newRig(t)
	r.configureAndServe([]agent.Allow{agent.AllowSigning}, testfakes.LockedOpener)

	code, out, errOut := r.run("test", "ops")

	assert.Equal(t, command.ExitFailure, code)
	assert.Contains(t, out, "locked")
	assert.Contains(t, out, "key store refused: screen is locked")
	assert.Contains(t, errOut, "did not succeed")
}

func TestTestOfAnIdentityNobodyServesIsUnreachable(t *testing.T) {
	r := newRig(t)

	code, out, _ := r.run("test", "ghost")

	assert.Equal(t, command.ExitFailure, code)
	assert.Contains(t, out, "unreachable")
}

func TestSoakRepeatsEveryIntervalAndSummarizes(t *testing.T) {
	r := newRig(t)
	r.configureAndServe([]agent.Allow{agent.AllowSigning}, testfakes.FileOpener)
	start := r.clock.Now()

	code, out, errOut := r.run("test", "ops", "--soak", "3s", "--interval", "1s")

	require.Equal(t, command.ExitOK, code, errOut+out)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 5, out)
	for i := 0; i < 4; i++ {
		want := start.Add(time.Duration(i) * time.Second).UTC().Format(time.RFC3339)
		assert.True(t, strings.HasPrefix(lines[i], want+"  ok"), "line %d: %s", i, lines[i])
	}
	assert.Equal(t, "Summary: ok=4 refused=0 locked=0 unreachable=0 error=0", lines[4])
}

func TestSoakCountsLockedAttemptsAndFails(t *testing.T) {
	r := newRig(t)
	r.configureAndServe([]agent.Allow{agent.AllowSigning}, testfakes.LockedOpener)

	code, out, _ := r.run("test", "ops", "--soak", "2s", "--interval", "1s")

	assert.Equal(t, command.ExitFailure, code)
	assert.Contains(t, out, "Summary: ok=0 refused=0 locked=3 unreachable=0 error=0")
}

func TestSoakStopsWhenTheContextEnds(t *testing.T) {
	r := newRig(t)
	r.configureAndServe([]agent.Allow{agent.AllowSigning}, testfakes.FileOpener)
	r.deps.Sleep = func(context.Context, time.Duration) error { return context.Canceled }

	code, out, errOut := r.run("test", "ops", "--soak", "1h")

	assert.Equal(t, command.ExitFailure, code)
	assert.Contains(t, out, "Summary: ok=1")
	assert.Contains(t, errOut, "context canceled")
}

func TestTestRejectsAnUnsafeIdentityName(t *testing.T) {
	code, _, errOut := newRig(t).run("test", "../x")

	assert.Equal(t, command.ExitFailure, code)
	assert.Contains(t, errOut, "invalid identity name")
}
