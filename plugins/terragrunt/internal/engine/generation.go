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
	for _, generated := range cfg.GenerateConfigs {
		if generated.Disable && (generated.IfDisabledStr == "" || generated.IfDisabledStr == "skip") {
			continue
		}
		if !filepath.IsLocal(generated.Path) || generated.Path == "." {
			return fmt.Errorf("unsupported generated path: %s", generated.Path)
		}
		path := filepath.Clean(generated.Path)
		if _, exists := result.GeneratedPolicies[path]; exists {
			return fmt.Errorf("duplicate generated path: %s", generated.Path)
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
		}
		result.GeneratedPolicies[path] = policy
	}
	return generatedBackend(ctx, logger, cfg, result)
}

func generatedBackend(ctx context.Context, logger log.Logger, cfg *config.TerragruntConfig, result *model.Resolution) error {
	if cfg.RemoteState == nil || cfg.RemoteState.Generate == nil {
		return nil
	}
	generated := cfg.RemoteState.Generate
	if !filepath.IsLocal(generated.Path) || generated.Path == "." {
		return fmt.Errorf("unsupported backend generated path: %s", generated.Path)
	}
	path := filepath.Clean(generated.Path)
	if _, exists := result.GeneratedPolicies[path]; exists {
		return fmt.Errorf("duplicate backend generated path: %s", generated.Path)
	}
	content, err := atmosbridge.RenderBackend(ctx, logger, cfg.RemoteState.Config, result.Backend)
	if err != nil {
		return err
	}
	result.Generated[path] = string(content)
	result.GeneratedPolicies[path] = generated.IfExists
	result.BackendGenerated = true
	return nil
}
