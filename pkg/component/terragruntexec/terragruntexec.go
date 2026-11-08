// Package terragruntexec runs a Terragrunt unit through a pinned `terragrunt` executable.
//
// Executable mode is the migration path next to native adaptation. Atmos still owns which units
// run, their order, authentication, the toolchain, the environment, and the UI. This package only
// translates one Atmos terraform invocation into one `terragrunt run` command for one unit, and
// never schedules anything: `--all` is expanded by Atmos into one command per unit.
package terragruntexec

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Engine is the engine name executable-mode units are listed with.
	Engine = "terragrunt"
	// Binary is the tool name pinned in `dependencies.tools` or `.tool-versions`.
	Binary = "terragrunt"
	// MinimumVersion is the oldest Terragrunt with the `run` command and `--working-dir` this mode uses.
	MinimumVersion = "1.0.0"

	unitFileName = "terragrunt.hcl"
)

// Command is one terragrunt invocation.
type Command struct {
	Binary     string
	Args       []string
	Env        []string
	WorkingDir string
	DryRun     bool
}

// Host supplies what the executable needs from Atmos. Production code runs commands through the
// Atmos shell layer, which streams output through the UI, masks secrets, forwards interrupts, and
// returns the process exit code; tests inject a fake.
//
//go:generate go run go.uber.org/mock/mockgen -destination=mock_host_test.go -package=terragruntexec . Host
type Host interface {
	// Version runs `<binary> --version` and returns its output.
	Version(ctx context.Context, binary string, env []string) (string, error)
	// Execute runs the command to completion.
	Execute(ctx context.Context, command *Command) error
}

// Request describes one unit invocation.
type Request struct {
	Component, Stack string
	// Binary is the resolved terragrunt executable and Engine the resolved OpenTofu or Terraform one.
	Binary, Engine string
	// Unit is the project-relative path of the unit file; Root is the project directory.
	Unit, Root  string
	SubCommand  string
	SubCommand2 string
	// Arguments are the engine arguments passed after `--` on the Atmos command line.
	Arguments []string
	// Options are the provider options the user set; only explicit ones are present.
	Options map[string]schema.ComponentProviderOption
	// Features are the `features` of the provider configuration. Options of the command line and the
	// environment replace them per name.
	Features    map[string]any
	Environment []string
	DryRun      bool
	SkipInit    bool
}

// supported maps Atmos terraform subcommands to the engine command they run. Subcommands that
// Atmos implements itself, such as shell and clean, are not here because they have no
// engine equivalent in a unit.
var supported = map[string]bool{
	"apply": true, "deploy": true, "destroy": true, "fmt": true, "force-unlock": true, "get": true,
	"graph": true, "import": true, "init": true, "output": true, "plan": true, "providers": true,
	"refresh": true, "show": true, "state": true, "taint": true, "test": true, "untaint": true,
	"validate": true, "workspace": true,
}

// versionPattern keeps a prerelease and build suffix, so `v1.0.0-rc1` compares below 1.0.0.
var versionPattern = regexp.MustCompile(`v?(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)`)

// featureEnvironment lists the variables that set Terragrunt features, in the order the provider
// option reads them. Terragrunt ignores TG_FEATURE once a `--feature` flag is passed, so Atmos merges
// the environment into the flags it builds.
var featureEnvironment = []string{"ATMOS_TERRAGRUNT_FEATURE", "TG_FEATURE"}

// Owns reports whether the component runs through the terragrunt executable. The discovery engine
// recorded in the component settings decides, so one project section always runs in one mode.
func Owns(info *schema.ConfigAndStacksInfo) bool {
	defer perf.Track(nil, "terragruntexec.Owns")()

	configuration, _ := info.ComponentSettingsSection["configuration"].(map[string]any)
	engine, _ := configuration["engine"].(string)
	return engine == Engine
}

// CheckCommand rejects an Atmos terraform subcommand that has no Terragrunt equivalent for a unit.
func CheckCommand(info *schema.ConfigAndStacksInfo, command string) error {
	defer perf.Track(nil, "terragruntexec.CheckCommand")()

	if !supported[command] {
		return unsupported(info, fmt.Sprintf("`atmos terraform %s` has no Terragrunt equivalent for a unit", command),
			"Run `terragrunt` directly in the unit directory for this command, or switch the section to `mode: native`.")
	}
	return nil
}

