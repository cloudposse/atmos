package emulator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Vault/OpenBao bootstrap constants. The emulator runs OpenBao/Vault with a file
// storage backend (not dev mode) so secrets persist. A fresh server boots SEALED
// and UNINITIALIZED, so the manager initializes it, unseals it, enables the KV v2
// engine, and records the (single) unseal key + dynamic root token in a bootstrap
// file under the data dir. Because that data dir is the bind-mounted XDG cache,
// the bootstrap survives `down`/`up`; `reset` wipes it with the rest of the state.
const (
	// The vaultInternalAddr is the in-container API address (the file-backend
	// listener binds 0.0.0.0:8200 with TLS disabled).
	vaultInternalAddr = "http://127.0.0.1:8200"
	// The vaultDataDir is the in-container file-storage path (also the persistence dir).
	vaultDataDir = "/openbao/file"
	// The vaultBootstrapPath is where the unseal key + root token are recorded,
	// inside the data dir so it persists on the bind mount and is wiped by `reset`.
	vaultBootstrapPath = vaultDataDir + "/atmos-bootstrap.json"
	// The vaultReadyTimeout bounds how long we wait for the server to respond.
	vaultReadyTimeout = 60 * time.Second
	// The vaultReadyInterval is the poll interval while waiting for readiness.
	vaultReadyInterval = time.Second
)

// vaultStatus is the subset of `operator`/`status` JSON the bootstrap inspects.
type vaultStatus struct {
	Initialized bool `json:"initialized"`
	Sealed      bool `json:"sealed"`
}

// vaultInitResult is the subset of `operator init -format=json` output we keep.
type vaultInitResult struct {
	UnsealKeysB64 []string `json:"unseal_keys_b64"`
	RootToken     string   `json:"root_token"`
}

// vaultBootstrap is the persisted unseal key + root token (written under the data
// dir so it survives restarts and is removed by `reset`).
type vaultBootstrap struct {
	UnsealKey string `json:"unseal_key"`
	RootToken string `json:"root_token"`
}

// vaultExecer is the subset of container.Runtime the vault bootstrap needs (Exec).
// It keeps the helpers testable and the dependency explicit.
type vaultExecer interface {
	Exec(ctx context.Context, containerID string, cmd []string, opts *container.ExecOptions) error
}

// bootstrapVault makes a running Vault/OpenBao file-backend server ready to use:
// it initializes + unseals a fresh server (recording the key/token and enabling
// KV v2 at secret/), or re-unseals an already-initialized one from the recorded
// bootstrap. It is idempotent and a no-op once the server is initialized+unsealed.
func bootstrapVault(ctx context.Context, runtime vaultExecer, containerID string) error {
	defer perf.Track(nil, "emulator.bootstrapVault")()

	bin, err := vaultCLIBinary(ctx, runtime, containerID)
	if err != nil {
		return err
	}
	status, err := waitVaultResponding(ctx, runtime, containerID, bin)
	if err != nil {
		return err
	}
	if !status.Initialized {
		return initAndUnsealVault(ctx, runtime, containerID, bin)
	}
	if status.Sealed {
		boot, err := unsealFromBootstrap(ctx, runtime, containerID, bin)
		if err != nil {
			return err
		}
		return ensureVaultKV2(ctx, runtime, containerID, bin, boot.RootToken)
	}
	boot, err := readVaultBootstrap(ctx, runtime, containerID)
	if err != nil {
		return err
	}
	return ensureVaultKV2(ctx, runtime, containerID, bin, boot.RootToken)
}

// vaultRootToken reads the persisted root token from the running container's
// bootstrap file so callers (Resolve, the !emulator function, stores) can
// authenticate against the file-backed server.
func vaultRootToken(ctx context.Context, runtime vaultExecer, containerID string) (string, error) {
	defer perf.Track(nil, "emulator.vaultRootToken")()

	boot, err := readVaultBootstrap(ctx, runtime, containerID)
	if err != nil {
		return "", err
	}
	if boot.RootToken == "" {
		return "", fmt.Errorf("%w: vault bootstrap has no root token", errUtils.ErrEmulatorConfigInvalid)
	}
	return boot.RootToken, nil
}

