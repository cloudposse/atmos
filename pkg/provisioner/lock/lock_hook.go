// Package lock provides the built-in after.terraform.init provisioner that keeps
// .terraform.lock.hcl complete across all configured platforms. A network mirror or the
// default provider plugin cache forces `init` to record host-only checksums; this hook
// runs `providers lock -platform=...` to fill in every configured platform. For ephemeral
// or vendored components (workdir / JIT source) the canonical lock has no committable home,
// so the completed lock is also persisted to a per-instance dotfile (see persist).
package lock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cloudposse/atmos/pkg/cache"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// HookEventAfterTerraformInit fires after a successful `terraform init` (implicit or explicit).
const HookEventAfterTerraformInit = provisioner.HookEvent("after.terraform.init")

func init() {
	// Register the providers-lock provisioner to run after terraform init.
	_ = provisioner.RegisterProvisioner(provisioner.Provisioner{
		Type:      "terraform-providers-lock",
		HookEvent: HookEventAfterTerraformInit,
		Func:      autoLockProviders,
	})
}

// autoLockProviders completes the working directory's .terraform.lock.hcl for every
// configured platform, then persists a committable per-instance copy for ephemeral or
// vendored components. It is a silent no-op unless the gating conditions hold.
func autoLockProviders(
	_ context.Context,
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	_ *schema.AuthContext,
	execCtx *provisioner.TerraformExecContext,
) error {
	defer perf.Track(atmosConfig, "lock.autoLockProviders")()

	// Need a live runner + working dir (provided only by the after.terraform.init dispatch).
	if execCtx == nil || execCtx.Run == nil || execCtx.WorkingDir == "" {
		return nil
	}
	// Only meaningful when the user has declared target platforms beyond the host.
	platforms := lockPlatforms(atmosConfig)
	if len(platforms) == 0 {
		return nil
	}
	// Only needed when a customized provider installation method is active — the default
	// plugin cache or the registry cache — which is what forces the host-only lock. Without
	// one, native init already records the registry's signed cross-platform checksums.
	if !customizedInstallActive(atmosConfig) {
		return nil
	}

	lockPath := filepath.Join(execCtx.WorkingDir, provisioner.CanonicalLockFilename)

	// Complete the canonical lock in place, serialized against concurrent Atmos lock steps
	// for the same directory (e.g. parallel --all sharing one component dir).
	fl := cache.NewFileLock(provisioner.LockCoordPath(lockPath))
	if err := fl.WithLock(func() error {
		return execCtx.Run(append([]string{"providers", "lock"}, platformFlags(platforms)...))
	}); err != nil {
		return fmt.Errorf("complete provider lock for platforms %v: %w", platforms, err)
	}

	return persist(componentConfig, execCtx.WorkingDir, lockPath)
}

// lockPlatforms returns the configured platforms to lock, or nil when there is nothing to
// do (none configured, or only the host — which native completion already covers).
func lockPlatforms(atmosConfig *schema.AtmosConfiguration) []string {
	host := runtime.GOOS + "_" + runtime.GOARCH
	out := make([]string, 0, len(atmosConfig.Components.Terraform.Platforms))
	for _, p := range atmosConfig.Components.Terraform.Platforms {
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	if len(out) == 1 && out[0] == host {
		return nil
	}
	return out
}

// platformFlags renders each platform as a repeatable -platform= flag.
func platformFlags(platforms []string) []string {
	flags := make([]string, 0, len(platforms))
	for _, p := range platforms {
		flags = append(flags, "-platform="+p)
	}
	return flags
}

// customizedInstallActive reports whether a customized provider installation method is in
// effect (the default plugin cache, or the registry cache) — the condition that forces the
// host-only lock this hook fixes.
func customizedInstallActive(atmosConfig *schema.AtmosConfiguration) bool {
	tf := atmosConfig.Components.Terraform
	if tf.PluginCache {
		return true
	}
	return tf.Cache != nil && tf.Cache.Enabled
}

// persist writes the completed canonical lock to a committable per-instance dotfile for
// ephemeral/vendored components. Plain in-repo components return early: their canonical
// .terraform.lock.hcl is already committed in place.
func persist(componentConfig map[string]any, workingDir, lockPath string) error {
	dest := persistDir(componentConfig, workingDir)
	if dest == "" {
		return nil
	}
	destFile := filepath.Join(dest, provisioner.InstanceLockFilename(componentConfig))
	fl := cache.NewFileLock(provisioner.LockCoordPath(destFile))
	if err := fl.WithLock(func() error {
		return copyLock(lockPath, destFile)
	}); err != nil {
		return fmt.Errorf("persist per-instance lock %q: %w", destFile, err)
	}
	return nil
}

// persistDir resolves the committable destination directory for the per-instance lock, or
// "" when the component is plain in-repo (no persistence needed). For workdir components it
// is the original source dir recorded in workdir metadata; for non-workdir JIT-source
// components it is the vendored working dir itself.
func persistDir(componentConfig map[string]any, workingDir string) string {
	if wp, ok := componentConfig[workdir.WorkdirPathKey].(string); ok && wp != "" {
		md, err := workdir.ReadMetadata(wp)
		if err != nil || md == nil || md.Source == "" {
			log.Debug("Skipping per-instance lock persist: workdir source unknown", "workdir", wp)
			return ""
		}
		if md.SourceType == workdir.SourceTypeRemote {
			log.Debug("Skipping per-instance lock persist: remote workdir source is not a local directory", "workdir", wp, "source", md.Source)
			return ""
		}
		if info, statErr := os.Stat(md.Source); statErr != nil || !info.IsDir() {
			log.Debug("Skipping per-instance lock persist: workdir source is not an existing local directory", "workdir", wp, "source", md.Source, "error", statErr)
			return ""
		}
		return md.Source
	}
	if _, ok := componentConfig["source"]; ok {
		return workingDir
	}
	return ""
}

// copyLock copies the canonical lock to the per-instance dotfile. A missing canonical lock
// is not fatal (providers lock may have produced nothing for a provider-less component).
func copyLock(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read lock %q: %w", src, err)
	}
	if err := os.WriteFile(dst, data, provisioner.LockFilePerm); err != nil {
		return fmt.Errorf("write per-instance lock %q: %w", dst, err)
	}
	return nil
}
