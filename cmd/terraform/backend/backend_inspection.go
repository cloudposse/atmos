package backend

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Static discovery is separate from authenticated provisioning and injectable in command tests.
var inspectBackendStacks = describeStaticBackendStacks

func executeListCommandWithValues(stack, format string) error {
	return provisioner.ListBackends(atmosConfigPtr, &provisioner.BackendInspectionOptions{
		Stack: stack, Format: format, DescribeStacks: inspectBackendStacks,
	})
}

func executeDescribeCommandWithValues(component, stack, format string) error {
	return provisioner.DescribeBackend(atmosConfigPtr, component, &provisioner.BackendInspectionOptions{
		Stack: stack, Format: format, DescribeStacks: inspectBackendStacks,
	})
}

func describeStaticBackendStacks(atmosConfig *schema.AtmosConfiguration, stack string) (map[string]any, error) {
	staticConfig := *atmosConfig
	staticConfig.Templates.Settings.Enabled = false
	staticConfig.AuthManager = nil
	paths, relativePaths, physical, err := cfg.FindAllStackConfigsInPathsForStack(staticConfig, stack,
		staticConfig.IncludeStackAbsolutePaths, staticConfig.ExcludeStackAbsolutePaths)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("%w: %q", errUtils.ErrStackNotFound, stack)
	}
	staticConfig.StackConfigFilesAbsolutePaths = paths
	staticConfig.StackConfigFilesRelativePaths = relativePaths
	staticConfig.StackType = "Logical"
	if physical {
		staticConfig.StackType = "Directory"
	}
	return e.ExecuteDescribeStacksWithOptions(&staticConfig, stack, nil, nil, nil,
		false, false, false, true, nil, nil, true, e.DescribeStacksErrorOptions{StaticOnly: true})
}
