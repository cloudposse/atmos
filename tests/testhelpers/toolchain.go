package testhelpers

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/dependencies"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Tool describes an external binary the CLI test suite shells out to, pinned to
// a specific version.
type Tool struct {
	// Repo is the toolchain owner/repo spec used to install the tool.
	Repo string
	// Version is the pinned version. Keep in sync with .github/workflows/test.yml.
	Version string
	// Binary is the executable name to look for on PATH (e.g. opentofu installs `tofu`).
	Binary string
}

// DefaultTools lists the external binaries the CLI suite depends on. The pinned
// versions must match .github/workflows/test.yml so local and CI runs agree.
var DefaultTools = []Tool{
	{Repo: "opentofu/opentofu", Version: "1.12.2", Binary: "tofu"},
	{Repo: "hashicorp/terraform", Version: "1.15.6", Binary: "terraform"},
	{Repo: "hashicorp/packer", Version: "1.14.2", Binary: "packer"},
	{Repo: "helmfile/helmfile", Version: "v1.1.0", Binary: "helmfile"},
	{Repo: "helm/helm", Version: "v3.19.2", Binary: "helm"},
}

// ProvisionToolchain installs any of the given tools that aren't already on PATH
// via the Atmos toolchain — dogfooding the toolchain instead of relying on
// host-installed binaries (brew, setup-* GitHub Actions, …) — and prepends their
// bin directories to the process PATH so test subprocesses resolve the pinned
// binaries.
//
// It only provisions tools that are missing ("install as necessary"): in CI the
// binaries are supplied by setup-* actions, so nothing downloads there; locally
// (no host binaries) the toolchain installs them. Installation is idempotent and
// cached across runs.
//
// It is best-effort — on failure (offline, GitHub rate limits) it logs a warning
// and returns, leaving per-test preconditions to skip the affected tests.
//
// Set ATMOS_TEST_SKIP_TOOL_PROVISION=true to skip entirely and rely on whatever
// binaries are already on PATH.
func ProvisionToolchain(logger *log.AtmosLogger, tools []Tool) {
	envMap := envpkg.EnvironToMap()
	if envMap["ATMOS_TEST_SKIP_TOOL_PROVISION"] == "true" {
		logger.Info("skipping test toolchain provisioning (ATMOS_TEST_SKIP_TOOL_PROVISION=true)")
		return
	}

	// Only provision tools that aren't already on PATH.
	missing := map[string]string{}
	for _, t := range tools {
		if _, err := exec.LookPath(t.Binary); err != nil {
			missing[t.Repo] = t.Version
		}
	}
	if len(missing) == 0 {
		logger.Info("test toolchain: all tools already on PATH, nothing to provision")
		return
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		logger.Warn("test toolchain provisioning skipped: cannot resolve user cache dir", "error", err)
		return
	}
	// Stable, shared install path outside the repo so the download is cached
	// across runs and never dirties the working tree.
	installPath := filepath.Join(cacheDir, "atmos", "test-toolchain")

	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Toolchain.InstallPath = installPath
	atmosConfig.Toolchain.VersionsFile = filepath.Join(installPath, "test-tool-versions")

	// NewEnvironmentFromDeps installs the missing tools (idempotent) and returns
	// an environment that knows the toolchain bin directories.
	env, err := dependencies.NewEnvironmentFromDeps(atmosConfig, missing)
	if err != nil {
		logger.Warn("test toolchain provisioning failed; binary-dependent tests will skip", "error", err)
		return
	}

	// Prepend the toolchain bin dirs to the process PATH. The per-test harness
	// copies os.Getenv("PATH") into each subprocess env (and AtmosRunner uses
	// os.Environ()), so this makes the pinned binaries visible everywhere.
	if newPATH := env.PrependToPath(envMap["PATH"]); newPATH != "" {
		os.Setenv("PATH", newPATH)
	}
	logger.Info("provisioned test toolchain via Atmos toolchain", "install_path", installPath, "tools", len(missing))
}
