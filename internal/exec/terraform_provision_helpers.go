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
	log "github.com/cloudposse/atmos/pkg/logger"
	provSource "github.com/cloudposse/atmos/pkg/provisioner/source"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// workdirSubpathAppliedMarker is the sentinel value stored under
// WorkdirSubpathAppliedKey. It is a private type so YAML-derived values
// (strings, bools, maps) cannot impersonate it via a type assertion.
type workdirSubpathAppliedMarker struct{}

// resolveWorkdirSubpath returns workdirRoot joined with metadata.component if
// the joined directory exists on disk, otherwise workdirRoot unchanged.
//
// The metadata.component field has two valid uses for JIT components: (1) a
// real subdirectory inside the cloned repo where the Terraform module lives
// (e.g. `modules/iam-policy` for terraform-aws-iam-style repos — issue #2364),
// and (2) an inheritance/identity pointer naming an abstract base component
// when the cloned repo's `.tf` files live at its root. Disambiguating by
// string shape alone is unreliable, so we disambiguate by filesystem
// existence: after the source provisioner has cloned, either the joined
// directory exists (case 1) or it doesn't (case 2). Stat failures other than
// ENOENT surface as errors so corrupt state isn't masked.
//
// ".." segments in the subpath are intentionally permitted: upstream modules
// often reference shared files via relative parent paths, and
// metadata.component is YAML-author controlled (same trust class as
// !exec / !template / !terraform.state). Absolute subpaths are rejected
// because they violate the documented contract.
func resolveWorkdirSubpath(metadataComponentSubpath, workdirRoot string) (string, error) {
	if metadataComponentSubpath == "" {
		return workdirRoot, nil
	}
	// metadata.component must be a relative subpath inside the provisioned
	// workdir. Reject absolute paths up front: filepath.Join cleans them into
	// a child of workdirRoot on Unix (so the join is non-escaping) and handles
	// drive letters specially on Windows, but in either case an absolute value
	// is nonsensical input and silently coercing it would mask author error.
	if filepath.IsAbs(metadataComponentSubpath) {
		return "", errors.Join(errUtils.ErrWorkdirProvision, fmt.Errorf("workdir component subpath %q must be relative", metadataComponentSubpath))
	}
	candidate := filepath.Join(workdirRoot, metadataComponentSubpath)
	fi, err := os.Stat(candidate)
	if err == nil {
		if fi.IsDir() {
			return candidate, nil
		}
		return "", errors.Join(errUtils.ErrWorkdirProvision, fmt.Errorf("workdir component path %q exists but is not a directory", candidate))
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", errors.Join(errUtils.ErrWorkdirProvision, fmt.Errorf("stat workdir component path %q: %w", candidate, err))
	}
	log.Debug("metadata.component subpath not found in workdir; using workdir root",
		"subpath", metadataComponentSubpath, "workdirRoot", workdirRoot)
	return workdirRoot, nil
}

// applyWorkdirSubpathToSection resolves WorkdirPathKey in info.ComponentSection
// against metadata.component (joining the subpath only when it exists on disk;
// see resolveWorkdirSubpath) and returns the resolved path, or "" when
// WorkdirPathKey is absent or empty. Stat errors propagate as a non-nil error.
//
// The mutation is idempotent: a private-typed sentinel under
// WorkdirSubpathAppliedKey prevents re-resolving across repeat calls (e.g.
// terraform init then terraform plan within one command lifecycle).
func applyWorkdirSubpathToSection(info *schema.ConfigAndStacksInfo) (string, error) {
	workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string)
	if !ok || workdirPath == "" {
		return "", nil
	}
	if _, applied := info.ComponentSection[provWorkdir.WorkdirSubpathAppliedKey].(workdirSubpathAppliedMarker); applied {
		return workdirPath, nil
	}
	resolved, err := resolveWorkdirSubpath(info.BaseComponentPath, workdirPath)
	if err != nil {
		return "", err
	}
	info.ComponentSection[provWorkdir.WorkdirPathKey] = resolved
	info.ComponentSection[provWorkdir.WorkdirSubpathAppliedKey] = workdirSubpathAppliedMarker{}
	return resolved, nil
}

// resolveWorkdirComponentPath computes the effective Terraform working
// directory for a workdir-enabled component by deriving the workdir root from
// provWorkdir.BuildPath and joining metadata.component onto it (only when the
// subpath actually exists on disk; see resolveWorkdirSubpath).
//
// Used by code paths that run after ProcessStacks has rebuilt ComponentSection
// (which does not carry WorkdirPathKey). Returns the resolved path, an
// existence flag (true when the resolved directory exists on disk), and any
// non-ENOENT stat error.
func resolveWorkdirComponentPath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, bool, error) {
	workdirRoot := provWorkdir.BuildPath(
		atmosConfig.BasePath,
		cfg.TerraformComponentType,
		info.FinalComponent,
		info.Stack,
		info.ComponentSection,
	)
	resolved, err := resolveWorkdirSubpath(info.BaseComponentPath, workdirRoot)
	if err != nil {
		return "", false, err
	}

	fi, statErr := os.Stat(resolved)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return resolved, false, nil
		}
		return "", false, errors.Join(errUtils.ErrWorkdirProvision, fmt.Errorf("stat workdir component path %q: %w", resolved, statErr))
	}
	if !fi.IsDir() {
		return "", false, errors.Join(errUtils.ErrWorkdirProvision, fmt.Errorf("workdir component path %q exists but is not a directory", resolved))
	}
	return resolved, true, nil
}

// componentDirExists wraps u.IsDirectory: ENOENT is reported as exists=false
// with a nil error (the boolean carries that signal to callers), while any
// other stat failure is wrapped with errUtils.ErrWorkdirProvision so upstream
// can classify it. The contextLabel identifies the call site in the wrapped
// message.
func componentDirExists(componentPath, contextLabel string) (bool, error) {
	exists, err := u.IsDirectory(componentPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, errors.Join(errUtils.ErrWorkdirProvision, fmt.Errorf("%s %q: %w", contextLabel, componentPath, err))
	}
	return exists, nil
}

// provisionComponentSource performs JIT source provisioning when configured, then
// checks whether the component directory exists. Returns the (possibly updated)
// component path, existence flag, and any error.
func provisionComponentSource(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	componentPath string,
) (string, bool, error) {
	exists, err := componentDirExists(componentPath, "check component path")
	if err != nil {
		return "", false, err
	}

	if !provSource.HasSource(info.ComponentSection) {
		return componentPath, exists, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if autoErr := provSource.AutoProvisionSource(ctx, atmosConfig, cfg.TerraformComponentType, info.ComponentSection, info.AuthContext); autoErr != nil {
		return "", false, errors.Join(errUtils.ErrWorkdirProvision, fmt.Errorf("auto-provision component source: %w", autoErr))
	}

	workdirPath, subpathErr := applyWorkdirSubpathToSection(info)
	if subpathErr != nil {
		return "", false, subpathErr
	}
	if workdirPath != "" {
		exists, err := componentDirExists(workdirPath, "workdir path")
		if err != nil {
			return "", false, err
		}
		return workdirPath, exists, nil
	}

	// Re-check existence after provisioning.
	exists, err = componentDirExists(componentPath, "re-check component path after provisioning")
	return componentPath, exists, err
}