// vaultCLIBinary resolves the in-container CLI binary: OpenBao ships `bao`,
// HashiCorp Vault ships `vault`.
func vaultCLIBinary(ctx context.Context, runtime vaultExecer, containerID string) (string, error) {
	// `command -v` exits non-zero when a binary is absent, so the exit error is
	// expected and ignored; the resolved path (if any) is on stdout.
	out, _ := execCapture(ctx, runtime, containerID, nil,
		[]string{"sh", "-c", "command -v bao 2>/dev/null || command -v vault 2>/dev/null"})
	bin := strings.TrimSpace(out)
	if bin == "" {
		return "", fmt.Errorf("%w: no vault/bao CLI found in emulator container", errUtils.ErrEmulatorConfigInvalid)
	}
	return bin, nil
}

// waitVaultResponding polls `status` until the server answers (sealed is fine),
// returning its parsed status. The status command exits non-zero when sealed, so
// the exit error is ignored in favor of parsing stdout.
func waitVaultResponding(ctx context.Context, runtime vaultExecer, containerID, bin string) (vaultStatus, error) {
	deadline := time.Now().Add(vaultReadyTimeout)
	for {
		out, _ := execCapture(ctx, runtime, containerID, vaultAddrEnv(), []string{bin, "status", "-format=json"})
		var status vaultStatus
		if json.Unmarshal([]byte(out), &status) == nil && strings.Contains(out, "initialized") {
			return status, nil
		}
		if ctx.Err() != nil {
			return vaultStatus{}, ctx.Err()
		}
		if time.Now().After(deadline) {
			return vaultStatus{}, fmt.Errorf("%w: vault/openbao server did not respond within %s", errUtils.ErrEmulatorNotRunning, vaultReadyTimeout)
		}
		select {
		case <-ctx.Done():
			return vaultStatus{}, ctx.Err()
		case <-time.After(vaultReadyInterval):
		}
	}
}

// initAndUnsealVault initializes a fresh server, records the key/token, unseals
// it, and enables the KV v2 engine at secret/.
func initAndUnsealVault(ctx context.Context, runtime vaultExecer, containerID, bin string) error {
	out, err := execCapture(ctx, runtime, containerID, vaultAddrEnv(),
		[]string{bin, "operator", "init", "-key-shares=1", "-key-threshold=1", "-format=json"})
	if err != nil {
		return fmt.Errorf("%w: vault operator init: %w", errUtils.ErrEmulatorConfigInvalid, err)
	}
	var result vaultInitResult
	if jErr := json.Unmarshal([]byte(out), &result); jErr != nil || len(result.UnsealKeysB64) == 0 || result.RootToken == "" {
		return fmt.Errorf("%w: parse vault init output", errUtils.ErrEmulatorConfigInvalid)
	}
	boot := vaultBootstrap{UnsealKey: result.UnsealKeysB64[0], RootToken: result.RootToken}
	if wErr := writeVaultBootstrap(ctx, runtime, containerID, boot); wErr != nil {
		return wErr
	}
	if uErr := unsealVault(ctx, runtime, containerID, bin, boot.UnsealKey); uErr != nil {
		return uErr
	}
	return ensureVaultKV2(ctx, runtime, containerID, bin, boot.RootToken)
}

// unsealFromBootstrap unseals an already-initialized server using the recorded
// unseal key.
func unsealFromBootstrap(ctx context.Context, runtime vaultExecer, containerID, bin string) (vaultBootstrap, error) {
	boot, err := readVaultBootstrap(ctx, runtime, containerID)
	if err != nil {
		return vaultBootstrap{}, err
	}
	if err := unsealVault(ctx, runtime, containerID, bin, boot.UnsealKey); err != nil {
		return vaultBootstrap{}, err
	}
	return boot, nil
}

