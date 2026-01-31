package version

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	// ReexecGuardEnvVar prevents infinite re-exec loops.
	ReexecGuardEnvVar = "ATMOS_REEXEC_GUARD"

	// VersionEnvVar is the environment variable for specifying the Atmos version to use.
	// This is a convenience alias that matches common conventions (e.g., tfenv, goenv).
	VersionEnvVar = "ATMOS_VERSION"

	// VersionUseEnvVar is the environment variable for specifying the Atmos version.
	// This matches the config path `version.use` in atmos.yaml.
	// The --use-version flag also sets this env var.
	VersionUseEnvVar = "ATMOS_VERSION_USE"

	// LogFieldPR is the log field name for PR numbers.
	logFieldPR = "pr"

	// LogFieldSHA is the log field name for commit SHAs.
	logFieldSHA = "sha"
)

// VersionFinder is an interface for finding and installing atmos versions.
type VersionFinder interface {
	FindBinaryPath(owner, repo, version string, binaryName ...string) (string, error)
}

// VersionInstaller is an interface for installing atmos versions.
type VersionInstaller interface {
	Install(toolSpec string, force, allowPrereleases bool) error
}

// ExecFunc is the function signature for syscall.Exec.
type ExecFunc func(argv0 string, argv []string, envv []string) error

// ReexecConfig holds dependencies for version re-execution.
type ReexecConfig struct {
	Finder    VersionFinder
	Installer VersionInstaller
	ExecFn    ExecFunc
	GetEnv    func(string) string
	SetEnv    func(string, string) error
	Args      []string
	Environ   func() []string
}

// DefaultReexecConfig returns the default production configuration.
func DefaultReexecConfig() *ReexecConfig {
	defer perf.Track(nil, "version.DefaultReexecConfig")()

	installer := toolchain.NewInstaller()
	return &ReexecConfig{
		Finder:    installer,
		Installer: &defaultInstaller{},
		ExecFn:    syscall.Exec,
		GetEnv:    getEnvWrapper,
		SetEnv:    os.Setenv,
		Args:      os.Args,
		Environ:   os.Environ,
	}
}

// getEnvWrapper wraps os.Getenv for use in ReexecConfig.
// This allows forbidigo exclusion while maintaining DI testability.
//
//nolint:forbidigo // Intentional os.Getenv wrapper for DI testing pattern.
func getEnvWrapper(key string) string {
	return os.Getenv(key)
}

// defaultInstaller wraps toolchain.RunInstall.
type defaultInstaller struct{}

// Install implements VersionInstaller by delegating to toolchain.RunInstall.
func (d *defaultInstaller) Install(toolSpec string, force, allowPrereleases bool) error {
	defer perf.Track(nil, "version.defaultInstaller.Install")()

	// Suppress PATH hint for --use-version installs (automatic version switching), but show progress bar.
	return toolchain.RunInstall(toolSpec, force, allowPrereleases, false, true)
}

// CheckAndReexec checks if version.use is configured and re-executes with the specified version.
// This should be called after config/profiles are loaded but before command execution.
// Returns true if re-exec was triggered (caller should exit), false otherwise.
func CheckAndReexec(atmosConfig *schema.AtmosConfiguration) bool {
	defer perf.Track(atmosConfig, "version.CheckAndReexec")()

	return CheckAndReexecWithConfig(atmosConfig, DefaultReexecConfig())
}

// CheckAndReexecWithConfig checks if version.use is configured and re-executes with the specified version.
// This variant accepts a ReexecConfig for testability.
func CheckAndReexecWithConfig(atmosConfig *schema.AtmosConfiguration, cfg *ReexecConfig) bool {
	defer perf.Track(atmosConfig, "version.CheckAndReexecWithConfig")()

	requestedVersion := resolveRequestedVersion(atmosConfig, cfg)
	if requestedVersion == "" {
		return false
	}

	if shouldSkipReexec(requestedVersion, cfg) {
		return false
	}

	return executeVersionSwitch(requestedVersion, cfg)
}

// resolveRequestedVersion determines the version to use with precedence:
// ATMOS_VERSION_USE > ATMOS_VERSION > version.use in config.
func resolveRequestedVersion(atmosConfig *schema.AtmosConfiguration, cfg *ReexecConfig) string {
	if v := cfg.GetEnv(VersionUseEnvVar); v != "" {
		return v
	}
	if v := cfg.GetEnv(VersionEnvVar); v != "" {
		return v
	}
	return atmosConfig.Version.Use
}

