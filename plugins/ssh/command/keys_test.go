package command_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/command"
	"github.com/cloudposse/atmos/plugins/ssh/statusfile"
)

func TestKeyListWithNoKeys(t *testing.T) {
	code, out, _ := newRig(t).run("key", "list")

	assert.Equal(t, command.ExitOK, code)
	assert.Equal(t, "No keys on this machine.\n", out)
}

func TestKeyListShowsStoreRequirementsFingerprintAndDate(t *testing.T) {
	r := newRig(t)
	b := r.createKey("beta")
	r.createKey("alpha")

	code, out, errOut := r.run("key", "list")

	require.Equal(t, command.ExitOK, code, errOut)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 3)
	assert.Equal(t, []string{"KEY", "STORE", "REQUIRE", "FINGERPRINT", "CREATED"}, strings.Fields(lines[0]))
	alpha := strings.Fields(lines[1])
	assert.Equal(t, "alpha", alpha[0])
	assert.Equal(t, "file", alpha[1])
	assert.Equal(t, "-", alpha[2])
	assert.True(t, strings.HasPrefix(alpha[3], "SHA256:"))
	beta := strings.Fields(lines[2])
	assert.Equal(t, authority.Fingerprint(b), beta[3])
	assert.Equal(t, time.Now().UTC().Format("2006-01-02"), beta[4])
}

func TestKeyShowFormats(t *testing.T) {
	r := newRig(t)
	pub := r.createKey("ops")
	line := authority.AuthorizedKey(pub)

	t.Run("authorized-key is the default", func(t *testing.T) {
		code, out, _ := r.run("key", "show", "ops")
		assert.Equal(t, command.ExitOK, code)
		assert.Equal(t, line+" ops\n", out)
	})
	t.Run("allowed-signers", func(t *testing.T) {
		code, out, _ := r.run("key", "show", "ops", "--format", "allowed-signers")
		assert.Equal(t, command.ExitOK, code)
		assert.Equal(t, `* namespaces="git" `+line+"\n", out)
	})
	t.Run("github", func(t *testing.T) {
		code, out, _ := r.run("key", "show", "ops", "--format", "github")
		assert.Equal(t, command.ExitOK, code)
		assert.Contains(t, out, line)
		assert.Contains(t, out, "https://github.com/settings/ssh/new")
		assert.Contains(t, out, "gh ssh-key add --type signing --title 'ops@testhost (Atmos)'")
		assert.Contains(t, out, "echo '"+line+"' | gh ssh-key add")
	})
	t.Run("json", func(t *testing.T) {
		code, out, _ := r.run("key", "show", "ops", "--format", "json")
		assert.Equal(t, command.ExitOK, code)
		var got map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &got))
		assert.Equal(t, "ops", got["name"])
		assert.Equal(t, "file", got["store"])
		assert.Equal(t, authority.Fingerprint(pub), got["fingerprint"])
		assert.Equal(t, line, got["public_key"])
		assert.Empty(t, got["require"])
	})
}

func TestKeyShowOfAMissingKeyFails(t *testing.T) {
	code, out, errOut := newRig(t).run("key", "show", "ghost")

	assert.Equal(t, command.ExitFailure, code)
	assert.Empty(t, out)
	assert.Contains(t, errOut, `no such key: "ghost"`)
}

func TestKeyShowWithTheWrongStoreFindsNothing(t *testing.T) {
	r := newRig(t)
	r.createKey("ops")

	code, _, _ := r.run("key", "show", "ops", "--store", "secure-enclave")
	assert.Equal(t, command.ExitFailure, code)
	code, _, _ = r.run("key", "show", "ops", "--store", "file")
	assert.Equal(t, command.ExitOK, code)
}

func TestKeyDeleteWithoutYesRefusesAndSaysSo(t *testing.T) {
	r := newRig(t)
	r.createKey("ops")

	code, out, errOut := r.run("key", "delete", "ops")

	assert.Equal(t, command.ExitFailure, code)
	assert.Empty(t, out)
	assert.Contains(t, errOut, "--yes")
	assert.Contains(t, errOut, "refused")
	assert.True(t, r.hasKey("ops"), "nothing was deleted")
}

func TestKeyDeleteRemovesTheKeyAndItsAgentEntryAndLeavesTheReceipt(t *testing.T) {
	r := newRig(t)
	pub := r.createKey("ops")
	r.createKey("other")
	r.configure(agent.Identity{Name: "ops", Store: "file", Key: "ops", Allow: []agent.Allow{agent.AllowSigning}})
	r.configure(agent.Identity{Name: "second", Store: "file", Key: "ops", Allow: []agent.Allow{agent.AllowSigning}})
	r.configure(agent.Identity{Name: "other", Store: "file", Key: "other", Allow: []agent.Allow{agent.AllowSigning}})
	r.saveReceipt(authority.Receipt{Authority: "github", KeyName: "ops", Fingerprint: authority.Fingerprint(pub), Uses: []authority.Use{authority.UseSigning}})

	code, out, errOut := r.run("key", "delete", "ops", "--yes")

	require.Equal(t, command.ExitOK, code, errOut)
	assert.False(t, r.hasKey("ops"))
	assert.True(t, r.hasKey("other"), "other keys are untouched")
	cfg, err := agent.LoadConfig(r.paths.AgentConfigFile())
	require.NoError(t, err)
	require.Len(t, cfg.Identities, 1)
	assert.Equal(t, "other", cfg.Identities[0].Name)
	store, err := authority.NewStore(r.paths.ReceiptsDir())
	require.NoError(t, err)
	_, err = store.Load("github", "ops")
	assert.NoError(t, err, "the receipt stays for the next login or logout to revoke")
	assert.Contains(t, out, "Deleted key \"ops\"")
	assert.Contains(t, out, "ops, second")
	assert.Contains(t, out, "next \"atmos auth login\" or \"atmos auth logout\"")
}