// CheckSupported rejects invocations that executable mode cannot honor, before anything runs.
func CheckSupported(info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "terragruntexec.CheckSupported")()

	if err := CheckCommand(info, info.SubCommand); err != nil {
		return err
	}
	if info.UseTerraformPlan || info.PlanFile != "" || info.StoredPlanFile != "" {
		return unsupported(info, "saved plans managed by Atmos (`--from-plan`, `--planfile`)",
			"Pass the plan file yourself after `--`, for example `atmos terraform plan <unit> -s <stack> -- -out=plan.tfplan`.")
	}
	if len(info.ComponentVarsSection) > 0 {
		return unsupported(info, "stack `vars` for a Terragrunt unit",
			"Set inputs in the unit's `terragrunt.hcl`; Terragrunt owns the variables in this mode.")
	}
	if len(info.ComponentProvidersSection) > 0 {
		return unsupported(info, "stack `providers` overrides for a Terragrunt unit",
			"Configure providers in the unit's `terragrunt.hcl`; Terragrunt owns generated files in this mode.")
	}
	return nil
}

func unsupported(info *schema.ConfigAndStacksInfo, what, hint string) error {
	return errUtils.Build(errUtils.ErrExecutableModeUnsupported).
		WithExplanationf("Component `%s` in stack `%s` runs through the terragrunt executable, which does not support %s.", info.ComponentFromArg, info.Stack, what).
		WithContext("component", info.ComponentFromArg).
		WithContext("stack", info.Stack).
		WithHint(hint).
		Err()
}

// UnitDirectory returns the directory that holds the unit file and the `--config` value needed when
// the file is not named terragrunt.hcl.
func UnitDirectory(root, unit string) (directory, config string) {
	defer perf.Track(nil, "terragruntexec.UnitDirectory")()

	path := filepath.Join(root, filepath.FromSlash(unit))
	if filepath.Base(path) != unitFileName {
		config = path
	}
	return filepath.Dir(path), config
}

// BuildCommand translates one Atmos invocation into a `terragrunt run` command. Provider options
// become flags of `run`, which is where Terragrunt defines them; a sensitive option travels through the environment so it never appears in
// an argument list. Engine arguments follow `--` untouched.
func BuildCommand(request *Request) *Command {
	defer perf.Track(nil, "terragruntexec.BuildCommand")()

	directory, config := UnitDirectory(request.Root, request.Unit)
	// The process starts in the unit directory, so a relative `--working-dir` would be resolved a
	// second time from there. Absolute paths mean the same unit wherever Terragrunt resolves them.
	directory, config = absolute(directory), absolute(config)
	args := []string{"--working-dir", directory, "run"}
	if config != "" {
		args = append(args, "--config", config)
	}
	if request.SkipInit {
		args = append(args, "--no-auto-init")
	}
	optionArgs, optionEnv := optionFlags(request.Options, request.Features, request.Environment)
	args = append(append(args, optionArgs...), "--")
	args = append(args, engineArguments(request)...)
	env := slices.Clone(request.Environment)
	env = withDefault(env, "TG_NON_INTERACTIVE", "true")
	// Forward the engine output untouched, so plan, output -json, and show -json stay pipeable and
	// read like the native output instead of carrying Terragrunt log prefixes.
	env = withDefault(env, "TG_TF_FORWARD_STDOUT", "true")
	if request.Engine != "" {
		env = withDefault(env, "TG_TF_PATH", request.Engine)
	}
	env = append(env, optionEnv...)
	return &Command{Binary: request.Binary, Args: args, Env: env, WorkingDir: directory, DryRun: request.DryRun}
}

// engineArguments is the command after `--`. `deploy` is `apply` that does not ask for approval.
func engineArguments(request *Request) []string {
	command := request.SubCommand
	args := []string{command}
	if command == "deploy" {
		args = []string{"apply", "-auto-approve"}
	}
	if request.SubCommand2 != "" {
		args = append(args, request.SubCommand2)
	}
	return append(args, request.Arguments...)
}

// withDefault appends a variable unless the environment already sets it, so an `env` entry of the
// component wins over the default.
func withDefault(env []string, key, value string) []string {
	for _, entry := range env {
		if name, _, found := strings.Cut(entry, "="); found && name == key {
			return env
		}
	}
	return append(env, key+"="+value)
}

// absolute returns path as an absolute path; an empty path stays empty.
func absolute(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	if resolved, err := filepath.Abs(path); err == nil {
		return resolved
	}
	return path
}

