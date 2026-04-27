package exec

// terraform_provision_helpers.go contains JIT source provisioning helpers
// extracted from terraform_execute_helpers.go to keep that file under 600 lines.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	cfg "github.com/cloudposse/atmos/pkg/config"
	provSource "github.com/cloudposse/atmos/pkg/provisioner/source"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// applyMetadataComponentSubpath joins baseComponentPath onto workdirPath for JIT
// source components whose metadata.component specifies a subdirectory within the
// cloned repo (e.g. metadata.component: modules/iam-policy means the TF module is
// at <workdir>/modules/iam-policy/). When baseComponentPath is empty (no
// metadata.component, or metadata.component equals the component instance name),
// workdirPath is returned unchanged. ".." is resolved naturally by filepath.Join —
// intentional escape hatch.
func applyMetadataComponentSubpath(baseComponentPath, workdirPath string) string {
	if baseComponentPath == "" {
		return workdirPath
	}
	return filepath.Join(workdirPath, baseComponentPath)
}

// provisionComponentSource performs JIT source provisioning when configured, then
// checks whether the component directory exists. Returns the (possibly updated)
// component path, existence flag, and any error.
func provisionComponentSource(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	componentPath string,
) (string, bool, error) {
	exists, err := u.IsDirectory(componentPath)

	if !provSource.HasSource(info.ComponentSection) {
		return componentPath, exists, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if autoErr := provSource.AutoProvisionSource(ctx, atmosConfig, cfg.TerraformComponentType, info.ComponentSection, info.AuthContext); autoErr != nil {
		return "", false, fmt.Errorf("failed to auto-provision component source: %w", autoErr)
	}

	if workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string); ok && workdirPath != "" {
		// Apply the metadata.component subpath exactly once. The sentinel key prevents
		// double-joining if provisionComponentSource is called again with the same
		// ComponentSection (e.g. init then plan within the same invocation).
		if _, applied := info.ComponentSection[provWorkdir.WorkdirSubpathAppliedKey]; !applied {
			workdirPath = applyMetadataComponentSubpath(info.BaseComponentPath, workdirPath)
			info.ComponentSection[provWorkdir.WorkdirPathKey] = workdirPath
			info.ComponentSection[provWorkdir.WorkdirSubpathAppliedKey] = struct{}{}
		}
		exists, errDir := u.IsDirectory(workdirPath)
		if errDir != nil && !errors.Is(errDir, os.ErrNotExist) {
			return "", false, fmt.Errorf("workdir path %q is not accessible: %w", workdirPath, errDir)
		}
		return workdirPath, exists, nil
	}

	// Re-check existence after provisioning.
	exists, err = u.IsDirectory(componentPath)
	return componentPath, exists, err
}
