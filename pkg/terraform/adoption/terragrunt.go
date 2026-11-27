package adoption

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component/terragruntexec"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

const (
	unitFile  = "terragrunt.hcl"
	stackFile = "terragrunt.stack.hcl"
	tg        = terragruntexec.Binary
)

// TerragruntUnit identifies one explicitly selected unit without evaluating HCL.
func TerragruntUnit(path string) (string, error) {
	defer perf.Track(nil, "terraformadoption.TerragruntUnit")()
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".terragrunt-cache" || part == ".terragrunt-stack" {
			return "", fmt.Errorf("%w: select the source unit outside %s", errUtils.ErrInvalidComponent, part)
		}
	}
	if filepath.Base(path) == stackFile {
		return "", stackDeclaration(path)
	}
	if filepath.Base(path) == unitFile {
		return regularUnit(path)
	}
	if _, err := os.Stat(filepath.Join(path, stackFile)); err == nil {
		return "", stackDeclaration(filepath.Join(path, stackFile))
	}
	unit, err := regularUnit(filepath.Join(path, unitFile))
	if os.IsNotExist(err) {
		return "", nil
	}
	return unit, err
}

func stackDeclaration(path string) error {
	return fmt.Errorf("%w: %s declares a Terragrunt stack; select a single unit's terragrunt.hcl or configure Terragrunt stack discovery", errUtils.ErrInvalidComponent, path)
}

func regularUnit(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: expected a regular unit file at %s", errUtils.ErrInvalidComponent, path)
	}
	return path, nil
}

type terragruntTool struct{ path, version string }

// Resolve an exact project pin before PATH, just as the named executor does.
func unitTool(ctx context.Context, unit, project, savedPin string, config *schema.AtmosConfiguration) (terragruntTool, error) {
	dir := filepath.Dir(unit)
	pin, err := unitPin(evidenceDirectories(dir, project))
	if err != nil {
		return terragruntTool{}, err
	}
	pin = cmp.Or(pin, savedPin)
	binary := tg
	if pin != "" {
		if installed, err := toolchain.NewInstallerForConfig(config).FindBinaryPath("gruntwork-io", tg, pin, tg); err == nil {
			binary = installed
		}
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		return terragruntTool{}, fmt.Errorf("terragrunt unit %s needs terragrunt %s or newer: %w", unit, terragruntexec.MinimumVersion, err)
	}
	ctx, cancel := context.WithTimeout(ctx, nativeVersionTimeout)
	defer cancel()
	probe := exec.CommandContext(ctx, path, "--version")
	probe.Dir = dir
	out, err := probe.Output()
	if err != nil {
		return terragruntTool{}, fmt.Errorf("terragrunt --version: %w", err)
	}
	version, err := unitVersion(string(out), pin)
	return terragruntTool{path, version}, err
}

func unitVersion(banner, pin string) (string, error) {
	raw := strings.TrimPrefix(strings.TrimSpace(banner), "terragrunt version ")
	v, err := semver.StrictNewVersion(strings.TrimPrefix(raw, "v"))
	if err != nil || v.LessThan(semver.MustParse(terragruntexec.MinimumVersion)) {
		return "", fmt.Errorf("%w: expected terragrunt %s or newer, got %q", errUtils.ErrExecutableVersion, terragruntexec.MinimumVersion, strings.TrimSpace(banner))
	}
	if pin != "" && v.String() != pin {
		return "", fmt.Errorf("%w: project pins terragrunt %s but the executable reports %s", errUtils.ErrExecutableVersion, pin, v)
	}
	return v.String(), nil
}

func unitPin(dirs []string) (string, error) {
	for _, dir := range dirs {
		tv, err := toolchain.LoadToolVersions(filepath.Join(dir, ".tool-versions"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		short, _ := toolchain.GetDefaultVersion(tv, tg)
		full, _ := toolchain.GetDefaultVersion(tv, "gruntwork-io/terragrunt")
		if short != "" && full != "" && short != full {
			return "", fmt.Errorf("%w: conflicting terragrunt pins in %s", errUtils.ErrInvalidToolSpec, dir)
		}
		if pin := strings.TrimPrefix(cmp.Or(short, full), "v"); pin != "" {
			if pin == "system" {
				return "", nil
			}
			if !releasePin.MatchString(pin) {
				return "", fmt.Errorf("%w: expected an exact terragrunt pin in %s, got %q", errUtils.ErrInvalidToolSpec, dir, pin)
			}
			return pin, nil
		}
	}
	return "", nil
}

// TerragruntPlan builds the native trial in the selected unit. Without an override,
// Terragrunt owns binary selection, including inherited HCL and TG_TF_PATH.
func TerragruntPlan(ctx context.Context, unit, project, explicit string, args []string) (*terragruntexec.Command, error) {
	defer perf.Track(nil, "terraformadoption.TerragruntPlan")()
	tool, err := unitTool(ctx, unit, project, "", toolchain.GetAtmosConfig())
	if err != nil {
		return nil, err
	}
	env := os.Environ()
	if explicit != "" {
		env = append(env, "TG_TF_PATH="+explicit)
	}
	return terragruntexec.BuildCommand(&terragruntexec.Request{Root: filepath.Dir(unit), Unit: unitFile, Binary: tool.path, SubCommand: "plan", Arguments: args, Environment: env}), nil
}

func unitOverride(explicit string) string {
	env := viper.New()
	env.AutomaticEnv()
	return cmp.Or(explicit, env.GetString("TG_TF_PATH"), env.GetString("TERRAGRUNT_TFPATH"))
}
