package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/hashicorp/go-version"
	"github.com/mattn/go-shellwords"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

const currentDirectory = "."

// ErrVersionRejected reports a terraform or terragrunt version that violates a
// declared version constraint.
var ErrVersionRejected = errors.New("version constraint rejected")

func localPath(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(base, path)
}

// plainLocalSource accepts native absolute paths, including Windows drive
// letters, while rejecting URL/query and module subdirectory syntax.
func plainLocalSource(source string) bool {
	path := strings.TrimPrefix(source, filepath.VolumeName(source))
	return !strings.ContainsAny(path, "?:") && !strings.Contains(filepath.ToSlash(path), "//")
}

// localModuleSubdir separates Terragrunt's local root//module spelling without
// cleaning away the delimiter. A subdirectory may descend within the root,
// but must never select a parent or an absolute path. A bare relative root whose first segment looks like
// a hostname is a remote shorthand unless existingRoot reports that it names a local directory.
func localModuleSubdir(source string, existingRoot func(root string) bool) (root, subdir string, ok bool) {
	volume := filepath.VolumeName(source)
	rest := source[len(volume):]
	if strings.ContainsAny(rest, "?:") {
		return "", "", false
	}
	index := strings.Index(filepath.ToSlash(rest), "//")
	if index <= 0 {
		return "", "", false
	}
	root, subdir = volume+rest[:index], rest[index+2:]
	first := strings.SplitN(strings.TrimPrefix(root, "./"), "/", 2)[0]
	if strings.Contains(first, ".") && !strings.HasPrefix(root, ".") && !filepath.IsAbs(root) && (existingRoot == nil || !existingRoot(root)) {
		return "", "", false
	}
	return root, subdir, true
}

func safeModuleSubdir(subdir string) bool {
	if subdir == "" {
		return true
	}
	if absoluteModuleSubdir(subdir) {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(subdir), "/") {
		if part == "" || part == ".." || part == currentDirectory {
			return false
		}
	}
	return true
}

// absoluteModuleSubdir rejects native absolute paths and Windows-qualified paths on every host.
func absoluteModuleSubdir(subdir string) bool {
	return (len(subdir) >= 2 && subdir[1] == ':') || filepath.IsAbs(subdir) || filepath.VolumeName(subdir) != "" || strings.HasPrefix(subdir, "/") || strings.HasPrefix(subdir, `\`)
}

func checkVersions(ctx context.Context, request *model.Request, terraform, terragrunt string) error {
	check := func(name, constraint, actual string) error {
		if constraint == "" {
			return nil
		}
		rules, err := version.NewConstraint(constraint)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		v, err := version.NewVersion(actual)
		if err != nil {
			return err
		}
		if !rules.Check(v) {
			return fmt.Errorf("%w: %s %q rejects version %s", ErrVersionRejected, name, constraint, actual)
		}
		return nil
	}
	if err := check("terragrunt_version_constraint", terragrunt, "1.2.1"); err != nil {
		return err
	}
	if terraform == "" {
		return nil
	}
	// The configured Terraform executable is selected deliberately; arguments are fixed and no shell is used.
	cmd := exec.CommandContext(ctx, request.Tofu, "version", "-json") //nolint:gosec // G204: query the explicitly selected engine version.
	cmd.Env = make([]string, 0, len(request.Environment))
	for key, value := range request.Environment {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	data, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("terraform_version_constraint: cannot inspect selected executable: %w", err)
	}
	var result struct {
		Version string `json:"terraform_version"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}
	return check("terraform_version_constraint", terraform, result.Version)
}

