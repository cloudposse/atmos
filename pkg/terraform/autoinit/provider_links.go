package autoinit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/terraform/lockfile"
)

const providerRecoveryDirPerm = 0o700

// PrepareProviderInstallation preserves incomplete local packages, removes unusable links, and selects a component-local
// recovery cache when a shared package is incomplete. It never deletes shared package contents:
// Terraform refuses to reinstall over dangling links or import a cache package without a binary.
// Call only after policy permits init; nil inputs (dry run) are a no-op. Invalid lockfiles remain
// the engine's responsibility. The returned cache persists with the data directory so provider
// links created by init remain usable on subsequent warm commands.

func PrepareProviderInstallation(in *Inputs) (string, error) {
	defer perf.Track(nil, "autoinit.PrepareProviderInstallation")()

	if in == nil {
		return "", nil
	}
	providers, err := lockfile.ParseFile(filepath.Join(in.ComponentPath, lockfile.Name))
	if err != nil {
		return "", nil
	}
	recovery := false
	for _, provider := range providers {
		needed, err := prepareProviderLinks(in, provider)
		if err != nil {
			return "", err
		}
		recovery = recovery || needed
	}
	if !recovery {
		return "", nil
	}
	path := filepath.Join(in.effectiveDataDir(), "atmos-provider-recovery")
	if err := os.MkdirAll(path, providerRecoveryDirPerm); err != nil {
		return "", fmt.Errorf("%w: creating provider recovery cache: %w", errUtils.ErrTerraformInit, err)
	}
	entry, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("%w: inspecting provider recovery cache: %w", errUtils.ErrTerraformInit, err)
	}
	if entry.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%w: provider recovery directory must not be a symlink: %s", errUtils.ErrTerraformInit, path)
	}
	return path, nil
}

func providerRepairDirs(dataDir string, provider lockfile.Provider) ([]string, string) {
	parts := strings.Split(provider.Source, "/")
	if len(parts) != 3 {
		return nil, ""
	}
	prefix := "terraform-provider-" + parts[2] + "_v" + provider.Version
	dirs := providerPackageDirs(dataDir, provider)
	if len(dirs) == 0 {
		return nil, ""
	}
	for i, path := range dirs {
		if providerExecutablePresent(path, prefix, i < 2) {
			return nil, ""
		}
	}
	// Never move aggregate legacy plugin directories. Leave alternate installations alone
	// when one already suffices; native engine diagnostics may still require repairing a
	// partial native package before the runtime retry.
	dirs = dirs[:2]
	if !alternativeProviderInstalled(dataDir, provider, prefix) {
		dirs = append(dirs, alternateProviderPackageDirs(dataDir, provider)...)
	}
	return dirs, prefix
}

func prepareProviderLinks(in *Inputs, provider lockfile.Provider) (bool, error) {
	dirs, prefix := providerRepairDirs(in.effectiveDataDir(), provider)
	if len(dirs) == 0 {
		return false, nil
	}
	recovery := false
	for _, path := range dirs {
		needed, err := prepareLocalPackage(in.effectiveDataDir(), path, prefix, true)
		if err != nil {
			return false, err
		}
		recovery = recovery || needed
	}

	if cacheDir := envLookup(in.EnvLookup, envTFPluginCacheDir); cacheDir != "" {
		dirs := providerPackageDirs(cacheDir, provider)
		if len(dirs) > 0 {
			// A shared cache has the registry layout directly under its root, without "providers".
			packagePath := filepath.Join(cacheDir, filepath.FromSlash(provider.Source), provider.Version, filepath.Base(dirs[0]))
			if dirExists(packagePath) && !providerExecutablePresent(packagePath, prefix, true) {
				recovery = true
			}
		}
	}
	return recovery, nil
}

// prepareLocalPackage returns whether init must bypass a shared package missing its executable.
func prepareLocalPackage(dataDir, path, prefix string, versionInPath bool) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%w: inspecting provider package %s: %w", errUtils.ErrTerraformInit, path, err)
	}
	if providerExecutablePresent(path, prefix, versionInPath) {
		return false, nil
	}
	if !packageReadableOrMissing(path) {
		return false, nil // An unreadable package is not evidence that its contents are missing.
	}
	if info.IsDir() && versionInPath {
		return false, preserveIncompletePackage(dataDir, path)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, nil
	}
	sharedPackageExists := dirExists(path)
	// Unlink only the local installation reference; its target can belong to another component.
	if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("%w: removing unusable provider package link %s: %w", errUtils.ErrTerraformInit, path, err)
	}
	return sharedPackageExists, nil
}

// preserveIncompletePackage moves an owned, incomplete package aside for OpenTofu, which otherwise
// refuses to replace it as locally modified. Resolve ancestors before moving anything: a provider
// directory reached through a shared-cache symlink belongs to that cache, not this component.
func preserveIncompletePackage(dataDir, packagePath string) error {
	if !ownedProviderPackage(dataDir, packagePath) {
		return nil
	}
	quarantine, err := os.MkdirTemp(dataDir, "atmos-incomplete-provider-")
	if err != nil {
		return fmt.Errorf("%w: preserving incomplete provider package: %w", errUtils.ErrTerraformInit, err)
	}
	if err := os.Rename(packagePath, filepath.Join(quarantine, "package")); err != nil {
		_ = os.Remove(quarantine)
		return fmt.Errorf("%w: preserving incomplete provider package: %w", errUtils.ErrTerraformInit, err)
	}
	log.Debug("Preserved incomplete provider installation before init", "path", packagePath, "quarantine", quarantine)
	return nil
}

func ownedProviderPackage(dataDir, packagePath string) bool {
	root, rootErr := filepath.EvalSymlinks(dataDir)
	resolved, pathErr := filepath.EvalSymlinks(packagePath)
	if rootErr != nil || pathErr != nil {
		return false
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func packageReadableOrMissing(path string) bool {
	_, err := os.ReadDir(path)
	return err == nil || os.IsNotExist(err)
}
