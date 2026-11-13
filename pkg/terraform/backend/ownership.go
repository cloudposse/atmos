package backend

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// The receipt records only the fixed host backend, never a caller-supplied path.
// Keep it separate from provider-generated files and out of module content digests.
const (
	hostBackendReceipt       = ".atmos/host-backend.sha256"
	hostBackendName          = "backend.tf.json"
	hostBackendDirectoryMode = 0o700
)

func backendDigest(root *os.Root) (string, error) {
	info, err := root.Lstat(hostBackendName)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: host backend is not a regular file", errUtils.ErrPluginInvalidConfig)
	}
	data, err := root.ReadFile(hostBackendName)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func recordHostBackend(directory string) error {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	digest, err := backendDigest(root)
	if err != nil {
		return err
	}
	if err := root.MkdirAll(".atmos", hostBackendDirectoryMode); err != nil {
		return err
	}
	return root.WriteFile(hostBackendReceipt, []byte(digest), backendFileMode)
}

// RemoveHostBackend removes only the unchanged file recorded by the host writer.
// Older, unrecorded backends remain user-owned: never infer ownership from a name
// or matching backend settings. Terraform will reject any remaining duplicate.
func RemoveHostBackend(directory string) error {
	defer perf.Track(nil, "backend.RemoveHostBackend")()
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	receipt, err := root.ReadFile(hostBackendReceipt)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	digest, err := backendDigest(root)
	if errors.Is(err, os.ErrNotExist) {
		return root.Remove(hostBackendReceipt)
	}
	if err != nil {
		return err
	}
	if digest != string(receipt) {
		return fmt.Errorf("%w: refusing to remove modified host backend %s", errUtils.ErrPluginInvalidConfig, hostBackendName)
	}
	if err := root.Remove(hostBackendName); err != nil {
		return err
	}
	return root.Remove(hostBackendReceipt)
}
