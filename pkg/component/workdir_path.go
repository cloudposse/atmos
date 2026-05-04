package component

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provSource "github.com/cloudposse/atmos/pkg/provisioner/source"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// workdirSubpathAppliedKey marks that the metadata.component subpath has
// already been joined onto provWorkdir.WorkdirPathKey within the current
// command lifecycle. The value stored here is a private-typed sentinel
// (subpathAppliedMarker) so YAML-author values cannot forge the marker —
// see TestApplyWorkdirSubpathToSection_UserYAMLCannotForgeSentinel.
//
// Lives in this package because read/write access is confined to
// pkg/component; placing the constant here keeps the protocol single-sourced
// next to the only code that uses it.
const workdirSubpathAppliedKey = "_workdir_subpath_applied"

// subpathAppliedMarker is the sentinel value stored under
// workdirSubpathAppliedKey. It is a private type so YAML-derived values
// (strings, bools, maps) cannot impersonate it via a type assertion.
type subpathAppliedMarker struct{}

// ResolveWorkdirSubpath returns workdirRoot joined with metadataSubpath if the
// joined directory exists on disk, otherwise workdirRoot unchanged.
//
// The metadata.component field has two valid uses for JIT components: (1) a
// real subdirectory inside the cloned repo where the module lives (e.g.
// `modules/iam-policy` for repos that organize modules under `modules/`), and
// (2) an inheritance/identity pointer naming an abstract base component when
// the cloned repo's files live at its root. Disambiguating by string shape
// alone is unreliable, so we disambiguate by filesystem existence: after the
// source provisioner has cloned, either the joined directory exists (case 1)
// or it doesn't (case 2). Stat failures other than ENOENT surface as errors
// so corrupt state isn't masked.
//
// ".." segments in the subpath are intentionally permitted: upstream modules
// often reference shared files via relative parent paths, and
// metadata.component is YAML-author controlled (same trust class as
// !exec / !template / !terraform.state). Absolute subpaths are rejected
// because they violate the documented contract.
func ResolveWorkdirSubpath(metadataSubpath, workdirRoot string) (string, error) {
	defer perf.Track(nil, "component.ResolveWorkdirSubpath")()

	if metadataSubpath == "" {
		return workdirRoot, nil
	}
	// metadata.component must be a relative subpath inside the provisioned
	// workdir. Reject absolute paths up front: filepath.Join cleans them into
	// a child of workdirRoot on Unix (so the join is non-escaping) and handles
	// drive letters specially on Windows, but in either case an absolute value
	// is nonsensical input and silently coercing it would mask author error.
	if filepath.IsAbs(metadataSubpath) {
		return "", errors.Join(errUtils.ErrWorkdirProvision, fmt.Errorf("workdir component subpath %q must be relative", metadataSubpath))
	}
	candidate := filepath.Join(workdirRoot, metadataSubpath)
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
		"subpath", metadataSubpath, "workdirRoot", workdirRoot)
	return workdirRoot, nil
}

// ApplyWorkdirSubpathToSection resolves provWorkdir.WorkdirPathKey in
// info.ComponentSection against info.BaseComponentPath (joining the subpath
// only when it exists on disk; see ResolveWorkdirSubpath) and returns the
// resolved path, or "" when WorkdirPathKey is absent or empty. Stat errors
// propagate as a non-nil error.
//
// Mutates info.ComponentSection in place: writes the resolved path back to
// provWorkdir.WorkdirPathKey and sets a private-typed sentinel under
// workdirSubpathAppliedKey. ComponentSection is a shared map[string]any; the
// caller is responsible for ensuring no goroutine reads or writes the same
// section concurrently for the duration of this call.
//
// The mutation is idempotent: the sentinel prevents re-resolving across
// repeat calls (e.g. terraform init then terraform plan within one command
// lifecycle).
func ApplyWorkdirSubpathToSection(info *schema.ConfigAndStacksInfo) (string, error) {
	defer perf.Track(nil, "component.ApplyWorkdirSubpathToSection")()

	workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string)
	if !ok || workdirPath == "" {
		return "", nil
	}
	if _, applied := info.ComponentSection[workdirSubpathAppliedKey].(subpathAppliedMarker); applied {
		return workdirPath, nil
	}
	resolved, err := ResolveWorkdirSubpath(info.BaseComponentPath, workdirPath)
	if err != nil {
		return "", err
	}
	info.ComponentSection[provWorkdir.WorkdirPathKey] = resolved
	info.ComponentSection[workdirSubpathAppliedKey] = subpathAppliedMarker{}
	return resolved, nil
}

