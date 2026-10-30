// Package manage commits explicit plugin lifecycle changes after artifact preparation.
package manage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var aliasPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// Manager owns one scope's configuration and release lock transaction.
type Manager struct {
	Project    string
	ConfigFile string
	Config     *schema.AtmosConfiguration
	Installer  *install.Service
}

// AddOptions selects an explicit package declaration and optional offline layout.
type AddOptions struct{ Alias, Source, Constraint, Layout, PinnedDigest, SelectedVersion string }

// Add installs a package and preserves unrelated root configuration and comments.
//
// The alias is derived before the previous entry is looked up. Without --version an existing
// declaration for the same source keeps its constraint, and a new declaration is pinned with a
// caret from the release actually installed. An unchanged install writes nothing.
//
//nolint:gocritic // Copy options so defaulting never mutates the caller’s request.
func (m *Manager) Add(ctx context.Context, options AddOptions) error {
	defer perf.Track(m.Config, "pluginmanage.Manager.Add")()

	return config.Mutate(ctx, m.Project, func() ([]config.Change, error) {
		file, err := pluginlock.Load(m.Project)
		if err != nil {
			return nil, err
		}
		plan, err := m.planAddition(&options, file)
		if err != nil {
			return nil, err
		}
		entry, err := m.prepareAddition(ctx, &options, &plan, file)
		if err != nil {
			return nil, err
		}
		if plan.pin {
			entry.Constraint = pinConstraint(entry.Version)
		}
		alias := plan.alias
		if alias == "" {
			alias = defaultAlias(entry.Plugin)
			if err := m.checkDerivedAlias(alias, options.Source, file); err != nil {
				return nil, err
			}
		}
		if !aliasPattern.MatchString(alias) {
			return nil, invalidAlias(alias)
		}
		previous, hadPrevious := file.Spec.Packages[alias]
		file.Spec.Packages[alias] = entry
		if err := checkCollisions(file); err != nil {
			return nil, err
		}
		return m.additionChanges(alias, &options, &entry, file, hadPrevious && sameEntry(&previous, &entry))
	})
}

// checkDerivedAlias prevents an inferred name from repointing another declaration.
func (m *Manager) checkDerivedAlias(alias, source string, file *pluginlock.File) error {
	if existing, found := m.Config.Plugins.Packages[alias]; found && existing.Source != source {
		return aliasSourceCollision(alias, existing.Source)
	}
	if existing, found := file.Spec.Packages[alias]; found && existing.Source != source {
		return aliasSourceCollision(alias, existing.Source)
	}
	return nil
}

func aliasSourceCollision(alias, source string) error {
	return errUtils.Build(errUtils.ErrPluginInvalidConfig).
		WithExplanationf("The derived alias `%s` is already used by a different plugin source (`%s`).", alias, source).
		WithHint("Pass `--as <alias>` to choose the alias explicitly.").Err()
}

func (m *Manager) additionChanges(alias string, options *AddOptions, entry *pluginlock.Entry, file *pluginlock.File, lockUnchanged bool) ([]config.Change, error) {
	var changes []config.Change
	declared, found := m.Config.Plugins.Packages[alias]
	if !found || declared.Source != options.Source || declared.Version != entry.Constraint {
		change, err := m.declarationChange(alias, &schema.PluginPackage{Source: options.Source, Version: entry.Constraint})
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}
	if !lockUnchanged {
		lockChange, err := stageLock(m.Project, file)
		if err != nil {
			return nil, err
		}
		changes = append(changes, lockChange)
	}
	return changes, nil
}

func sameEntry(first, second *pluginlock.Entry) bool {
	return equalEntries(map[string]pluginlock.Entry{"entry": *first}, map[string]pluginlock.Entry{"entry": *second})
}

func (m *Manager) prepareAddition(ctx context.Context, options *AddOptions, plan *addPlan, file *pluginlock.File) (pluginlock.Entry, error) {
	label := plan.alias
	if label == "" {
		label = options.Source
	}
	request := &install.Request{Source: options.Source, Constraint: plan.constraint, Layout: options.Layout, PinnedDigest: options.PinnedDigest, SelectedVersion: options.SelectedVersion, Alias: plan.alias}
	if previous, exists := file.Spec.Packages[plan.alias]; exists && plan.alias != "" {
		request.Previous = &previous
		// An unchanged declaration is restored from its lock: install never advances a locked release.
		if previous.Source == options.Source && previous.Constraint == plan.constraint && options.PinnedDigest == "" {
			request.Locked = &previous
		}
	}
	entries, err := m.Installer.PrepareBatch(ctx, map[string]*install.Request{label: request})
	return entries[label], err
}

// declarationChange edits only the source and version scalars, so block style, comments and
// blank lines in the user's configuration survive. A nil declaration deletes the package.
func (m *Manager) declarationChange(alias string, declaration *schema.PluginPackage) (config.Change, error) {
	before, err := readOptional(filepath.Join(m.Project, m.ConfigFile))
	if err != nil {
		return config.Change{}, err
	}
	path := []string{"plugins", "packages", alias}
	if declaration == nil {
		after, err := config.DeleteKey(before, path)
		return config.Change{Name: m.ConfigFile, Before: before, After: after}, err
	}
	after, err := config.SetFields(before, path, []config.Field{{Key: "source", Value: declaration.Source}, {Key: "version", Value: declaration.Version}})
	return config.Change{Name: m.ConfigFile, Before: before, After: after}, err
}

func stageLock(project string, file *pluginlock.File) (config.Change, error) {
	before, err := readOptional(pluginlock.Path(project))
	if err != nil {
		return config.Change{}, err
	}
	after, err := yaml.Marshal(file)
	return config.Change{Name: filepath.Base(pluginlock.Path(project)), Before: before, After: after}, err
}