// optionFlags maps provider options to Terragrunt global flags, in a stable order. Features merge per
// name: configuration, then the environment, then the command line.
func optionFlags(options map[string]schema.ComponentProviderOption, configured map[string]any, environment []string) (args, env []string) {
	features := maps.Clone(configured)
	if features == nil {
		features = map[string]any{}
	}
	maps.Copy(features, environmentFeatures(environment))
	if names, ok := options["feature"].Value.(map[string]any); ok {
		maps.Copy(features, names)
	}
	for _, name := range slices.Sorted(maps.Keys(features)) {
		args = append(args, "--feature", name+"="+fmt.Sprint(features[name]))
	}
	for _, flag := range []struct{ option, flag string }{
		{"iam_assume_role", "--iam-assume-role"},
		{"iam_assume_role_duration", "--iam-assume-role-duration"},
		{"iam_assume_role_session_name", "--iam-assume-role-session-name"},
		{"download_dir", "--download-dir"},
	} {
		if option, set := options[flag.option]; set {
			args = append(args, flag.flag, fmt.Sprint(option.Value))
		}
	}
	if token, set := options["iam_assume_role_web_identity_token"]; set {
		env = append(env, "TG_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN="+fmt.Sprint(token.Value))
	}
	return args, env
}

// environmentFeatures reads the features the environment sets: the component `env` first, then the
// process environment, from the first variable that is present.
func environmentFeatures(environment []string) map[string]any {
	for _, name := range featureEnvironment {
		value, present := lookup(environment, name)
		if !present {
			continue
		}
		features := map[string]any{}
		for _, entry := range strings.Split(value, ",") {
			key, setting, found := strings.Cut(strings.TrimSpace(entry), "=")
			if found && strings.TrimSpace(key) != "" {
				features[strings.TrimSpace(key)] = setting
			}
		}
		return features
	}
	return nil
}

// lookup finds a variable in an environment list, where the last entry wins, and then in the process.
func lookup(environment []string, name string) (string, bool) {
	for index := len(environment) - 1; index >= 0; index-- {
		if key, value, found := strings.Cut(environment[index], "="); found && key == name {
			return value, true
		}
	}
	return os.LookupEnv(name)
}

// CheckVersion verifies that the executable exists and is not older than MinimumVersion.
func CheckVersion(ctx context.Context, host Host, binary string, env []string) error {
	defer perf.Track(nil, "terragruntexec.CheckVersion")()

	output, err := host.Version(ctx, binary, env)
	if err != nil {
		// An interrupt is not a missing executable.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return notFound(binary, err)
		}
		return errUtils.Build(errUtils.ErrExecutableNotFound).
			WithCause(err).
			WithExplanationf("`%s --version` failed.", binary).
			WithHint("Check that the pinned terragrunt executable runs on this machine.").
			Err()
	}
	match := versionPattern.FindStringSubmatch(output)
	if match == nil {
		return versionError("the version output could not be read", binary)
	}
	found, err := semver.NewVersion(match[1])
	if err != nil {
		return versionError("the version output could not be read", binary)
	}
	if found.LessThan(semver.MustParse(MinimumVersion)) {
		return versionError(fmt.Sprintf("version %s is older than the minimum %s", found, MinimumVersion), binary)
	}
	return nil
}

func notFound(binary string, cause error) error {
	return errUtils.Build(errUtils.ErrExecutableNotFound).
		WithCause(cause).
		WithExplanationf("Executable mode runs units with `%s`, which was not found.", binary).
		WithHint("Pin terragrunt for the project, for example `terragrunt 1.2.1` in `.tool-versions` or `dependencies.tools` in `atmos.yaml`; Atmos installs and uses the pinned version.").
		WithHintf("Or install terragrunt %s or newer and put it on PATH.", MinimumVersion).
		Err()
}

func versionError(problem, binary string) error {
	return errUtils.Build(errUtils.ErrExecutableVersion).
		WithExplanationf("`%s`: %s.", binary, problem).
		WithHintf("Executable mode requires terragrunt %s or newer. Pin a newer version in `.tool-versions` or `dependencies.tools`.", MinimumVersion).
		Err()
}

// Run checks the executable and runs the unit's command. A dry run skips the version probe, so it
// works without the executable installed and has no side effects.
func Run(ctx context.Context, host Host, request *Request) error {
	defer perf.Track(nil, "terragruntexec.Run")()

	command := BuildCommand(request)
	if !request.DryRun {
		if err := CheckVersion(ctx, host, request.Binary, command.Env); err != nil {
			return err
		}
	}
	return host.Execute(ctx, command)
}