// BuildAndResolveWorkdirPath computes the effective component working directory
// for a workdir-enabled component by deriving the workdir root from
// provWorkdir.BuildPath and joining metadata.component onto it (only when the
// subpath actually exists on disk; see ResolveWorkdirSubpath).
//
// Used by code paths that run after ProcessStacks has rebuilt ComponentSection
// (which does not carry WorkdirPathKey). Returns the resolved path, an
// existence flag (true when the resolved directory exists on disk), and any
// non-ENOENT stat error.
//
// The componentType parameter is one of cfg.{Terraform,Helmfile,Packer,Ansible}ComponentType.
func BuildAndResolveWorkdirPath(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	componentType string,
) (string, bool, error) {
	defer perf.Track(atmosConfig, "component.BuildAndResolveWorkdirPath")()

	workdirRoot := provWorkdir.BuildPath(
		atmosConfig.BasePath,
		componentType,
		info.FinalComponent,
		info.Stack,
		info.ComponentSection,
	)
	resolved, err := ResolveWorkdirSubpath(info.BaseComponentPath, workdirRoot)
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
// other stat failure is wrapped with the provided sentinel so upstream can
// classify it (ErrWorkdirProvision for workdir-path failures,
// ErrInvalidComponent for the local component-dir fallback). The contextLabel
// identifies the call site in the wrapped message.
func componentDirExists(componentPath, contextLabel string, sentinel error) (bool, error) {
	exists, err := u.IsDirectory(componentPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, errors.Join(sentinel, fmt.Errorf("%s %q: %w", contextLabel, componentPath, err))
	}
	return exists, nil
}

// ProvisionAndResolveComponentPath performs JIT source provisioning when the
// component declares a source, then resolves the effective component path by
// applying metadata.component as a subpath onto WorkdirPathKey when set.
// Returns the (possibly updated) component path, an existence flag, and any
// error.
//
// Error classification:
//   - AutoProvisionSource failures wrap errUtils.ErrProvisionerFailed (matches
//     the established pattern in pkg/provisioner/registry.go and
//     internal/exec/terraform_shell.go).
//   - Path resolution / stat / abs-subpath rejection failures wrap
//     errUtils.ErrWorkdirProvision.
//
// componentType is one of cfg.{Terraform,Helmfile,Packer,Ansible}ComponentType.
// The caller owns ctx — typically constructed with a per-call-site timeout
// (e.g., context.WithTimeout(parent, 5*time.Minute)) before invocation.
func ProvisionAndResolveComponentPath(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	componentType, fallbackComponentPath string,
) (string, bool, error) {
	defer perf.Track(atmosConfig, "component.ProvisionAndResolveComponentPath")()

	// Short-circuit components without a JIT source: only the fallback dir is
	// relevant, so do the stat here rather than up front (a non-ENOENT stat
	// failure on the fallback must not abort JIT for components that DO declare
	// a source — that was the legacy helmfile/packer behavior). The fallback is
	// a local component dir, not a workdir, so stat failures wrap
	// ErrInvalidComponent rather than ErrWorkdirProvision.
	if !provSource.HasSource(info.ComponentSection) {
		exists, err := componentDirExists(fallbackComponentPath, "check component path", errUtils.ErrInvalidComponent)
		return fallbackComponentPath, exists, err
	}

	if autoErr := provSource.AutoProvisionSource(ctx, atmosConfig, componentType, info.ComponentSection, info.AuthContext); autoErr != nil {
		return "", false, errors.Join(errUtils.ErrProvisionerFailed, fmt.Errorf("auto-provision component source: %w", autoErr))
	}

	workdirPath, subpathErr := ApplyWorkdirSubpathToSection(info)
	if subpathErr != nil {
		return "", false, subpathErr
	}
	if workdirPath != "" {
		exists, err := componentDirExists(workdirPath, "workdir path", errUtils.ErrWorkdirProvision)
		if err != nil {
			return "", false, err
		}
		return workdirPath, exists, nil
	}

	// Re-check existence after provisioning (source-only, no workdir): the
	// fallback is again a local component dir, so wrap with ErrInvalidComponent.
	exists, err := componentDirExists(fallbackComponentPath, "re-check component path after provisioning", errUtils.ErrInvalidComponent)
	return fallbackComponentPath, exists, err
}