// ensureVaultKV2 enables the secret/ KV v2 engine if it is not already present.
func ensureVaultKV2(ctx context.Context, runtime vaultExecer, containerID, bin, token string) error {
	if token == "" {
		return fmt.Errorf("%w: vault bootstrap has no root token", errUtils.ErrEmulatorConfigInvalid)
	}
	out, err := execCapture(ctx, runtime, containerID, vaultTokenEnv(token), []string{bin, "secrets", "list", "-format=json"})
	if err != nil {
		return fmt.Errorf("%w: list vault secrets engines: %w", errUtils.ErrEmulatorConfigInvalid, err)
	}
	var mounts map[string]any
	if jErr := json.Unmarshal([]byte(out), &mounts); jErr != nil {
		return fmt.Errorf("%w: parse vault secrets engines: %w", errUtils.ErrEmulatorConfigInvalid, jErr)
	}
	if _, ok := mounts["secret/"]; ok {
		return nil
	}
	if _, eErr := execCapture(ctx, runtime, containerID, vaultTokenEnv(token),
		[]string{bin, "secrets", "enable", "-path=secret", "kv-v2"}); eErr != nil {
		return fmt.Errorf("%w: enable kv-v2 engine: %w", errUtils.ErrEmulatorConfigInvalid, eErr)
	}
	return nil
}

func unsealVault(ctx context.Context, runtime vaultExecer, containerID, bin, key string) error {
	if key == "" {
		return fmt.Errorf("%w: vault bootstrap has no unseal key", errUtils.ErrEmulatorConfigInvalid)
	}
	if _, err := execCapture(ctx, runtime, containerID, vaultAddrEnv(), []string{bin, "operator", "unseal", key}); err != nil {
		return fmt.Errorf("%w: vault operator unseal: %w", errUtils.ErrEmulatorConfigInvalid, err)
	}
	return nil
}

// writeVaultBootstrap records the unseal key + root token under the data dir. The
// content is passed as an argv parameter (not stdin) so it never needs shell
// quoting. The file is created world-readable (umask 022 -> 0644) so the host can
// read it through the bind mount even when the container process runs as a
// different uid — the same rationale as the k3s kubeconfig (the container writes
// it; the host reads the root-written, bind-mounted file). It holds a throwaway
// local root token and is wiped by `atmos emulator reset`.
func writeVaultBootstrap(ctx context.Context, runtime vaultExecer, containerID string, boot vaultBootstrap) error {
	payload, err := json.Marshal(boot)
	if err != nil {
		return err
	}
	cmd := []string{"sh", "-c", `umask 022; printf '%s' "$1" > ` + vaultBootstrapPath, "sh", string(payload)}
	if _, err := execCapture(ctx, runtime, containerID, nil, cmd); err != nil {
		return fmt.Errorf("%w: record vault bootstrap: %w", errUtils.ErrEmulatorConfigInvalid, err)
	}
	return nil
}

// readVaultBootstrap reads and parses the recorded bootstrap file.
func readVaultBootstrap(ctx context.Context, runtime vaultExecer, containerID string) (vaultBootstrap, error) {
	out, err := execCapture(ctx, runtime, containerID, nil, []string{"cat", vaultBootstrapPath})
	if err != nil {
		return vaultBootstrap{}, fmt.Errorf("%w: read vault bootstrap: %w", errUtils.ErrEmulatorConfigInvalid, err)
	}
	var boot vaultBootstrap
	if jErr := json.Unmarshal([]byte(out), &boot); jErr != nil {
		return vaultBootstrap{}, fmt.Errorf("%w: parse vault bootstrap: %w", errUtils.ErrEmulatorConfigInvalid, jErr)
	}
	return boot, nil
}

// execCapture runs a command in the container and returns its stdout. The exit
// error is returned too; callers that tolerate non-zero exits (e.g. `status`)
// ignore it and parse stdout.
func execCapture(ctx context.Context, runtime vaultExecer, containerID string, env, cmd []string) (string, error) {
	var stdout bytes.Buffer
	err := runtime.Exec(ctx, containerID, cmd, &container.ExecOptions{
		Env:          env,
		AttachStdout: true,
		Stdout:       &stdout,
	})
	return stdout.String(), err
}

// vaultAddrEnv points the CLI at the in-container HTTP listener (both the OpenBao
// and Vault env var names, since one path serves both images).
func vaultAddrEnv() []string {
	return []string{"BAO_ADDR=" + vaultInternalAddr, "VAULT_ADDR=" + vaultInternalAddr}
}

// vaultTokenEnv adds the root token to the address env for authenticated calls.
func vaultTokenEnv(token string) []string {
	return append(vaultAddrEnv(), "BAO_TOKEN="+token, "VAULT_TOKEN="+token)
}
