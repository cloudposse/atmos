package manage

import (
	"context"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Remove removes a release declaration and lock only after checking auth references.
// Shared immutable cache entries remain available for other projects and rollback.
func (m *Manager) Remove(ctx context.Context, alias string) error {
	defer perf.Track(m.Config, "pluginmanage.Manager.Remove")()

	if !aliasPattern.MatchString(alias) {
		return invalidAlias(alias)
	}
	return config.Mutate(ctx, m.Project, func() ([]config.Change, error) {
		if declaration, found := m.Config.Plugins.Packages[alias]; found && config.IsLocal(&declaration) {
			return nil, errUtils.Build(fmt.Errorf("%w: %s runs from plugins.packages.%s.path", errUtils.ErrPluginPathPackage, alias, alias)).
				WithHintf("The path comes from configuration, not from a file `atmos plugin remove` created. Edit the profile or `atmos.yaml` that sets `plugins.packages.%s.path` (%s), or deactivate that profile%s.", alias, config.DisplayPath(declaration.Path), activeProfileNote(m.Config)).
				Err()
		}
		file, err := pluginlock.Load(m.Project)
		if err != nil {
			return nil, err
		}
		entry, found := file.Spec.Packages[alias]
		if !found {
			return nil, errUtils.Build(fmt.Errorf("%w: %s is not in the release lock", errUtils.ErrPluginNotInstalled, alias)).
				WithHint("Run `atmos plugin list` to see declared packages and local path packages.").
				Err()
		}
		if err := m.checkReferences(&entry); err != nil {
			return nil, err
		}
		delete(file.Spec.Packages, alias)
		declaration, err := m.declarationChange(alias, nil)
		if err != nil {
			return nil, err
		}
		lock, err := stageLock(m.Project, file)
		if err != nil {
			return nil, err
		}
		return []config.Change{declaration, lock}, nil
	})
}

func (m *Manager) checkReferences(entry *pluginlock.Entry) error {
	references := make(map[string]string)
	for name := range m.Config.Auth.Providers {
		provider := m.Config.Auth.Providers[name]
		references["auth.provider:"+provider.Kind] = "auth.providers." + name
	}
	for name := range m.Config.Auth.Identities {
		identity := m.Config.Auth.Identities[name]
		references["auth.identity:"+identity.Kind] = "auth.identities." + name
	}
	for _, export := range entry.Exports {
		if path, found := references[export.Capability+":"+export.Name]; found {
			return fmt.Errorf("%w: %s still uses %s", errUtils.ErrPluginInUse, path, export.Name)
		}
	}
	return nil
}

// activeProfileNote names the active profiles when the loaded configuration retained them.
func activeProfileNote(atmos *schema.AtmosConfiguration) string {
	if len(atmos.ActiveProfiles) == 0 {
		return ""
	}
	return " (active profiles: " + strings.Join(atmos.ActiveProfiles, ", ") + ")"
}
