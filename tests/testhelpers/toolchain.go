package testhelpers

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/dependencies"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// Tool describes an external binary the CLI test suite shells out to, pinned to
// a specific version.
type Tool struct {
	// Repo is the toolchain owner/repo spec used to install the tool.
	Repo string
	// Version is the pinned version, resolved from the repo root's .tool-versions
	// (falling back to a hardcoded default if that file or the entry is missing).
	Version string
	// Binary is the executable name to look for on PATH (e.g. opentofu installs `tofu`).
	Binary string
}

// defaultToolVersions are the pins used when .tool-versions can't be read (e.g. this
// helper is somehow invoked outside a checkout). Keeping these here as a fallback --
// rather than a second hardcoded source of truth callers must remember to update --
// avoids the exact drift this function exists to fix: DefaultTools used to hardcode
// versions that silently fell behind .tool-versions (see docs/fixes).
var defaultToolVersions = map[string]string{
	"opentofu/opentofu":   "1.12.2",
	"hashicorp/terraform": "1.15.6",
	"hashicorp/packer":    "1.14.2",
	"helmfile/helmfile":   "v1.1.0",
	"helm/helm":           "v3.19.2",
}

// DefaultTools lists the external binaries the CLI suite depends on, with versions
// resolved from the repo root's .tool-versions file so local and CI runs agree with
// the single source of truth instead of a second, easily-stale hardcoded list.
func DefaultTools() []Tool {
	repos := []struct {
		repo   string
		binary string
	}{
		{"opentofu/opentofu", "tofu"},
		{"hashicorp/terraform", "terraform"},
		{"hashicorp/packer", "packer"},
		{"helmfile/helmfile", "helmfile"},
		{"helm/helm", "helm"},
	}

	versions := loadToolVersionPins()

	tools := make([]Tool, 0, len(repos))
	for _, r := range repos {
		version := versions[r.repo]
		if version == "" {
			version = defaultToolVersions[r.repo]
		}
		tools = append(tools, Tool{Repo: r.repo, Version: version, Binary: r.binary})
	}
	return tools
}

// loadToolVersionPins reads <repo root>/.tool-versions and returns a map from
// "owner/repo" to its pinned (default/first) version. It returns an empty map
// (never an error) if the repo root or file can't be found, leaving callers to
// fall back to defaultToolVersions.
func loadToolVersionPins() map[string]string {
	repoRoot, err := FindRepoRoot()
	if err != nil {
		return map[string]string{}
	}

	toolVersions, err := toolchain.LoadToolVersions(filepath.Join(repoRoot, ".tool-versions"))
	if err != nil {
		return map[string]string{}
	}

	pins := make(map[string]string, len(toolVersions.Tools))
	for tool := range toolVersions.Tools {
		if version, ok := toolchain.GetDefaultVersion(toolVersions, tool); ok {
			pins[tool] = version
		}
	}
	return pins
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
