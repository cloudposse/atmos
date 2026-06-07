package provisioner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/cache"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CanonicalLockFilename is Terraform's fixed dependency lock filename. It is the only lock
// name terraform/tofu read; Atmos treats it as scratch for ephemeral/vendored components and
// keeps the committed truth in the per-instance file (see InstanceLockFilename).
const CanonicalLockFilename = ".terraform.lock.hcl"

// LockFilePerm is the permission for committed/restored lock files (non-sensitive).
const LockFilePerm = 0o644

// LockCoordPath maps a lock-file path to a stable, machine-local coordination path (under the
// temp dir, keyed by the absolute lock path) so the advisory flock sidecar never lands in — and
// pollutes — a committed component directory.
func LockCoordPath(lockPath string) string {
	defer perf.Track(nil, "provisioner.LockCoordPath")()

	abs, err := filepath.Abs(lockPath)
	if err != nil {
		abs = lockPath
	}
	sum := sha256.Sum256([]byte(abs))
	return filepath.Join(os.TempDir(), "atmos-tflock-"+hex.EncodeToString(sum[:8]))
}

// RestorePerInstanceLock seeds workingDir's canonical .terraform.lock.hcl from the committed
// per-instance lock (.<stack>-<component>.terraform.lock.hcl) in srcDir, when one exists, so
// terraform init honors the instance's pinned providers and hashes. It is the pre-init
// counterpart to the after.terraform.init persist step. A missing per-instance lock is a no-op
// (first run for this instance). The write is serialized with a file lock.
func RestorePerInstanceLock(srcDir, workingDir string, componentConfig map[string]any) error {
	defer perf.Track(nil, "provisioner.RestorePerInstanceLock")()

	if srcDir == "" || workingDir == "" {
		return nil
	}
	srcFile := filepath.Join(srcDir, InstanceLockFilename(componentConfig))
	data, err := os.ReadFile(srcFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read per-instance lock %q: %w", srcFile, err)
	}

	dstFile := filepath.Join(workingDir, CanonicalLockFilename)
	fl := cache.NewFileLock(LockCoordPath(dstFile))
	return fl.WithLock(func() error {
		if err := os.WriteFile(dstFile, data, LockFilePerm); err != nil {
			return fmt.Errorf("restore lock %q: %w", dstFile, err)
		}
		return nil
	})
}