func readOptional(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return raw, err
}

// RestoreOptions controls locked restoration and explicit version advancement.
type RestoreOptions struct {
	Frozen bool
	Update bool
	Alias  string
	Layout string
}

// Restore materializes declarations; frozen restores never rewrite either file.
func (m *Manager) Restore(ctx context.Context, options RestoreOptions) error {
	defer perf.Track(m.Config, "pluginmanage.Manager.Restore")()

	return config.Mutate(ctx, m.Project, func() ([]config.Change, error) {
		file, err := pluginlock.Load(m.Project)
		if err != nil {
			return nil, err
		}
		if options.Frozen {
			if err := m.refuseFrozenLocal(); err != nil {
				return nil, err
			}
			if err := pluginlock.CheckDeclarations(enabledLock(file, m.Config.Plugins.Packages), enabledDeclarations(m.Config.Plugins.Packages)); err != nil {
				return nil, err
			}
		}
		previous := maps.Clone(file.Spec.Packages)
		if err := m.prepareDeclared(ctx, file, &options); err != nil {
			return nil, err
		}
		if err := checkCollisions(file); err != nil {
			return nil, err
		}
		if options.Frozen || equalEntries(previous, file.Spec.Packages) {
			return nil, nil
		}
		change, err := stageLock(m.Project, file)
		return []config.Change{change}, err
	})
}

func (m *Manager) prepareDeclared(ctx context.Context, file *pluginlock.File, options *RestoreOptions) error {
	if options.Alias != "" {
		if _, exists := m.Config.Plugins.Packages[options.Alias]; !exists {
			return errUtils.Build(fmt.Errorf("%w: %s is not declared", errUtils.ErrPluginNotInstalled, options.Alias)).
				WithHintf("Run `atmos plugin list` for declared packages, or `atmos plugin install <source> --as %s` to add it.", options.Alias).
				Err()
		}
	}
	requests := make(map[string]*install.Request)
	for _, alias := range slices.Sorted(maps.Keys(m.Config.Plugins.Packages)) {
		declaration := m.Config.Plugins.Packages[alias]
		if options.Alias != "" && alias != options.Alias {
			continue
		}
		if skipDeclared(alias, &declaration) {
			continue
		}

		request := &install.Request{Source: declaration.Source, Constraint: declaration.Version, Layout: options.Layout, Alias: alias, Frozen: options.Frozen}
		if previous, exists := file.Spec.Packages[alias]; exists {
			if options.Update {
				request.Previous = &previous
			} else {
				request.Locked = &previous
			}
		}
		requests[alias] = request
	}
	entries, err := m.Installer.PrepareBatch(ctx, requests)
	if err != nil {
		return err
	}
	maps.Copy(file.Spec.Packages, entries)
	return nil
}

// refuseFrozenLocal rejects a frozen install while the active configuration runs a package from a
// local path: a frozen install reproduces released packages from the lock, and a local build has none.
func (m *Manager) refuseFrozenLocal() error {
	var aliases []string
	for _, alias := range slices.Sorted(maps.Keys(m.Config.Plugins.Packages)) {
		declaration := m.Config.Plugins.Packages[alias]
		if config.IsLocal(&declaration) {
			aliases = append(aliases, alias)
		}
	}
	if len(aliases) == 0 {
		return nil
	}
	hint := fmt.Sprintf("The active configuration sets `path` for %s, which runs a local build with no lock entry. Frozen installs are for released packages: run without `--frozen`, or use a configuration that does not set `path`", "`"+strings.Join(aliases, "`, `")+"`")
	if profiles := m.Config.ActiveProfiles; len(profiles) != 0 {
		hint += fmt.Sprintf(" (active profiles: %s)", strings.Join(profiles, ", "))
	}
	return errUtils.Build(fmt.Errorf("%w: plugins.packages.%s.path is set", errUtils.ErrPluginFrozenDrift, aliases[0])).WithHint(hint + ".").Err()
}

func equalEntries(first, second map[string]pluginlock.Entry) bool {
	a, firstErr := json.Marshal(first)
	b, secondErr := json.Marshal(second)
	return firstErr == nil && secondErr == nil && string(a) == string(b)
}

func disabled(declaration *schema.PluginPackage) bool {
	return declaration.Enabled != nil && !*declaration.Enabled
}

func enabledDeclarations(declarations map[string]schema.PluginPackage) map[string]schema.PluginPackage {
	result := make(map[string]schema.PluginPackage, len(declarations))
	for alias := range declarations {
		if declaration := declarations[alias]; !disabled(&declaration) {
			result[alias] = declaration
		}
	}
	return result
}

// enabledLock hides disabled packages from the frozen comparison: they are neither installed nor drift.
func enabledLock(file *pluginlock.File, declarations map[string]schema.PluginPackage) *pluginlock.File {
	view := *file
	view.Spec.Packages = make(map[string]pluginlock.Entry, len(file.Spec.Packages))
	for alias := range file.Spec.Packages {
		if declaration, found := declarations[alias]; !found || !disabled(&declaration) {
			view.Spec.Packages[alias] = file.Spec.Packages[alias]
		}
	}
	return &view
}

func skipDeclared(alias string, declaration *schema.PluginPackage) bool {
	if disabled(declaration) {
		ui.Infof("Skipping plugin `%s`: it is disabled by `enabled: false` in plugins.packages.%s.", alias, alias)
		return true
	}
	if config.IsLocal(declaration) {
		ui.Infof("Skipping plugin `%s`: plugins.packages.%s.path runs a local build, which is never fetched or locked.", alias, alias)
		return true
	}
	return false
}
