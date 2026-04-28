package exec

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

// workdirSubpathAppliedMarker is the sentinel value stored under
// WorkdirSubpathAppliedKey. It is a private type so YAML-derived values
// (strings, bools, maps) cannot impersonate it via a type assertion.
type workdirSubpathAppliedMarker struct{}

// applyMetadataComponentSubpath joins metadata.component onto a JIT workdir
// path so the Terraform module subdirectory inside a cloned repo (e.g.
// metadata.component: modules/iam-policy) becomes the working directory.
// ".." is intentionally permitted: upstream modules often reference shared
// files via relative parent paths, and metadata.component is YAML-author
// controlled (same trust class as !exec / !template / !terraform.state).
// See issue #2364.
func applyMetadataComponentSubpath(metadataComponentSubpath, workdirPath string) string {
	if metadataComponentSubpath == "" {
		return workdirPath
	}
	return filepath.Join(workdirPath, metadataComponentSubpath)
}

// applyWorkdirSubpathToSection joins metadata.component onto WorkdirPathKey in
// info.ComponentSection and returns the joined path, or "" when WorkdirPathKey
// is absent or empty. The mutation is idempotent: a private-typed sentinel
// under WorkdirSubpathAppliedKey prevents double-joining across repeat calls
// (e.g. terraform init then terraform plan within one command lifecycle).
func applyWorkdirSubpathToSection(info *schema.ConfigAndStacksInfo) string {
	workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string)
	if !ok || workdirPath == "" {
		return ""
	}
	if _, applied := info.ComponentSection[provWorkdir.WorkdirSubpathAppliedKey].(workdirSubpathAppliedMarker); applied {
		return workdirPath
	}
	workdirPath = applyMetadataComponentSubpath(info.BaseComponentPath, workdirPath)
	info.ComponentSection[provWorkdir.WorkdirPathKey] = workdirPath
	info.ComponentSection[provWorkdir.WorkdirSubpathAppliedKey] = workdirSubpathAppliedMarker{}
	return workdirPath
}

// resolveWorkdirComponentPath computes the effective Terraform working
// directory for a workdir-enabled component by deriving the workdir root from
// provWorkdir.BuildPath and joining metadata.component onto it.
//
// Used by code paths that run after ProcessStacks has rebuilt ComponentSection
// (which does not carry WorkdirPathKey). Returns the candidate path, an
// existence flag, and any non-ENOENT stat error so callers can distinguish
// "workdir not provisioned yet" (exists=false, no error) from "stat failed for
// another reason" (e.g. EACCES) which surfaces as an error. A non-directory at
// the candidate path also surfaces as an error so corrupt state is not masked.
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
	if !fi.IsDir() {
		return "", false, errors.Join(errUtils.ErrWorkdirProvision, fmt.Errorf("workdir component path %q exists but is not a directory", candidate))
	}
	return candidate, true, nil
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

	if workdirPath := applyWorkdirSubpathToSection(info); workdirPath != "" {
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
