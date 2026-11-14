package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/hashicorp/go-version"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

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
			return fmt.Errorf("%s %q rejects version %s", name, constraint, actual)
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
func checkUnitPolicies(ctx context.Context, request model.Request, cfg *config.TerragruntConfig) error {
	if cfg.Terraform == nil || cfg.Terraform.Source == nil {
		return fmt.Errorf("native adapter requires terraform.source")
	}
	if request.Phase != "backend" {
		if err := checkVersions(ctx, request, cfg.TerraformVersionConstraint, cfg.TerragruntVersionConstraint); err != nil {
			return err
		}
	}
	if cfg.TerraformBinary != "" {
		return fmt.Errorf("native adaptation of terraform_binary is not implemented")
	}
	// Hooks and extra arguments are adapted as execution data by unitExecution.
	if cfg.PreventDestroy != nil && *cfg.PreventDestroy && atmosbridge.IsDestroyOperation(request.Command, effectivePolicyArguments(request, cfg)) {
		return ErrPreventDestroy
	}
	if cfg.Errors != nil || cfg.Exclude != nil || len(cfg.FeatureFlags) > 0 || cfg.IamRole != "" || cfg.IamWebIdentityToken != "" {
		return fmt.Errorf("native adaptation of errors, exclude, feature flags, or IAM settings is not implemented")
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
	return fmt.Errorf("native adapter does not support this module source transport")
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
