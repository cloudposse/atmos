package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/hashicorp/go-version"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

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
// but must never select a parent or an absolute path.
func localModuleSubdir(source string) (root, subdir string, ok bool) {
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
	if strings.Contains(first, ".") && !strings.HasPrefix(root, ".") && !filepath.IsAbs(root) {
		return "", "", false
	}
	return root, subdir, true
}

func safeModuleSubdir(subdir string) bool {
	if subdir == "" || filepath.IsAbs(subdir) || filepath.VolumeName(subdir) != "" || strings.HasPrefix(subdir, "/") || strings.HasPrefix(subdir, `\`) {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(subdir), "/") {
		if part == "" || part == ".." || part == "." {
			return false
		}
	}
	return true
}

func checkVersions(ctx context.Context, request model.Request, terraform, terragrunt string) error {
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
	cmd := exec.CommandContext(ctx, request.Tofu, "version", "-json")
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
func checkUnitPolicies(ctx context.Context, request model.Request, cfg *config.TerragruntConfig, path string) error {
	if cfg.Terraform == nil || cfg.Terraform.Source == nil {
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
	if cfg.PreventDestroy != nil && *cfg.PreventDestroy && atmosbridge.IsDestroyOperation(request.Command, effectivePolicyArguments(request, cfg)) {
		return ErrPreventDestroy
	}
	return nil
}

// validateRemoteSource preserves the getter URI for Atmos's source provisioner. A `tfr://` registry
// source is resolved to one of these URIs when the unit is prepared.
func validateRemoteSource(source string) error {
	for _, prefix := range []string{"git::https://", "git::http://", "git::ssh://", "git::file://", "https://", "http://", registryScheme} {
		if strings.HasPrefix(source, prefix) {
			return nil
		}
	}
	scheme, _, found := strings.Cut(source, "://")
	return &model.Problem{
		Code: model.CodeInvalidSource, Summary: "Unsupported module source transport",
		Detail: fmt.Sprintf("The module source scheme %q is not supported.", scheme),
		Hints:  []string{"Use a local path, a `git::https://`, `git::ssh://`, or `https://` source, a `tfr://` registry source, or a Git source pinned with `?ref=`."},
		Key:    "terraform.source", Safe: found, Err: ErrUnsupportedExecution,
	}
}

// decimalBase is the base of the IAM session duration, a count of seconds.
const decimalBase = 10

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
		seconds, err := strconv.ParseInt(configured, decimalBase, 64)
		if err != nil || seconds < 0 {
			return nil, nil, setting(model.CodeEvaluationError, "iam_assume_role_duration", path, "Invalid IAM session duration",
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

// effectivePolicyArguments follows host ordering: extra arguments precede user arguments.
func effectivePolicyArguments(request model.Request, cfg *config.TerragruntConfig) []string {
	var args []string
	for _, extra := range cfg.Terraform.ExtraArgs {
		if slices.Contains(extra.Commands, request.Command) && extra.Arguments != nil {
			args = append(args, (*extra.Arguments)...)
		}
	}
	return append(args, request.Arguments...)
}
