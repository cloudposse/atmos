package source

import (
	"path/filepath"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// CheckDeletable verifies that targetDir is safe for `source delete` to remove.
//
// It refuses (ErrSourceDeleteRefused) when:
//   - targetDir is the local component directory of a component in stackComponents that has no
//     `source:` (a hand-written component that merely shares a directory with the instance), or
//   - targetDir carries no evidence the source provisioner created it (see HasProvenance).
//
// stackComponents is the `components.<type>` map of the stack being operated on (component
// instance name to its section). The check is read-only.
func CheckDeletable(
	atmosConfig *schema.AtmosConfiguration,
	componentType string,
	targetDir string,
	stackComponents map[string]any,
) error {
	defer perf.Track(atmosConfig, "source.CheckDeletable")()

	if owner := sourcelessOwner(atmosConfig, componentType, targetDir, stackComponents); owner != "" {
		return errUtils.Build(errUtils.ErrSourceDeleteRefused).
			WithExplanationf("`%s` is the local component directory of `%s`, a component in this stack that has no `source:`", targetDir, owner).
			WithHintf("`source delete` only removes directories the source provisioner created; delete `%s` by hand if you really want it gone, or point the sourced component at a different `metadata.component`", targetDir).
			WithContext("path", targetDir).
			WithContext("owner_component", owner).
			Err()
	}

	if !HasProvenance(targetDir) {
		return errUtils.Build(errUtils.ErrSourceDeleteRefused).
			WithExplanationf("`%s` carries no marker showing the source provisioner created it (`.atmos/%s`)", targetDir, ProvenanceFile).
			WithHint("The directory may be hand-written or vendored by an older Atmos version; run `source pull --force` to re-provision it (which adds the marker), or delete it by hand").
			WithContext("path", targetDir).
			Err()
	}
	return nil
}

// sourcelessOwner returns the name of a component without `source:` whose local component
// directory is targetDir, or "" if there is none. Names are visited in sorted order so the
// reported owner is deterministic.
func sourcelessOwner(atmosConfig *schema.AtmosConfiguration, componentType, targetDir string, stackComponents map[string]any) string {
	names := make([]string, 0, len(stackComponents))
	for name := range stackComponents {
		names = append(names, name)
	}
	sort.Strings(names)

	want := canonicalDir(targetDir)
	for _, name := range names {
		section, ok := stackComponents[name].(map[string]any)
		if !ok || HasSource(section) {
			continue
		}
		dir, ok := localComponentDir(atmosConfig, componentType, name, section)
		if ok && canonicalDir(dir) == want {
			return name
		}
	}
	return ""
}

// localComponentDir returns the directory a component without `source:` runs from.
func localComponentDir(atmosConfig *schema.AtmosConfiguration, componentType, name string, section map[string]any) (string, bool) {
	if dir := getWorkingDirectoryOverride(section); dir != "" {
		return dir, true
	}
	component := extractComponentName(section)
	if component == "" {
		component = name
	}
	base, err := resolveComponentBasePath(atmosConfig, componentType)
	if err != nil {
		return "", false
	}
	return filepath.Join(base, component), true
}

// canonicalDir returns an absolute, symlink-resolved form of dir for comparison.
func canonicalDir(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return filepath.Clean(dir)
	}
	resolved, err := resolveExistingSymlinks(abs)
	if err != nil {
		return abs
	}
	return resolved
}
