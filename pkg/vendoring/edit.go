// Package vendoring provides helpers for reading and editing Atmos vendor
// manifests (vendor.yaml) while preserving comments, anchors, and formatting.
package vendoring

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// SourcesPath is the yq path to the sources list in a vendor manifest.
const SourcesPath = ".spec.sources[]"

// yqStringLiteral renders s as a double-quoted yq string literal, escaping
// backslashes and quotes so component/version values are injected safely.
func yqStringLiteral(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// selectByComponent builds a yq filter selecting the source whose `component`
// field equals the given name.
func selectByComponent(component string) string {
	return fmt.Sprintf("%s | select(.component == %s)", SourcesPath, yqStringLiteral(component))
}

// GetComponentVersion returns the version pinned for a component in a vendor
// manifest file. Returns ErrYAMLPathNotFound if the component is absent.
func GetComponentVersion(vendorFile, component string) (string, error) {
	defer perf.Track(nil, "vendoring.GetComponentVersion")()

	expr := selectByComponent(component) + " | .version"
	return atmosyaml.QueryFile(vendorFile, expr)
}

// SetComponentVersion sets the version for a component in a vendor manifest
// file, preserving comments/anchors/formatting. The edit targets the matching
// source by component name (not by index), so reordering the manifest is safe.
func SetComponentVersion(vendorFile, component, version string) error {
	defer perf.Track(nil, "vendoring.SetComponentVersion")()

	// yq's select() silently no-ops when nothing matches, so confirm the
	// component exists first and report a clear error otherwise. Only the
	// path-not-found case means "component missing"; an unreadable file or
	// invalid YAML must surface its real cause.
	if _, err := GetComponentVersion(vendorFile, component); err != nil {
		if errors.Is(err, atmosyaml.ErrYAMLPathNotFound) {
			return fmt.Errorf("%w: component %q not found in %s", err, component, vendorFile)
		}
		return err
	}

	expr := fmt.Sprintf("(%s | .version) = %s", selectByComponent(component), yqStringLiteral(version))
	return atmosyaml.EvalFile(vendorFile, expr)
}
