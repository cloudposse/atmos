package version

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/toolchain"
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
	installer := toolchain.NewInstaller()
	return &ReexecConfig{
		Finder:    installer,
		Installer: &defaultInstaller{},
		ExecFn:    syscall.Exec,
		GetEnv:    os.Getenv,
		SetEnv:    os.Setenv,
		Args:      os.Args,
		Environ:   os.Environ,
	}
}

// defaultInstaller wraps toolchain.RunInstall.
type defaultInstaller struct{}

func (d *defaultInstaller) Install(toolSpec string, force, allowPrereleases bool) error {
	return toolchain.RunInstall(toolSpec, force, allowPrereleases)
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

	// Check for version specification with precedence:
	// ATMOS_VERSION_USE (set by --use-version flag or env) > ATMOS_VERSION > version.use in config.
	requestedVersion := cfg.GetEnv(VersionUseEnvVar)
	if requestedVersion == "" {
		requestedVersion = cfg.GetEnv(VersionEnvVar)
	}
	if requestedVersion == "" {
		requestedVersion = atmosConfig.Version.Use
	}
	if requestedVersion == "" {
		return false
	}

	// Check re-exec guard to prevent infinite loops.
	if cfg.GetEnv(ReexecGuardEnvVar) == requestedVersion {
		log.Debug("Re-exec guard active, skipping version switch",
			"requested_version", requestedVersion,
			"guard_value", cfg.GetEnv(ReexecGuardEnvVar))
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
		return false
	}

	log.Debug("Version mismatch, will re-exec",
		"current", currentVersion,
		"requested", targetVersion)

	// Find or install the requested version.
	binaryPath, err := findOrInstallVersionWithConfig(targetVersion, cfg)
	if err != nil {
		// Log error but don't fail - allow current version to run.
		_ = ui.Warningf("Failed to switch to Atmos version %s: %v", requestedVersion, err)
		_ = ui.Warningf("Continuing with current version %s", Version)
		return false
	}

	// Set re-exec guard to prevent loops.
	if err := cfg.SetEnv(ReexecGuardEnvVar, requestedVersion); err != nil {
		log.Warn("Failed to set re-exec guard", "error", err)
		return false
	}

	// Re-exec with the new binary.
	_ = ui.Successf("Switching to Atmos version `%s`", requestedVersion)

	// Strip --chdir/-C flags from args since the directory has already been changed.
	// Passing them again would cause double directory change (e.g., "dir/dir").
	args := stripChdirFlags(cfg.Args)

	// Use syscall.Exec for true process replacement (Unix).
	// This replaces the current process entirely.
	//nolint:gosec // Intentional subprocess exec for version switching.
	if err := cfg.ExecFn(binaryPath, args, cfg.Environ()); err != nil {
		_ = ui.Errorf("Failed to exec %s: %v", binaryPath, err)
		return false
	}

	// This line is never reached on successful exec.
	return true
}

// findOrInstallVersion finds the binary for a version, installing if needed.
func findOrInstallVersion(version string) (string, error) {
	defer perf.Track(nil, "version.findOrInstallVersion")()

	return findOrInstallVersionWithConfig(version, DefaultReexecConfig())
}

// findOrInstallVersionWithConfig finds the binary for a version, installing if needed.
// This variant accepts a ReexecConfig for testability.
func findOrInstallVersionWithConfig(version string, cfg *ReexecConfig) (string, error) {
	defer perf.Track(nil, "version.findOrInstallVersionWithConfig")()

	// Try to find existing installation.
	binaryPath, err := cfg.Finder.FindBinaryPath("cloudposse", "atmos", version)
	if err == nil && binaryPath != "" {
		log.Debug("Found existing installation", "version", version, "path", binaryPath)
		return binaryPath, nil
	}

	// Install the requested version.
	log.Debug("Version not installed, installing", "version", version)
	toolSpec := fmt.Sprintf("atmos@%s", version)

	if installErr := cfg.Installer.Install(toolSpec, false, false); installErr != nil {
		return "", fmt.Errorf("failed to install Atmos %s: %w", version, installErr)
	}

	// Find the newly installed binary.
	binaryPath, err = cfg.Finder.FindBinaryPath("cloudposse", "atmos", version)
	if err != nil {
		return "", fmt.Errorf("installed Atmos %s but could not find binary: %w", version, err)
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
