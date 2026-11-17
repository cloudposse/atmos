package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/hashicorp/go-version"
	"github.com/mattn/go-shellwords"

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
func checkUnitPolicies(ctx context.Context, request *model.Request, cfg *config.TerragruntConfig, path string) error {
	if cfg.Terraform == nil || cfg.Terraform.Source == nil {
		return missingSourceProblem(path)
	}
	if request.Phase != "backend" {
		if err := checkVersions(ctx, *request, cfg.TerraformVersionConstraint, cfg.TerragruntVersionConstraint); err != nil {
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
	return filepath.IsAbs(source) || strings.HasPrefix(source, ".")
}

// validateRemoteSource preserves the getter URI for Atmos's source provisioner.
// Registry sources need Terragrunt version selection and are not interchangeable
// with direct archive or Git sources.
func validateRemoteSource(source string) error {
	if localSubdirSource(source) {
		return nil
	}
	for _, prefix := range []string{"git::https://", "git::http://", "git::ssh://", "git::file://", "https://", "http://"} {
		if strings.HasPrefix(source, prefix) {
			return nil
		}
	}
	scheme, _, found := strings.Cut(source, "://")
	return &model.Problem{
		Code: model.CodeInvalidSource, Summary: "Unsupported module source transport",
		Detail: fmt.Sprintf("The module source scheme %q is not supported. Registry sources such as `tfr://` need Terragrunt's version selection and are not interchangeable with Git or archive sources.", scheme),
		Hints:  []string{"Use a local path, a `git::https://`, `git::ssh://`, or `https://` source, or a Git source pinned with `?ref=`."},
		Key:    "terraform.source", Safe: found, Err: ErrUnsupportedExecution,
	}
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
