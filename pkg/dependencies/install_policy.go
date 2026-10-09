package dependencies

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// InstallPolicy returns the validated toolchain.install policy. An unset policy is
// auto. A nil configuration is auto as well, so callers without a loaded
// configuration behave like a project with the default.
//
// An unknown value fails loudly instead of falling back to a default, because a
// typo in a policy that gates downloads must never be guessed at.
func InstallPolicy(atmosConfig *schema.AtmosConfiguration) (schema.ToolchainInstall, error) {
	defer perf.Track(atmosConfig, "dependencies.InstallPolicy")()

	if atmosConfig == nil {
		return schema.ToolchainInstallAuto, nil
	}
	policy := atmosConfig.Toolchain.Install
	if !policy.IsValid() {
		return "", invalidInstallPolicyError(policy)
	}
	return atmosConfig.Toolchain.EffectiveInstall(), nil
}

// InstallsAutomatically reports whether the toolchain.install policy allows Atmos
// to download tools without an explicit `atmos toolchain install`. It is false
// only for the never policy.
func InstallsAutomatically(atmosConfig *schema.AtmosConfiguration) (bool, error) {
	defer perf.Track(atmosConfig, "dependencies.InstallsAutomatically")()

	policy, err := InstallPolicy(atmosConfig)
	if err != nil {
		return false, err
	}
	return policy != schema.ToolchainInstallNever, nil
}

func invalidInstallPolicyError(policy schema.ToolchainInstall) error {
	values := make([]string, 0, len(schema.ToolchainInstallValues))
	for _, value := range schema.ToolchainInstallValues {
		values = append(values, string(value))
	}
	return errUtils.Build(errUtils.ErrInvalidToolchainInstall).
		WithExplanationf("`%s` is not a valid toolchain.install value.", policy).
		WithHintf("Set `toolchain.install` (or `ATMOS_TOOLCHAIN_INSTALL`) to one of: %s", strings.Join(values, ", ")).
		WithContext("value", string(policy)).
		Err()
}

// requireInstalled turns the plan's explicit dependencies into PATH-only entries
// for the never policy. Nothing may be installed, so a dependency that is not
// installed is an error. Version constraints resolve against installed versions.
func requireInstalled(plan *toolPlan, ids *toolIdentity) error {
	var missing []string
	resolved := make(map[string]string, len(plan.install))
	for _, tool := range sortedKeys(plan.install) {
		version := plan.install[tool]
		id, err := ids.identity(tool)
		if err != nil {
			return fmt.Errorf("resolve dependency %q: %w", tool, err)
		}
		installed, ok := ids.installedVersion(id, version)
		if !ok {
			// Use the .tool-versions "tool version" form: "tool@version" renders as an
			// email link in markdown error output.
			missing = append(missing, fmt.Sprintf("%s %s", tool, version))
			continue
		}
		resolved[tool] = installed
	}
	if len(missing) > 0 {
		return notInstalledError(missing)
	}

	plan.install = nil
	for tool, version := range resolved {
		plan.present[tool] = version
	}
	return nil
}

func notInstalledError(missing []string) error {
	return errUtils.Build(errUtils.ErrToolNotInstalled).
		WithExplanationf("These dependencies are not installed: %s.", strings.Join(missing, ", ")).
		WithHint("Run `atmos toolchain install` to install them").
		WithHint("Set `toolchain.install` to `declared` or `auto` to let Atmos install dependencies automatically").
		WithContext("tools", strings.Join(missing, ", ")).
		Err()
}

// installedOnly keeps the entries of deps whose tool is installed. It is the
// lenient counterpart of requireInstalled for callers that hand over a whole
// manifest rather than explicit dependencies: tools that are not installed, or
// that cannot be resolved, are skipped.
func installedOnly(deps map[string]string, ids *toolIdentity) map[string]string {
	present := make(map[string]string, len(deps))
	for _, tool := range sortedKeys(deps) {
		id, err := ids.identity(tool)
		if err != nil {
			log.Debug("Skipping tool: cannot be resolved and toolchain.install is never", logKeyTool, tool, "error", err)
			continue
		}
		version, ok := ids.installedVersion(id, strings.TrimSpace(deps[tool]))
		if !ok {
			log.Debug("Skipping tool: not installed and toolchain.install is never", logKeyTool, tool)
			continue
		}
		present[tool] = version
	}
	return present
}
