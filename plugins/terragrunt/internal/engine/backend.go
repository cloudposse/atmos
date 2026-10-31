package engine

import (
	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

func resolveBackend(cfg *config.TerragruntConfig, result *model.Resolution) error {
	if cfg.RemoteState == nil {
		return nil
	}
	if cfg.RemoteState.DisableInit || len(cfg.RemoteState.Encryption) > 0 {
		return &model.Problem{
			Code: model.CodeUnsupportedSetting, Summary: "Unsupported Terragrunt setting in `remote_state`", Detail: "The native adaptation does not implement `disable_init` or `encryption` in `remote_state`.",
			Hints: []string{"Remove `disable_init` and `encryption` from `remote_state`, or configure the backend through Atmos."}, File: result.Path, Key: "remote_state", Safe: true, Err: ErrUnsupportedExecution,
		}
	}
	result.BackendType = cfg.RemoteState.BackendName
	result.Backend = atmosbridge.BackendConfig(cfg.RemoteState.Config)
	if result.BackendType != "local" {
		return nil
	}
	// Local state remains relative to Terragrunt's source cache identity,
	// independently of the execution directory chosen by Atmos.
	for key, fallback := range map[string]string{"path": "terraform.tfstate", "workspace_dir": "terraform.tfstate.d"} {
		value, _ := result.Backend[key].(string)
		if value == "" {
			value = fallback
		}
		result.Backend[key] = localPath(result.WorkingDir, value)
	}
	return nil
}
