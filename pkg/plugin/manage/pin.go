package manage

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
)

// wildcard resolves the newest release when the caller supplies no constraint.
const wildcard = "*"

// pinConstraint converts the release that was actually installed into the constraint recorded
// for a new declaration without --version. Stable releases get a caret pin (^1.4.2 allows the
// same major; ^0.1.0 pins the minor because caret treats 0.x minors as breaking). A pre-release
// is pinned exactly because caret ranges never advance across pre-release tags.
func pinConstraint(installed string) string {
	version, err := semver.StrictNewVersion(installed)
	if err != nil {
		return installed
	}
	if version.Prerelease() != "" {
		return version.String()
	}
	return fmt.Sprintf("^%d.%d.%d", version.Major(), version.Minor(), version.Patch())
}

// addPlan is the decision made before any network access.
type addPlan struct {
	alias      string
	constraint string
	// explicit means --version was supplied and is stored verbatim.
	explicit bool
	// pin means the declaration is new and the constraint is derived after installation.
	pin bool
}

// planAddition derives the alias first, so the previous entry is found even when --as is omitted,
// then chooses the constraint: --version always wins; an existing declaration for the same source
// keeps its constraint; a new declaration is pinned from the release actually installed.
func (m *Manager) planAddition(options *AddOptions, file *pluginlock.File) (addPlan, error) {
	alias, err := m.resolveAlias(options, file)
	if err != nil {
		return addPlan{}, err
	}
	plan := addPlan{alias: alias, constraint: options.Constraint, explicit: options.Constraint != ""}
	if plan.explicit {
		return plan, nil
	}
	if declared, found := m.Config.Plugins.Packages[alias]; found && alias != "" && declared.Source == options.Source && declared.Version != "" {
		plan.constraint = declared.Version
		return plan, nil
	}
	plan.constraint, plan.pin = wildcard, true
	return plan, nil
}

// resolveAlias returns the explicit alias, or the unique alias already bound to the source.
// An empty result means the package is new and its alias comes from the manifest name.
func (m *Manager) resolveAlias(options *AddOptions, file *pluginlock.File) (string, error) {
	if options.Alias != "" {
		if !aliasPattern.MatchString(options.Alias) {
			return "", invalidAlias(options.Alias)
		}
		return options.Alias, nil
	}
	matchesByAlias := make(map[string]bool)
	for alias, declared := range m.Config.Plugins.Packages {
		if declared.Source == options.Source {
			matchesByAlias[alias] = true
		}
	}
	for alias := range file.Spec.Packages {
		if entry := file.Spec.Packages[alias]; entry.Source == options.Source {
			matchesByAlias[alias] = true
		}
	}
	matches := slices.Sorted(maps.Keys(matchesByAlias))
	switch len(matches) {
	case 0:
		return "", nil
	case 1:
		return matches[0], nil
	}
	return "", errUtils.Build(errUtils.ErrPluginInvalidConfig).
		WithExplanationf("The source `%s` is already installed under several aliases: %v.", options.Source, matches).
		WithHint("Pass `--as <alias>` to choose which declaration to change.").
		Err()
}

func invalidAlias(alias string) error {
	return errUtils.Build(errUtils.ErrPluginInvalidConfig).
		WithExplanationf("`%s` is not a valid plugin alias.", alias).
		WithHint("Aliases start with a lowercase letter and contain only lowercase letters, digits, `-` and `_`.").
		Err()
}

// defaultAlias derives the short product alias used when no alias is given.
func defaultAlias(packageID string) string {
	_, product, _ := strings.Cut(packageID, "/")
	return product
}