// shouldSkipReexec checks if re-exec should be skipped due to guard or version match.
func shouldSkipReexec(requestedVersion string, cfg *ReexecConfig) bool {
	// Check re-exec guard to prevent infinite loops.
	if cfg.GetEnv(ReexecGuardEnvVar) == requestedVersion {
		log.Debug("Re-exec guard active, skipping version switch",
			"requested_version", requestedVersion,
			"guard_value", cfg.GetEnv(ReexecGuardEnvVar))
		return true
	}

	// PR versions (pr:NNNN or just digits) always need re-exec - never skip.
	if _, isPR := toolchain.IsPRVersion(requestedVersion); isPR {
		log.Debug("PR version requested, will re-exec", "requested", requestedVersion)
		return false
	}

	// SHA versions (sha:ceb7526 or hex strings) always need re-exec - never skip.
	if _, isSHA := toolchain.IsSHAVersion(requestedVersion); isSHA {
		log.Debug("SHA version requested, will re-exec", "requested", requestedVersion)
		return false
	}

	// Normalize versions for comparison (strip 'v' prefix).
	currentVersion := strings.TrimPrefix(Version, "v")
	targetVersion := strings.TrimPrefix(requestedVersion, "v")

	// If versions match, no re-exec needed.
	if currentVersion == targetVersion {
		log.Debug("Current version matches requested version",
			"current", currentVersion,
			"requested", targetVersion)
		return true
	}

	log.Debug("Version mismatch, will re-exec",
		"current", currentVersion,
		"requested", targetVersion)
	return false
}

// executeVersionSwitch performs the actual version switch.
//
//nolint:revive // os.Exit is intentional for hard failures on PR/SHA version errors.
func executeVersionSwitch(requestedVersion string, cfg *ReexecConfig) bool {
	targetVersion := strings.TrimPrefix(requestedVersion, "v")

	// Find or install the requested version.
	binaryPath, err := findOrInstallVersionWithConfig(targetVersion, cfg)
	if err != nil {
		// For PR versions, fail hard - don't continue with wrong version.
		if _, isPR := toolchain.IsPRVersion(requestedVersion); isPR {
			formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
			ui.Errorf("%s", formatted)
			os.Exit(1)
		}

		// For SHA versions, fail hard - don't continue with wrong version.
		if _, isSHA := toolchain.IsSHAVersion(requestedVersion); isSHA {
			formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
			ui.Errorf("%s", formatted)
			os.Exit(1)
		}

		// Check if this is an invalid version format error - fail hard for those too.
		_, _, parseErr := toolchain.ParseVersionSpec(requestedVersion)
		if parseErr != nil {
			formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
			ui.Errorf("%s", formatted)
			os.Exit(1)
		}

		// For regular semver versions, fall back to current (existing behavior).
		ui.Warningf("Failed to switch to Atmos version %s: %v", requestedVersion, err)
		ui.Warningf("Continuing with current version %s", Version)
		return false
	}

	// Set re-exec guard to prevent loops.
	if err := cfg.SetEnv(ReexecGuardEnvVar, requestedVersion); err != nil {
		log.Warn("Failed to set re-exec guard", "error", err)
		return false
	}

	// Re-exec with the new binary.
	ui.Successf("Switching to Atmos version `%s`", requestedVersion)

	// Strip flags that shouldn't be passed to the target version.
	args := stripChdirFlags(cfg.Args)
	args = stripUseVersionFlags(args)

	if err := cfg.ExecFn(binaryPath, args, cfg.Environ()); err != nil {
		ui.Errorf("Failed to exec %s: %v", binaryPath, err)
		return false
	}

	// This line is never reached on successful exec.
	return true
}

// findOrInstallVersionWithConfig finds the binary for a version, installing if needed.
// This variant accepts a ReexecConfig for testability.
func findOrInstallVersionWithConfig(version string, cfg *ReexecConfig) (string, error) {
	defer perf.Track(nil, "version.findOrInstallVersionWithConfig")()

	// Validate version format first.
	vType, normalizedVersion, err := toolchain.ParseVersionSpec(version)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrVersionFormatInvalid).
			WithExplanationf("Version '%s' is not a valid format", version).
			WithHint("Version must be a PR number, pr:NNNN, sha:XXXXXXX, or semver (e.g., 1.2.3)").
			WithCause(err).
			WithExitCode(1).
			Err()
	}

	// Handle PR versions (pr:NNNN or just digits) - install from PR artifact.
	if vType == toolchain.VersionTypePR {
		prNumber, _ := toolchain.IsPRVersion(version)
		return findOrInstallPRVersion(prNumber, cfg)
	}

	// Handle SHA versions (sha:XXXXXXX or auto-detected hex strings) - install from SHA artifact.
	if vType == toolchain.VersionTypeSHA {
		return findOrInstallSHAVersion(normalizedVersion, cfg)
	}

	// For semver versions, try to find existing installation.
	binaryPath, err := cfg.Finder.FindBinaryPath("cloudposse", "atmos", normalizedVersion)
	if err == nil && binaryPath != "" {
		log.Debug("Found existing installation", "version", normalizedVersion, "path", binaryPath)
		return binaryPath, nil
	}

	// Install the requested version.
	log.Debug("Version not installed, installing", "version", normalizedVersion)
	toolSpec := fmt.Sprintf("atmos@%s", normalizedVersion)

	if installErr := cfg.Installer.Install(toolSpec, false, false); installErr != nil {
		return "", fmt.Errorf("failed to install Atmos %s: %w", normalizedVersion, installErr)
	}

	// Find the newly installed binary.
	binaryPath, err = cfg.Finder.FindBinaryPath("cloudposse", "atmos", normalizedVersion)
	if err != nil {
		return "", fmt.Errorf("installed Atmos %s but could not find binary: %w", normalizedVersion, err)
	}

	return binaryPath, nil
}

