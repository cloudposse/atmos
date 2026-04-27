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

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	provSource "github.com/cloudposse/atmos/pkg/provisioner/source"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// applyMetadataComponentSubpath joins metadataComponentSubpath onto workdirPath for
// JIT source components whose metadata.component points at a subdirectory within
// the cloned repo (e.g. metadata.component: modules/iam-policy means the Terraform
// module is at <workdir>/modules/iam-policy/). When metadataComponentSubpath is
// empty (no metadata.component, or metadata.component equals the component
// instance name), workdirPath is returned unchanged.
//
// ".." in metadataComponentSubpath is resolved naturally by filepath.Join. This
// is intentional: many upstream Terraform modules reference shared files via
// relative parent paths (e.g. ../../shared-vars.tf) and need the full repo on
// disk with the working directory pointed at a subdirectory. Restricting to
// strict subpaths would break those layouts. Atmos's threat model assumes a
// trusted operator running atmos against their own stack configs — the
// metadata.component value is YAML-author-controlled, on par with !exec,
// !template, and !terraform.state, all of which can read or invoke arbitrary
// host resources. See GitHub issue #2364 for the original report.
//
// Absolute paths in metadataComponentSubpath are rooted under workdirPath per
// filepath.Join semantics (the leading separator is stripped). Callers should
// not rely on absolute paths to escape the workdir.
func applyMetadataComponentSubpath(metadataComponentSubpath, workdirPath string) string {
	if metadataComponentSubpath == "" {
		return workdirPath
	}
	return filepath.Join(workdirPath, metadataComponentSubpath)
}

// applyWorkdirSubpathToSection joins metadata.component onto WorkdirPathKey in
// info.ComponentSection (if not already applied), mutates the section to hold
// the joined path, and returns it. Returns ("", false) when WorkdirPathKey is
// absent or empty.
//
// The mutation is idempotent across repeated calls on the same section: the
// WorkdirSubpathAppliedKey sentinel prevents double-joining when this function
// is invoked twice within the same command lifecycle (e.g. terraform init then
// terraform plan), where AutoProvisionSource short-circuits via its own
// invocationDoneKey but WorkdirPathKey still carries the already-joined value
// from the first call.
func applyWorkdirSubpathToSection(info *schema.ConfigAndStacksInfo) (string, bool) {
	workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string)
	if !ok || workdirPath == "" {
		return "", false
	}
	if _, applied := info.ComponentSection[provWorkdir.WorkdirSubpathAppliedKey]; !applied {
		workdirPath = applyMetadataComponentSubpath(info.BaseComponentPath, workdirPath)
		info.ComponentSection[provWorkdir.WorkdirPathKey] = workdirPath
		info.ComponentSection[provWorkdir.WorkdirSubpathAppliedKey] = struct{}{}
	}
	return workdirPath, true
}

// resolveWorkdirComponentPath computes the effective Terraform working directory
// for a workdir-enabled component by deriving the workdir root from
// provWorkdir.BuildPath and joining the metadata.component subpath onto it.
//
// It is used by code paths that run after ProcessStacks has rebuilt
// ComponentSection (which does not carry WorkdirPathKey). It returns the
// candidate path, an existence flag, and any non-ENOENT stat error so callers
// can distinguish "workdir not provisioned yet" (exists=false, no error) from
// "stat failed for another reason" (e.g. EACCES) which surfaces as an error.
func resolveWorkdirComponentPath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, bool, error) {
	workdirRoot := provWorkdir.BuildPath(
		atmosConfig.BasePath,
		cfg.TerraformComponentType,
		info.FinalComponent,
		info.Stack,
		info.ComponentSection,
	)
	candidate := applyMetadataComponentSubpath(info.BaseComponentPath, workdirRoot)

	fi, err := os.Stat(candidate)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return candidate, false, nil
		}
		return "", false, errors.Join(errUtils.ErrWorkdirProvision, fmt.Errorf("stat workdir component path %q: %w", candidate, err))
	}
	return candidate, fi.IsDir(), nil
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
		return "", false, errors.Join(errUtils.ErrWorkdirProvision, fmt.Errorf("auto-provision component source: %w", autoErr))
	}

	if workdirPath, ok := applyWorkdirSubpathToSection(info); ok {
		exists, errDir := u.IsDirectory(workdirPath)
		if errDir != nil && !errors.Is(errDir, os.ErrNotExist) {
			return "", false, errors.Join(errUtils.ErrWorkdirProvision, fmt.Errorf("workdir path %q: %w", workdirPath, errDir))
		}
		return workdirPath, exists, nil
	}

	// Re-check existence after provisioning.
	exists, err = u.IsDirectory(componentPath)
	return componentPath, exists, err
}
