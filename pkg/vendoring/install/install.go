package install

import (
	"context"
	"fmt"
	"os"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
)

const tempDirPermissions = 0o700

const (
	// LockEnforcementSilent re-fetches a drifted package with no reporting -- today's exact
	// behavior, preserved byte-for-byte as one of the three enforcement levels.
	LockEnforcementSilent = "silent"
	// LockEnforcementWarn re-fetches a drifted package and prints one warning per package naming
	// why it drifted. The default when vendor.lock.enforcement is unset.
	LockEnforcementWarn = "warn"
	// LockEnforcementStrict refuses to run (before any fetch/copy/write) when a drifted package is
	// found and --refresh-lock was not explicitly passed.
	LockEnforcementStrict = "strict"
)

// InstallOptions configures Install and FilterPending. A single struct rather than adjacent
// bool parameters: this repo's Options Pattern mandate applies at two or more adjacent
// same-typed parameters, which DryRun/RefreshLock were before this refactor (see
// ExecuteComponentVendorInternal's pre-refactor `dryRun bool, refreshLock bool` signature).
type InstallOptions struct {
	// DryRun performs only the side effects a real fetch would also trigger for
	// go-getter-unsupported URI schemes (custom Git detection), without writing anything.
	DryRun bool
	// RefreshLock bypasses FilterPending's materialization check, forcing every package back
	// through Install even when an existing vendor.lock.yaml receipt says it's unchanged.
	RefreshLock bool
	// LockEnforcement is one of LockEnforcementSilent/Warn/Strict, governing how FilterPending
	// reacts to a drifted package. Empty is treated as LockEnforcementWarn, the config default.
	LockEnforcement string
}

// Result reports the outcome of installing a single VendorPackage.
type Result struct {
	Name string
	Err  error
}

// Install installs pkg into its declared target, or -- when opts.DryRun is set -- performs only
// the dry-run side effect (custom-detector probing) without writing anything. It is a plain,
// synchronous function with no Bubble Tea dependency: directly unit-testable, and reusable by any
// future non-interactive/CI-only call path. The TUI (internal/exec/vendor_model.go) calls this
// from a thin tea.Cmd closure that translates the returned Result into its own tea.Msg.
func Install(atmosConfig *schema.AtmosConfiguration, pkg VendorPackage, opts InstallOptions) (Result, error) {
	defer perf.Track(atmosConfig, "install.Install")()

	ctx := context.Background()

	if opts.DryRun {
		if err := pkg.installer.dryRunCheck(ctx, atmosConfig); err != nil {
			return Result{Name: pkg.Name, Err: err}, nil
		}
		return Result{Name: pkg.Name}, nil
	}

	tempDir, err := createTempDir()
	if err != nil {
		return Result{Name: pkg.Name, Err: fmt.Errorf("%s: %w", pkg.Name, err)}, nil
	}
	defer removeTempDir(tempDir)

	if err := pkg.installer.install(ctx, tempDir, atmosConfig); err != nil {
		return Result{Name: pkg.Name, Err: fmt.Errorf("%s: %w", pkg.Name, err)}, nil
	}
	return Result{Name: pkg.Name}, nil
}

// FilterPending drops every package an existing vendor.lock.yaml receipt already proves is
// unchanged, leaving only the packages Install still needs to fetch. When opts.DryRun or
// opts.RefreshLock is set, the materialization check is skipped entirely and every package is
// returned as-is (matching the pre-unification "if !dryRun && !refreshLock { filter... }" guard
// duplicated at all three call sites this replaces).
//
// For every drifted (non-materialized) package, opts.LockEnforcement governs what happens next:
//   - LockEnforcementSilent: the package is added to pending with no reporting -- the only level
//     whose observable behavior matches this function before enforcement levels existed.
//   - LockEnforcementWarn (the default, including "" and any unrecognized value): the package is
//     added to pending, and one ui.Warningf line is printed naming the package and why it drifted.
//   - LockEnforcementStrict: the package is withheld from pending and its name+reason are
//     collected. If any package drifted, FilterPending returns ErrLockDriftBlocked (wrapping every
//     collected package+reason) instead of a partial pending list, so a strict caller never fetches
//     anything on a run it's about to fail.
func FilterPending(atmosConfig *schema.AtmosConfiguration, packages []VendorPackage, opts InstallOptions) ([]VendorPackage, error) {
	defer perf.Track(atmosConfig, "install.FilterPending")()

	if opts.DryRun || opts.RefreshLock {
		return packages, nil
	}

	enforcement := opts.LockEnforcement
	if enforcement == "" {
		enforcement = LockEnforcementWarn
	}

	pending := make([]VendorPackage, 0, len(packages))
	var drifted []string
	for _, pkg := range packages {
		check, err := pkg.installer.isMaterialized(atmosConfig)
		if err != nil {
			return nil, fmt.Errorf("verify vendor lock for %s: %w", pkg.Name, err)
		}
		if check.Materialized {
			log.Debug("Vendor target matches immutable lock receipt; skipping download", "package", pkg.Name, "target", pkg.Target())
			continue
		}
		if blocked := applyLockEnforcement(enforcement, pkg, check, &pending); blocked != "" {
			drifted = append(drifted, blocked)
		}
	}

	if len(drifted) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrLockDriftBlocked, strings.Join(drifted, "; "))
	}

	return pending, nil
}

// applyLockEnforcement applies one drifted package's enforcement-level outcome: appended to
// pending for LockEnforcementSilent/Warn (warning printed for the latter), or returned as a
// "name (reason)" description for LockEnforcementStrict, so the caller can collect every blocked
// package before failing the whole run at once. Returns "" for every level except strict.
func applyLockEnforcement(enforcement string, pkg VendorPackage, check lockfile.MaterializationCheck, pending *[]VendorPackage) string {
	switch enforcement {
	case LockEnforcementStrict:
		return fmt.Sprintf("%s (%s)", pkg.Name, check.Reason)
	case LockEnforcementSilent:
		*pending = append(*pending, pkg)
	default: // LockEnforcementWarn, and any unrecognized value.
		ui.Warningf("Vendor lock drift detected for %s: %s", pkg.Name, check.Reason)
		*pending = append(*pending, pkg)
	}
	return ""
}

func createTempDir() (string, error) {
	tempDir, err := os.MkdirTemp("", "atmos-vendor")
	if err != nil {
		return "", err
	}
	if err := os.Chmod(tempDir, tempDirPermissions); err != nil {
		return "", err
	}
	return tempDir, nil
}

func removeTempDir(path string) {
	if err := os.RemoveAll(path); err != nil {
		log.Warn(err.Error())
	}
}
