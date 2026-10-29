package manage

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	authregistry "github.com/cloudposse/atmos/pkg/auth/registry"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
)

// checkCollisions rejects exports that two packages share or that shadow a built-in. Aliases
// are visited in sorted order, so the message names the same owners on every run.
func checkCollisions(file *pluginlock.File) error {
	builtins := authregistry.NewRegistry()
	checks := map[string]func(string) bool{"auth.provider": builtins.HasProvider, "auth.identity": builtins.HasIdentity}
	aliases := slices.Sorted(maps.Keys(file.Spec.Packages))
	owners := make(map[string][]string)
	for _, alias := range aliases {
		entry := file.Spec.Packages[alias]
		for _, export := range entry.Exports {
			key := export.Capability + ":" + export.Name
			if !slices.Contains(owners[key], alias) {
				owners[key] = append(owners[key], alias)
			}
		}
	}
	for _, alias := range aliases {
		entry := file.Spec.Packages[alias]
		for _, export := range entry.Exports {
			key := export.Capability + ":" + export.Name
			if len(owners[key]) > 1 {
				return errUtils.Build(fmt.Errorf("%w: %s is exported by %s", errUtils.ErrDuplicatePluginExtension, key, joinOwners(owners[key]))).
					WithHint("Remove or pin only one of these plugins, or choose a plugin version that renames the colliding export.").Err()
			}
			if check := checks[export.Capability]; check != nil && check(export.Name) {
				return errUtils.Build(fmt.Errorf("%w: %s conflicts with a built-in (exported by %s)", errUtils.ErrDuplicatePluginExtension, key, alias)).
					WithHint("Built-in providers and identities cannot be replaced; use a plugin version that exports a distinct name.").Err()
			}
		}
	}
	return nil
}

// joinOwners renders "a and b" or "a, b, and c".
func joinOwners(aliases []string) string {
	if len(aliases) == 2 {
		return aliases[0] + " and " + aliases[1]
	}
	return strings.Join(aliases[:len(aliases)-1], ", ") + ", and " + aliases[len(aliases)-1]
}
