package hooks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Install writes configured hook shims, preserving custom hooks unless force is set.
// Forced replacement backs up recognized pre-commit framework hooks first.
func Install(ctx context.Context, cfg *schema.GitConfig, names []string, force bool) error {
	defer perf.Track(nil, "hooks.Install")()

	// Validate requested names against config before touching the filesystem.
	if err := validateHookNames(names, cfg); err != nil {
		return err
	}

	hookNames := hookNamesOrConfigured(names, cfg)
	if len(hookNames) == 0 {
		ui.Info("No hooks configured under git.hooks in atmos.yaml.")
		return nil
	}

	// Reject broken hook configuration before writing any shim, so a shim never exists for a
	// hook that fails on every Git operation.
	if err := validateHooks(hookNames, cfg); err != nil {
		return err
	}
	warnUnknownGitHookNames(hookNames)

	// Preserve hook directories managed by another tool, even with --force.
	if err := validateHooksPath(ctx); err != nil {
		return err
	}

	hooksDir, err := resolveHooksDir(ctx)
	if err != nil {
		return err
	}

	for _, name := range hookNames {
		if err := installHook(hooksDir, name, force); err != nil {
			return err
		}
	}

	return nil
}

// warnUnknownGitHookNames warns about configured names that Git never invokes.
func warnUnknownGitHookNames(names []string) {
	for _, name := range names {
		if !IsKnownGitHookName(name) {
			ui.Warningf("%q is not a Git hook name, so Git will never run it. Check git.hooks for a typo.", name)
		}
	}
}

// installHook writes a single shim file for hookName into hooksDir.
func installHook(hooksDir, hookName string, force bool) error {
	if err := ValidateShimName(hookName); err != nil {
		return err
	}

	dest := filepath.Join(hooksDir, hookName)
	shim := ShimContentFor(hookName, installedAtmosPath())

	unchanged, err := prepareExistingHook(dest, shim, force)
	if err != nil || unchanged {
		return err
	}

	// Ensure the hooks directory exists.
	if mkErr := os.MkdirAll(hooksDir, hooksDirPerm); mkErr != nil {
		return fmt.Errorf("creating hooks directory %q: %w", hooksDir, mkErr)
	}

	if writeErr := os.WriteFile(dest, []byte(shim), shimPerm); writeErr != nil {
		return fmt.Errorf("writing hook shim %q: %w", dest, writeErr)
	}
	if chmodErr := os.Chmod(dest, shimPerm); chmodErr != nil {
		return fmt.Errorf("setting executable permission on hook shim %q: %w", dest, chmodErr)
	}

	ui.Successf("Installed hook shim: %s", dest)
	return nil
}

// resolveHooksDir returns the absolute path to the .git/hooks directory,
// correctly handling linked worktrees where hooks live in the common dir.
func resolveHooksDir(ctx context.Context) (string, error) {
	defer perf.Track(nil, "hooks.resolveHooksDir")()

	runner := atmosgit.NewExecRunner()
	result, err := runner.Run(ctx, "git", []string{"rev-parse", "--git-path", "hooks"}, atmosgit.RunOptions{})
	if err != nil {
		return "", errUtils.Build(errUtils.ErrNotInGitRepository).
			WithHint("Run 'atmos git hooks install' from inside a Git repository.").
			WithExitCode(2).
			Err()
	}

	dir := strings.TrimSpace(result.Stdout)
	if dir == "" {
		return "", errUtils.Build(errUtils.ErrNotInGitRepository).
			WithHint("'git rev-parse --git-path hooks' returned an empty path.").
			WithExitCode(2).
			Err()
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolving hooks directory path %q: %w", dir, err)
	}

	return abs, nil
}

// validateHooksPath rejects externally managed hook directories before any writes.
// Git's --git-path hooks honors core.hooksPath, so resolving it without this guard
// could install Atmos shims into another tool's directory.
func validateHooksPath(ctx context.Context) error {
	result, err := atmosgit.NewExecRunner().Run(ctx, "git", []string{"config", "--get", "core.hooksPath"}, atmosgit.RunOptions{})
	if err != nil {
		if result.ExitCode == 1 {
			return nil // The configuration key is absent.
		}
		return err
	}
	if hooksPath := strings.TrimSpace(result.Stdout); hooksPath != "" {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHintf("core.hooksPath is configured as %q; no hook files were changed.", hooksPath).
			WithHint("Reconcile the existing hook manager and core.hooksPath setting before installing Atmos hooks; --force does not override this setting.").
			WithExitCode(2).Err()
	}
	return nil
}

// validateHookNames ensures all requested hook names are configured.
func validateHookNames(names []string, cfg *schema.GitConfig) error {
	if len(names) == 0 {
		return nil
	}

	for _, name := range names {
		if cfg == nil || cfg.Hooks == nil {
			return NotConfiguredError(name, nil)
		}
		if _, ok := cfg.Hooks[name]; !ok {
			return NotConfiguredError(name, cfg.Hooks)
		}
	}

	return nil
}

func backupPreCommitHook(path string, data []byte) error {
	backup := path + ".before-atmos-" + time.Now().UTC().Format("20060102T150405.000000000")
	file, err := os.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, shimPerm)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	ui.Infof("Previous hook backed up to %s (hooks are shared by linked worktrees)", backup)
	return nil
}

func prepareExistingHook(dest, shim string, force bool) (bool, error) {
	existing, err := os.ReadFile(dest)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking existing hook %q: %w", dest, err)
	}
	if string(existing) != shim {
		return false, protectExistingHook(dest, existing, force)
	}
	if err := os.Chmod(dest, shimPerm); err != nil {
		return false, fmt.Errorf("setting executable permission on hook shim %q: %w", dest, err)
	}
	ui.Successf("Hook shim already installed: %s", dest)
	return true, nil
}

func protectExistingHook(dest string, existing []byte, force bool) error {
	if strings.Contains(string(existing), ShimMarker) {
		return nil
	}
	if !force {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithHintf("Hook file %q already exists and was not generated by Atmos.", dest).
			WithHint("Pass --force to overwrite it.").WithExitCode(2).Err()
	}
	if strings.Contains(string(existing), "# File generated by pre-commit:") {
		return backupPreCommitHook(dest, existing)
	}
	return nil
}
