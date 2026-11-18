package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/hashicorp/go-version"
	"github.com/mattn/go-shellwords"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

var (
	errModuleSourceTransport = errors.New("native adapter does not support this module source transport")
	errTerraformSource       = errors.New("native adapter requires terraform.source")
	errTerraformBinary       = errors.New("native adaptation of terraform_binary is not implemented")
	errUnsupportedSettings   = errors.New("native adaptation of errors, exclude, feature flags, or IAM settings is not implemented")
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
	data, err := versionProbeOutput(ctx, cmd)
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
func checkUnitPolicies(ctx context.Context, request *model.Request, cfg *config.TerragruntConfig) error {
	if missingTerraformSource(cfg) {
		return errTerraformSource
	}
	if request.Phase != "backend" {
		if err := checkVersions(ctx, request, cfg.TerraformVersionConstraint, cfg.TerragruntVersionConstraint); err != nil {
			return err
		}
	}
	if cfg.TerraformBinary != "" {
		return errTerraformBinary
	}
	// Hooks and extra arguments are adapted as execution data by unitExecution.
	if err := checkPreventDestroy(request, cfg); err != nil {
		return err
	}
	return checkUnsupportedSettings(cfg)
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

// checkUnsupportedSettings rejects error handling, exclusion, feature flag and
// IAM settings, which have no native equivalent.
func checkUnsupportedSettings(cfg *config.TerragruntConfig) error {
	if cfg.Errors != nil || cfg.Exclude != nil || len(cfg.FeatureFlags) > 0 || cfg.IamRole != "" || cfg.IamWebIdentityToken != "" {
		return errUnsupportedSettings
	}
	return nil
}

// validateRemoteSource preserves the getter URI for Atmos's source provisioner.
// Registry sources need Terragrunt version selection and are not interchangeable
// with direct archive or Git sources.
func validateRemoteSource(source string) error {
	for _, prefix := range []string{"git::https://", "git::http://", "git::ssh://", "git::file://", "https://", "http://"} {
		if strings.HasPrefix(source, prefix) {
			return nil
		}
	}
	return errModuleSourceTransport
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
	if workspace == "" || workspace == "default" {
		return false
	}
	for _, dependency := range cfg.TerragruntDependencies {
		if (dependency.Enabled == nil || *dependency.Enabled) && (dependency.SkipOutputs == nil || !*dependency.SkipOutputs) {
			return true
		}
	}
	return false
}

func missingTerraformSource(cfg *config.TerragruntConfig) bool {
	return cfg.Terraform == nil || cfg.Terraform.Source == nil
}

// versionProbeOutput follows Atmos process-group/job cancellation semantics and
// bounds pipe draining when a wrapper leaves stdout open in a descendant.
func versionProbeOutput(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	const waitDelay = 2 * time.Second
	cmd.WaitDelay = waitDelay
	tree, err := prepareProcessTree(cmd)
	if err != nil {
		return nil, err
	}
	defer tree.close()
	var output bytes.Buffer
	cmd.Stdout = &output
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	if err := tree.started(); err != nil {
		_ = cmd.Process.Kill()
		return nil, errors.Join(err, cmd.Wait())
	}
	err = cmd.Wait()
	return output.Bytes(), errors.Join(err, ctx.Err())
}