func TestKeyDeleteWithNoAgentConfigurationStillDeletes(t *testing.T) {
	r := newRig(t)
	r.createKey("ops")

	code, _, errOut := r.run("key", "delete", "ops", "--yes")

	assert.Equal(t, command.ExitOK, code, errOut)
	assert.False(t, r.hasKey("ops"))
}

func TestKeyDeleteOfAMissingKeyFails(t *testing.T) {
	code, _, errOut := newRig(t).run("key", "delete", "ghost", "--yes")

	assert.Equal(t, command.ExitFailure, code)
	assert.Contains(t, errOut, "no such key")
}

func TestKeyStatusBuildsRowsFromKeysReceiptsAndStatusFiles(t *testing.T) {
	r := newRig(t)
	r.clock.Advance(0)
	pub := r.createKey("ops")
	created := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	r.saveReceipt(authority.Receipt{
		Authority: "github", Subject: "octocat", KeyName: "ops", Fingerprint: authority.Fingerprint(pub),
		Uses: []authority.Use{authority.UseSigning, authority.UseAuthentication}, Created: created,
	})
	require.NoError(t, statusfile.Write(r.paths, statusfile.Status{
		Identity: "ops", Operation: statusfile.OperationAuthenticate, Time: r.clock.Now(), Success: true,
		Outcome: "enrolled", Key: "ops", Store: "file", Authority: "github",
	}))
	// A login that failed before the key existed.
	require.NoError(t, statusfile.Write(r.paths, statusfile.Status{
		Identity: "broken", Operation: statusfile.OperationAuthenticate, Time: r.clock.Now(), Outcome: "failed",
		Key: "broken", Store: "file", Authority: "github", Code: "missing-scope",
		Error: "the token lacks the write:ssh_signing_key scope",
	}))
	// An enrollment whose key was deleted.
	r.saveReceipt(authority.Receipt{
		Authority: "github", Subject: "octocat", KeyName: "old", Fingerprint: "SHA256:gone",
		Uses: []authority.Use{authority.UseSigning}, Created: created,
		ExpiresAt: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	code, out, errOut := r.run("key", "status")

	require.Equal(t, command.ExitOK, code, errOut)
	rows := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n")[1:] {
		f := strings.Fields(line)
		if len(f) > 0 && !strings.HasPrefix(line, "Last error") && len(f) >= 8 {
			rows[f[0]] = f
		}
	}
	header := strings.Fields(strings.SplitN(out, "\n", 2)[0])
	assert.Equal(t, []string{"KEY", "STORE", "REQUIRE", "AUTHORITY", "SUBJECT", "ALLOW", "SINCE", "EXPIRES", "LAST", "RESULT"}, header)

	ops := rows["ops"]
	require.NotNil(t, ops, out)
	assert.Equal(t, []string{"ops", "file", "-", "github", "octocat", "signing,authentication", "2026-11-01", "never", "enrolled"}, ops[:9])

	broken := rows["broken"]
	require.NotNil(t, broken, out)
	assert.Equal(t, []string{"broken", "file", "-", "github", "-", "-", "-", "-", "failed"}, broken[:9])
	assert.Contains(t, out, "Last error for broken: the token lacks the write:ssh_signing_key scope")

	old := rows["old"]
	require.NotNil(t, old, out)
	assert.Equal(t, "2027-01-01T00:00:00Z", old[7])
	assert.Contains(t, out, "key deleted; enrollment withdrawn at next login or logout")
}

func TestKeyStatusMergesADeletedKeysReceiptWithItsLastLogin(t *testing.T) {
	r := newRig(t)
	r.saveReceipt(authority.Receipt{Authority: "github", KeyName: "ops", Fingerprint: "SHA256:gone", Uses: []authority.Use{authority.UseSigning}})
	require.NoError(t, statusfile.Write(r.paths, statusfile.Status{
		Identity: "ops", Operation: statusfile.OperationAuthenticate, Time: r.clock.Now(), Outcome: "enrolled", Key: "ops", Store: "file",
	}))

	_, out, _ := r.run("key", "status")

	assert.Equal(t, 1, strings.Count(out, "ops "), "one row for the key: %s", out)
	assert.Contains(t, out, "key deleted, enrollment withdrawn at next login or logout; last: enrolled")
}

func TestKeyStatusOfOneKeyAndOfNone(t *testing.T) {
	r := newRig(t)
	r.createKey("ops")
	r.createKey("other")
	require.NoError(t, statusfile.Write(r.paths, statusfile.Status{Identity: "nick", Key: "ops", Store: "file", Outcome: "unchanged", Operation: statusfile.OperationLogout, Time: r.clock.Now()}))

	_, out, _ := r.run("key", "status", "ops")
	assert.Contains(t, out, "ops")
	assert.NotContains(t, out, "other")
	assert.Contains(t, out, "logout: unchanged", "a logout is labelled")

	_, byIdentity, _ := r.run("key", "status", "nick")
	assert.Contains(t, byIdentity, "ops", "an identity name finds its key's row")

	code, _, errOut := r.run("key", "status", "ghost")
	assert.Equal(t, command.ExitFailure, code)
	assert.Contains(t, errOut, "no such key")

	empty := newRig(t)
	code, out, _ = empty.run("key", "status")
	assert.Equal(t, command.ExitOK, code)
	assert.Contains(t, out, "No keys, receipts, or login results")
}