// findOrInstallPRVersion finds the binary for a PR version, installing if needed.
// Uses TTL caching to minimize GitHub API calls:
//   - If binary exists and cache is within TTL (1 min) -> use cached binary, no API call.
//   - If binary exists but TTL expired -> check API for new commits.
//   - If no binary exists -> fresh install.
func findOrInstallPRVersion(prNumber int, _ *ReexecConfig) (string, error) {
	defer perf.Track(nil, "version.findOrInstallPRVersion")()

	ctx := context.Background()

	// Check cache status to determine what action to take.
	cacheStatus, binaryPath := toolchain.CheckPRCacheStatus(prNumber)

	switch cacheStatus {
	case toolchain.PRCacheValid:
		// Binary exists and cache is within TTL - use as-is without API call.
		log.Debug("Using cached PR binary (within TTL)", logFieldPR, prNumber, "path", binaryPath)
		return binaryPath, nil

	case toolchain.PRCacheNeedsCheck:
		// Binary exists but TTL expired - check if PR has new commits.
		log.Debug("Cache TTL expired, checking for updates", logFieldPR, prNumber)
		needsReinstall, err := toolchain.CheckPRCacheAndUpdate(ctx, prNumber, true)
		if err != nil {
			return "", fmt.Errorf("failed to check PR #%d for updates: %w", prNumber, err)
		}
		if !needsReinstall {
			// SHA unchanged, timestamp updated - use existing binary.
			return binaryPath, nil
		}
		// SHA changed - fall through to reinstall.
		log.Debug("PR has new commits, reinstalling", logFieldPR, prNumber)

	case toolchain.PRCacheNeedsInstall:
		// No binary exists - need fresh install.
		log.Debug("PR version not installed, installing from artifact", logFieldPR, prNumber)
	}

	// Install from PR artifact.
	binaryPath, err := toolchain.InstallFromPR(prNumber, true)
	if err != nil {
		return "", fmt.Errorf("failed to install Atmos from PR #%d: %w", prNumber, err)
	}

	return binaryPath, nil
}

// findOrInstallSHAVersion finds the binary for a SHA version, installing if needed.
// SHAs are immutable, so if a binary exists, it's always valid.
func findOrInstallSHAVersion(sha string, _ *ReexecConfig) (string, error) {
	defer perf.Track(nil, "version.findOrInstallSHAVersion")()

	// Check if binary already exists (SHAs are immutable, no TTL check needed).
	exists, binaryPath := toolchain.CheckSHACacheStatus(sha)
	if exists {
		log.Debug("Using cached SHA binary", logFieldSHA, sha, "path", binaryPath)
		return binaryPath, nil
	}

	// Binary doesn't exist - need to install.
	log.Debug("SHA version not installed, installing from artifact", logFieldSHA, sha)

	// Install from SHA artifact.
	binaryPath, err := toolchain.InstallFromSHA(sha, true)
	if err != nil {
		return "", fmt.Errorf("failed to install Atmos from SHA %s: %w", sha, err)
	}

	return binaryPath, nil
}

// stripChdirFlags removes --chdir, -C flags and their values from args.
// This prevents double directory changes when re-exec'ing after chdir has already been applied.
func stripChdirFlags(args []string) []string {
	defer perf.Track(nil, "version.stripChdirFlags")()

	result := make([]string, 0, len(args))
	skipNext := false

	for i, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}

		// Handle --chdir=value or -C=value (combined form).
		if strings.HasPrefix(arg, "--chdir=") || strings.HasPrefix(arg, "-C=") {
			continue
		}

		// Handle --chdir value or -C value (separate form).
		if arg == "--chdir" || arg == "-C" {
			// Skip this arg and the next one (the value).
			if i+1 < len(args) {
				skipNext = true
			}
			continue
		}

		result = append(result, arg)
	}

	return result
}

// stripUseVersionFlags removes --use-version flags and their values from args.
// This prevents passing the flag to older versions that don't understand it.
func stripUseVersionFlags(args []string) []string {
	defer perf.Track(nil, "version.stripUseVersionFlags")()

	result := make([]string, 0, len(args))
	skipNext := false

	for i, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}

		// Handle --use-version=value (combined form).
		if strings.HasPrefix(arg, "--use-version=") {
			continue
		}

		// Handle --use-version value (separate form).
		if arg == "--use-version" {
			// Skip this arg and the next one (the value).
			if i+1 < len(args) {
				skipNext = true
			}
			continue
		}

		result = append(result, arg)
	}

	return result
}
