package terraform

import (
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
	tf "github.com/cloudposse/atmos/pkg/terraform"
	tfbackend "github.com/cloudposse/atmos/pkg/terraform/backend"
)

type (
	GenerateBackendOptions = tf.GenerateBackendOptions
	VarfileOptions         = tf.VarfileOptions
	ShellCommandOption     = component.ExecutionOption
)

const (
	filePermissions = 0o644
	planFileMode    = 0o600
)

func generateComponentBackendConfig(backendType string, backendConfig map[string]any, workspace string, _ *schema.AuthContext) (map[string]any, error) {
	return tfbackend.Config(backendType, backendConfig, workspace)
}
