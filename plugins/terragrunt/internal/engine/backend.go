package engine

import (
	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// Warning codes for backend settings that have no effect.
const warningEncryptionNotGenerated = "remote_state_encryption_not_generated"

func resolveBackend(cfg *config.TerragruntConfig, result *model.Resolution, request *model.Request) error {
	if cfg.RemoteState == nil {
		return nil
	}
	// `disable_init` only stops Terragrunt from creating backend resources, which it also does not do unless
	// asked with --backend-bootstrap. Atmos never creates backend resources, and OpenTofu initializes the
	// backend normally in both cases, so the setting needs no handling here.
	if err := checkEncryption(len(cfg.RemoteState.Encryption) > 0, cfg.RemoteState.Generate != nil, result, request); err != nil {
		return err
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

// checkEncryption validates the state encryption settings. Terragrunt writes `encryption` into the
// `terraform { encryption }` block of the file that `remote_state.generate` produces and nowhere else, so it
// has no effect without `generate`; that is reported as a warning rather than silently ignored. The block is
// OpenTofu syntax, so it is rejected before Terraform fails on it at init. Whether the generated file is
// written is decided later, by the host in the provisioned working directory: when `if_exists = "skip"` keeps
// an existing file and nothing else configures encryption, the host warns that the state is not encrypted.
func checkEncryption(encrypted, generated bool, result *model.Resolution, request *model.Request) error {
	if !encrypted || request.Phase == "backend" {
		return nil
	}
	if !generated {
		result.Warnings = append(result.Warnings, model.Warning{
			Code: warningEncryptionNotGenerated, Path: mustRel(request.BasePath, result.Path),
			Message: "`remote_state.encryption` has no effect without `remote_state.generate`, so the state is not encrypted. Terragrunt behaves the same way.",
		})
		return nil
	}
	if IsOpenTofu(request.Tofu) {
		return nil
	}
	return &model.Problem{
		Code: model.CodeUnsupportedSetting, Summary: "State encryption needs OpenTofu",
		Detail: "`remote_state.encryption` writes an `encryption` block that only OpenTofu understands, and the selected executable is not OpenTofu.",
		Hints:  []string{"Set `components.terraform.command: tofu` in atmos.yaml, or remove `encryption` from `remote_state`."},
		File:   result.Path, Key: "remote_state.encryption", Safe: true, Err: ErrUnsupportedExecution,
	}
}
