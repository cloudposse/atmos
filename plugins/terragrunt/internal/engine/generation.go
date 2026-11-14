package engine

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

func generatedFiles(ctx context.Context, logger log.Logger, cfg *config.TerragruntConfig, result *model.Resolution) error {
	result.GeneratedPolicies = make(map[string]string)
	result.UnitFileCopies = make(map[string]bool)
	for _, generated := range cfg.GenerateConfigs {
		if generated.Disable && (generated.IfDisabledStr == "" || generated.IfDisabledStr == "skip") {
			continue
		}
		if !filepath.IsLocal(generated.Path) || generated.Path == "." {
			return &model.Problem{
				Code: model.CodeInvalidGenerate, Summary: "A `generate` block writes outside the unit's working directory", Detail: fmt.Sprintf("The `generate` path %q is absolute or escapes the working directory.", generated.Path),
				Hints: []string{"Use a relative `path` without `..`, for example `path = \"provider.tf\"`."}, File: result.Path, Key: "generate", Safe: true, Err: ErrUnsupportedExecution,
			}
		}
		path := filepath.Clean(generated.Path)
		if _, exists := result.GeneratedPolicies[path]; exists {
			return &model.Problem{
				Code: model.CodeInvalidGenerate, Summary: "Two `generate` blocks write the same file", Detail: fmt.Sprintf("More than one `generate` block writes %q.", generated.Path),
				Hints: []string{"Give each `generate` block a distinct `path`."}, File: result.Path, Key: "generate", Safe: true, Err: ErrUnsupportedExecution,
			}
		}
		policy := generated.IfExistsStr
		if generated.Disable {
			policy = generated.IfDisabledStr
			result.Generated[path] = ""
		} else {
			content, err := atmosbridge.RenderGenerated(ctx, logger, generated)
			if err != nil {
				return err
			}
			result.Generated[path] = string(content)
			// Any generate block can interpolate credentials, regardless of its target.
			markSensitive(result, path)
		}
		result.GeneratedPolicies[path] = policy
		if !generated.Disable && declaresBackend(path, result.Generated[path]) {
			// Arbitrary Terraform text has no verified state-reader identity.
			return unresolvedGeneratedBackend(result.Path)
		}
	}
	if err := unitLockFile(result); err != nil {
		return err
	}
	return generatedBackend(ctx, logger, cfg, result)
}

func generatedBackend(ctx context.Context, logger log.Logger, cfg *config.TerragruntConfig, result *model.Resolution) error {
	if cfg.RemoteState == nil || cfg.RemoteState.Generate == nil {
		return nil
	}
	generated := cfg.RemoteState.Generate
	if !filepath.IsLocal(generated.Path) || generated.Path == "." {
		return &model.Problem{
			Code: model.CodeInvalidGenerate, Summary: "The `remote_state` generate block writes outside the unit's working directory", Detail: fmt.Sprintf("The `remote_state` generate path %q is absolute or escapes the working directory.", generated.Path),
			Hints: []string{"Use a relative path without `..`, for example `path = \"backend.tf\"`."}, File: result.Path, Key: "remote_state.generate", Safe: true, Err: ErrUnsupportedExecution,
		}
	}
	path := filepath.Clean(generated.Path)
	if _, exists := result.GeneratedPolicies[path]; exists {
		return &model.Problem{
			Code: model.CodeInvalidGenerate, Summary: "A `generate` block and `remote_state` write the same file", Detail: fmt.Sprintf("A `generate` block already writes %q.", generated.Path),
			Hints: []string{"Give the `remote_state` generate block a different `path`."}, File: result.Path, Key: "remote_state.generate", Safe: true, Err: ErrUnsupportedExecution,
		}
	}
	content, err := atmosbridge.RenderBackend(ctx, logger, cfg.RemoteState.Config, result.Backend)
	if err != nil {
		return err
	}
	result.Generated[path] = string(content)
	result.GeneratedPolicies[path] = generated.IfExists
	result.BackendGenerated = true
	markSensitive(result, path)
	return nil
}

// markSensitive records a generated file whose bytes can hold credentials.
func markSensitive(result *model.Resolution, path string) {
	if result.SensitiveGenerated == nil {
		result.SensitiveGenerated = map[string]bool{}
	}
	result.SensitiveGenerated[path] = true
}
