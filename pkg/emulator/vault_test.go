package emulator

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
)

// fakeExecer is a programmable vaultExecer: respond maps a command to its stdout
// and error, and every invocation is recorded for sequence assertions.
type fakeExecer struct {
	respond func(cmd []string) (string, error)
	calls   [][]string
}

func (f *fakeExecer) Exec(_ context.Context, _ string, cmd []string, opts *container.ExecOptions) error {
	f.calls = append(f.calls, cmd)
	out, err := f.respond(cmd)
	if opts != nil && opts.Stdout != nil {
		_, _ = opts.Stdout.Write([]byte(out))
	}
	return err
}

// cmdContains reports whether the command's joined form contains all substrings.
func cmdContains(cmd []string, subs ...string) bool {
	joined := strings.Join(cmd, " ")
	for _, s := range subs {
		if !strings.Contains(joined, s) {
			return false
		}
	}
	return true
}

const fakeInitJSON = `{"unseal_keys_b64":["KEY=="],"root_token":"s.rootabc"}`

func TestBootstrapVault_FreshInitUnsealEnable(t *testing.T) {
	exec := &fakeExecer{respond: func(cmd []string) (string, error) {
		switch {
		case cmdContains(cmd, "command -v bao"):
			return "/usr/local/bin/bao\n", nil
		case cmdContains(cmd, "status", "-format=json"):
			return `{"initialized":false,"sealed":true}`, nil
		case cmdContains(cmd, "operator", "init"):
			return fakeInitJSON, nil
		case cmdContains(cmd, "printf"): // writing the bootstrap file
			return "", nil
		case cmdContains(cmd, "operator", "unseal"):
			return "", nil
		case cmdContains(cmd, "secrets", "list"):
			return `{}`, nil
		case cmdContains(cmd, "secrets", "enable"):
			return "", nil
		}
		return "", nil
	}}

	require.NoError(t, bootstrapVault(context.Background(), exec, "cid"))

	// The full fresh sequence must run: init, write bootstrap, unseal, enable kv.
	joined := make([]string, len(exec.calls))
	for i, c := range exec.calls {
		joined[i] = strings.Join(c, " ")
	}
	all := strings.Join(joined, "\n")
	assert.Contains(t, all, "operator init")
	assert.Contains(t, all, vaultBootstrapPath)      // wrote the bootstrap file.
	assert.Contains(t, all, "operator unseal KEY==") // unsealed with the init key.
	assert.Contains(t, all, "secrets enable -path=secret kv-v2")
}

func TestBootstrapVault_InitializedSealedReunseals(t *testing.T) {
	var unsealedWith string
	exec := &fakeExecer{respond: func(cmd []string) (string, error) {
		switch {
		case cmdContains(cmd, "command -v bao"):
			return "/usr/local/bin/bao", nil
		case cmdContains(cmd, "status", "-format=json"):
			return `{"initialized":true,"sealed":true}`, nil
		case cmdContains(cmd, "cat"): // read persisted bootstrap.
			return `{"unseal_key":"PERSISTKEY==","root_token":"s.persist"}`, nil
		case cmdContains(cmd, "operator", "unseal"):
			unsealedWith = cmd[len(cmd)-1]
			return "", nil
		case cmdContains(cmd, "secrets", "list"):
			return `{"secret/":{"type":"kv"}}`, nil
		}
		return "", nil
	}}

	require.NoError(t, bootstrapVault(context.Background(), exec, "cid"))
	assert.Equal(t, "PERSISTKEY==", unsealedWith, "must unseal with the persisted key")
	// Must NOT re-initialize an already-initialized server.
	for _, c := range exec.calls {
		assert.False(t, cmdContains(c, "operator", "init"), "must not re-init an initialized server")
	}
}

func TestBootstrapVault_InitializedUnsealedIsNoOp(t *testing.T) {
	exec := &fakeExecer{respond: func(cmd []string) (string, error) {
		switch {
		case cmdContains(cmd, "command -v bao"):
			return "/usr/local/bin/bao", nil
		case cmdContains(cmd, "status", "-format=json"):
			return `{"initialized":true,"sealed":false}`, nil
		case cmdContains(cmd, "cat"):
			return `{"unseal_key":"KEY==","root_token":"s.rootabc"}`, nil
		case cmdContains(cmd, "secrets", "list"):
			return `{"secret/":{"type":"kv"}}`, nil
		}
		return "", nil
	}}

	require.NoError(t, bootstrapVault(context.Background(), exec, "cid"))
	for _, c := range exec.calls {
		assert.False(t, cmdContains(c, "operator"), "an unsealed server needs no init/unseal: %v", c)
		assert.False(t, cmdContains(c, "secrets", "enable"), "existing secret/ engine must not be re-enabled: %v", c)
	}
}

func TestBootstrapVault_InitializedUnsealedEnablesMissingKV(t *testing.T) {
	var enabled bool
	exec := &fakeExecer{respond: func(cmd []string) (string, error) {
		switch {
		case cmdContains(cmd, "command -v bao"):
			return "/usr/local/bin/bao", nil
		case cmdContains(cmd, "status", "-format=json"):
			return `{"initialized":true,"sealed":false}`, nil
		case cmdContains(cmd, "cat"):
			return `{"unseal_key":"KEY==","root_token":"s.rootabc"}`, nil
		case cmdContains(cmd, "secrets", "list"):
			return `{}`, nil
		case cmdContains(cmd, "secrets", "enable", "-path=secret", "kv-v2"):
			enabled = true
			return "", nil
		}
		return "", nil
	}}

	require.NoError(t, bootstrapVault(context.Background(), exec, "cid"))
	assert.True(t, enabled, "initialized unsealed servers still get the secret/ KV engine")
}

func TestVaultRootToken_ParsesBootstrap(t *testing.T) {
	exec := &fakeExecer{respond: func(cmd []string) (string, error) {
		if cmdContains(cmd, "cat", vaultBootstrapPath) {
			return `{"unseal_key":"k","root_token":"s.harvested"}`, nil
		}
		return "", nil
	}}

	token, err := vaultRootToken(context.Background(), exec, "cid")
	require.NoError(t, err)
	assert.Equal(t, "s.harvested", token)
}

func TestVaultRootToken_MissingTokenErrors(t *testing.T) {
	exec := &fakeExecer{respond: func(cmd []string) (string, error) {
		return `{"unseal_key":"k","root_token":""}`, nil
	}}

	_, err := vaultRootToken(context.Background(), exec, "cid")
	require.Error(t, err)
}

func TestBootstrapVault_InitParseErrorFails(t *testing.T) {
	exec := &fakeExecer{respond: func(cmd []string) (string, error) {
		switch {
		case cmdContains(cmd, "command -v bao"):
			return "/usr/local/bin/bao", nil
		case cmdContains(cmd, "status", "-format=json"):
			return `{"initialized":false,"sealed":true}`, nil
		case cmdContains(cmd, "operator", "init"):
			return "not-json", nil
		}
		return "", nil
	}}

	require.Error(t, bootstrapVault(context.Background(), exec, "cid"))
}

func TestVaultCLIBinary_MissingErrors(t *testing.T) {
	exec := &fakeExecer{respond: func(cmd []string) (string, error) {
		return "", nil // no binary on PATH.
	}}

	_, err := vaultCLIBinary(context.Background(), exec, "cid")
	require.Error(t, err)
}