// checkUnitPolicies rejects Terragrunt semantics that the native adapter cannot
// represent faithfully. Backend reads skip the version probe, which may execute
// the selected binary.
func checkUnitPolicies(ctx context.Context, request *model.Request, cfg *config.TerragruntConfig, path string) error {
	if missingTerraformSource(cfg) {
		return missingSourceProblem(path)
	}
	if request.Phase != "backend" {
		if err := checkVersions(ctx, request, cfg.TerraformVersionConstraint, cfg.TerragruntVersionConstraint); err != nil {
			return err
		}
	}
	if err := rejectUnsupported(cfg, path); err != nil {
		return err
	}
	// Hooks and extra arguments are adapted as execution data by unitExecution.
	if err := checkPreventDestroy(request, cfg); err != nil {
		return err
	}
	return nil
}

// checkPreventDestroy evaluates the same effective arguments as the engine.
func checkPreventDestroy(request *model.Request, cfg *config.TerragruntConfig) error {
	if request.Phase == "backend" || cfg.PreventDestroy == nil || !*cfg.PreventDestroy {
		return nil
	}
	args, err := effectivePolicyArguments(request, cfg)
	if err != nil {
		return err
	}
	if atmosbridge.IsDestroyOperation(request.Command, args) {
		return ErrPreventDestroy
	}
	return nil
}

// localSubdirSource reports a local module path that selects a subdirectory with `//`, such as
// `../modules/sub//app`. Terragrunt copies the tree before `//`, so it is no remote transport. Only an
// explicit local spelling counts: a bare `host/org/repo//dir` is a remote shorthand.
func localSubdirSource(source string) bool {
	if _, ok := copiedSourceRoot("", source); !ok {
		return false
	}
	return filepath.IsAbs(source) || strings.HasPrefix(source, currentDirectory)
}

// validateRemoteSource preserves the getter URI for Atmos's source provisioner. A `tfr://` registry
// source is resolved to one of these URIs when the unit is prepared.
func validateRemoteSource(source string) error {
	if localSubdirSource(source) {
		return nil
	}
	for _, prefix := range []string{"git::https://", "git::http://", "git::ssh://", "git::file://", "https://", "http://", registryScheme} {
		if strings.HasPrefix(source, prefix) {
			return nil
		}
	}
	return &model.Problem{
		Code: model.CodeInvalidSource, Summary: "Unsupported module source transport",
		Detail: "The module source transport is not supported.",
		Hints:  []string{"Use a local path, a `git::https://`, `git::ssh://`, or `https://` source, a `tfr://` registry source, or a Git source pinned with `?ref=`."},
		Key:    "terraform.source", Safe: true, Err: ErrUnsupportedExecution,
	}
}

// decimalBase is the base of the IAM session duration, a count of seconds.
const decimalBase = 10

// iamDurationBits is the width of the signed IAM duration field.
const iamDurationBits = 64

// Warning code for IAM settings that have no effect.
const warningIAMTokenWithoutRole = "iam_web_identity_token_without_role"

// iamEnvironment names the variables that Terragrunt reads its IAM options from, newest name first.
// Like its --iam-assume-role flags, they take precedence over the same settings in the unit.
var iamEnvironment = map[string][]string{
	"role":     {"TG_IAM_ASSUME_ROLE", "TERRAGRUNT_IAM_ROLE"},
	"duration": {"TG_IAM_ASSUME_ROLE_DURATION", "TERRAGRUNT_IAM_ASSUME_ROLE_DURATION"},
	"session":  {"TG_IAM_ASSUME_ROLE_SESSION_NAME", "TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME"},
	"token":    {"TG_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN", "TERRAGRUNT_IAM_WEB_IDENTITY_TOKEN", "TERRAGRUNT_IAM_ASSUME_ROLE_WEB_IDENTITY_TOKEN"},
}

