package source

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Target is the directory the source provisioner populates for one component instance.
//
// It is resolved by a single function, ResolveTarget, shared by the runtime (AutoProvisionSource,
// used by render/deploy/plan) and by the `source pull` / `source delete` commands, so the commands
// always act on exactly the directory the runtime reads from.
type Target struct {
	// Dir is the absolute or base-path-relative destination directory.
	Dir string
	// IsWorkdir reports that Dir is an isolated per-instance workdir rather than a shared
	// components/<type>/<component> directory.
	IsWorkdir bool
	// Component is the resolved component name (the base component, i.e. metadata.component, not
	// the instance name) that names the directory outside workdir mode.
	Component string
}

// ResolveTarget resolves where the source provisioner places a component's source.
//
// The component name is taken from the component configuration the same way AutoProvisionSource
// takes it (the base component: "component", then "metadata.component", then the instance name
// "atmos_component"); fallbackComponent is used only when the configuration carries no name at all.
// Unsafe names (absolute, volume-qualified, or escaping the components directory) are rejected
// before any filesystem path is built. Placement honors provision.workdir.enabled and
// working_directory exactly as the runtime does.
func ResolveTarget(
	atmosConfig *schema.AtmosConfiguration,
	componentType string,
	fallbackComponent string,
	componentConfig map[string]any,
) (*Target, error) {
	defer perf.Track(atmosConfig, "source.ResolveTarget")()

	component := extractComponentName(componentConfig)
	if component == "" {
		component = fallbackComponent
	}
	if component == "" {
		return nil, errUtils.Build(errUtils.ErrSourceProvision).
			WithExplanation("Component name not found in configuration").
			Err()
	}
	if err := validateComponentName(component); err != nil {
		return nil, err
	}

	dir, isWorkdir, err := determineSourceTargetDirectory(atmosConfig, componentType, component, componentConfig)
	if err != nil {
		return nil, err
	}
	return &Target{Dir: dir, IsWorkdir: isWorkdir, Component: component}, nil
}

// validateComponentName rejects component names that cannot safely name a directory under the
// components base path: absolute paths (Unix, UNC, or Windows drive-qualified, regardless of the
// host OS so a manifest behaves the same everywhere) and names that climb out of the base path.
// Without it an absolute metadata.component such as "/Users/me/x" is silently re-rooted under
// components/<type>/ by filepath.Join, leaving a junk tree that the runtime then cannot use.
func validateComponentName(name string) error {
	reject := func(reason string) error {
		return errUtils.Build(fmt.Errorf("%w: component `%s` %s", errUtils.ErrSourceComponentNameInvalid, name, reason)).
			WithHint("Set `metadata.component` to a path relative to the components directory, for example `vpc` or `networking/vpc`").
			WithContext("component", name).
			Err()
	}

	if filepath.IsAbs(name) || filepath.VolumeName(name) != "" ||
		strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) || hasDriveLetterPrefix(name) {
		return reject("is an absolute path")
	}

	cleaned := path.Clean(strings.ReplaceAll(name, `\`, "/"))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return reject("escapes the components directory")
	}
	return nil
}

// hasDriveLetterPrefix reports a Windows drive-qualified name such as "C:\x" or "c:/x" or "C:x".
func hasDriveLetterPrefix(name string) bool {
	if len(name) < 2 || name[1] != ':' {
		return false
	}
	c := name[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