// iamSettings returns the role the unit asks for, or nil. Terragrunt assumes it only when a role ARN is
// present, so a token alone is reported and ignored. The host assumes the role, not the plugin.
func iamSettings(cfg *config.TerragruntConfig, request *model.Request, path string) (*model.IAM, []model.Warning, error) {
	pick := func(key, configured string) string {
		for _, name := range iamEnvironment[key] {
			if value := request.Environment[name]; value != "" {
				return value
			}
		}
		return configured
	}
	var configuredDuration int64
	if cfg.IamAssumeRoleDuration != nil {
		configuredDuration = *cfg.IamAssumeRoleDuration
	}
	iam := &model.IAM{
		Role: pick("role", cfg.IamRole), SessionName: pick("session", cfg.IamAssumeRoleSessionName),
		DurationSeconds: configuredDuration, WebIdentityToken: pick("token", cfg.IamWebIdentityToken),
	}
	if configured := pick("duration", ""); configured != "" {
		seconds, err := strconv.ParseInt(configured, decimalBase, iamDurationBits)
		if err != nil || seconds < 0 {
			return nil, nil, evaluationSetting("iam_assume_role_duration", path, "Invalid IAM session duration",
				"The IAM session duration from the environment is not a number of seconds.", "Set `TG_IAM_ASSUME_ROLE_DURATION` to a number of seconds, for example `3600`.")
		}
		iam.DurationSeconds = seconds
	}
	if iam.Role != "" {
		iam.WebIdentityToken = unitTokenFile(iam.WebIdentityToken, filepath.Dir(path))
		return iam, nil, nil
	}
	if iam.WebIdentityToken == "" {
		return nil, nil, nil
	}
	return nil, []model.Warning{{Code: warningIAMTokenWithoutRole, Path: mustRel(request.BasePath, path), Message: "`iam_web_identity_token` has no effect without `iam_role`, so no role is assumed. Terragrunt behaves the same way."}}, nil
}

// unitTokenFile anchors a relative web identity token file at the unit directory, so the host finds it
// whatever directory Atmos runs from. Absolute paths and token values that name no file are unchanged.
func unitTokenFile(token, unitDir string) string {
	if token == "" || filepath.IsAbs(token) {
		return token
	}
	candidate := filepath.Join(unitDir, token)
	if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
		return candidate
	}
	return token
}

// effectivePolicyArguments matches engine insertion order: command environment,
// global environment, extra arguments, then explicit arguments. Matching extra
// argument blocks override environment values in declaration order.
func effectivePolicyArguments(request *model.Request, cfg *config.TerragruntConfig) ([]string, error) {
	environment := map[string]string{}
	maps.Copy(environment, request.Environment)
	var args []string
	if cfg.Terraform != nil {
		for _, extra := range cfg.Terraform.ExtraArgs {
			if !slices.Contains(extra.Commands, request.Command) {
				continue
			}
			if extra.Arguments != nil {
				args = append(args, (*extra.Arguments)...)
			}
			if extra.EnvVars != nil {
				maps.Copy(environment, *extra.EnvVars)
			}
		}
	}
	var injected []string
	suffix := strings.NewReplacer("-", "_", " ", "_").Replace(request.Command)
	for _, name := range []string{"TF_CLI_ARGS_" + suffix, "TF_CLI_ARGS"} {
		parser := shellwords.NewParser()
		parser.ParseEnv, parser.ParseBacktick = false, false
		tokens, err := parser.Parse(environment[name])
		if err != nil {
			return nil, fmt.Errorf("invalid %s arguments: %w", name, err)
		}
		injected = append(injected, tokens...)
	}
	args = append(injected, args...)
	return append(args, request.Arguments...), nil
}

// unsupportedDependencyWorkspace preserves the existing output-dependency workspace guard.
func unsupportedDependencyWorkspace(request *model.Request, cfg *config.TerragruntConfig) bool {
	workspace := request.Environment["TF_WORKSPACE"]
	return workspace != "" && workspace != "default" && len(cfg.TerragruntDependencies) > 0 && cfg.Dependencies != nil && len(cfg.Dependencies.Paths) > 0
}

func missingTerraformSource(cfg *config.TerragruntConfig) bool {
	return cfg.Terraform == nil || cfg.Terraform.Source == nil
}
