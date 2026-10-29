package bootstrap

import (
	"context"
	"maps"
	"slices"

	authregistry "github.com/cloudposse/atmos/pkg/auth/registry"
	"github.com/cloudposse/atmos/pkg/plugin/config"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	"github.com/cloudposse/atmos/pkg/schema"
)

// neededPackages chooses which declared packages an ordinary command must activate.
//
// An ordinary command (describe, list, and validate) needs a plugin only when its configuration
// references an extension kind that Atmos does not provide, today an auth provider or identity
// kind. The lock records every package's exports, so the decision needs no network and no
// installation:
//
//  1. Collect the non-built-in provider and identity kinds the configuration references.
//     With none, no package is needed and nothing is installed, locked, downloaded or opened.
//  2. A path package whose local manifest exports a referenced kind is needed. Its manifest is read
//     from disk; an unreadable one is needed too, so activation reports the problem instead of
//     hiding it. Kinds it satisfies are never fetched from a registry.
//  3. A locked package whose exports satisfy a referenced kind is needed. Its release is cached
//     already or restored by exact digest.
//  4. Kinds that nothing satisfies cannot be resolved without installing. As a fallback every enabled
//     release package that has no lock entry yet is needed, because one of them must export the kind.
//
// Disabled packages are never needed. A path package is local, so it needs no restore, but like
// every package it is activated only when a command needs it.
func neededPackages(atmos *schema.AtmosConfiguration, state *config.Selections, file *pluginlock.File) map[string]schema.PluginPackage {
	needed := make(map[string]schema.PluginPackage)
	kinds := referencedKinds(atmos)
	if len(kinds) == 0 {
		return needed
	}
	collectNeededLocalPackages(atmos.Plugins.Packages, state, kinds, needed)
	collectNeededLockedPackages(atmos.Plugins.Packages, state, file, kinds, needed)
	return needed
}

func collectNeededLocalPackages(declarations map[string]schema.PluginPackage, state *config.Selections, kinds map[string]bool, needed map[string]schema.PluginPackage) {
	aliases := slices.Sorted(maps.Keys(declarations))
	// Path packages resolve first so the kinds they satisfy are never fetched from a registry.
	for _, alias := range aliases {
		selection, local := state.Packages[alias]
		declaration := declarations[alias]
		if !local || !enabled(&declaration) {
			continue
		}
		satisfied, readable := localSatisfied(&selection, kinds)
		if !readable || len(satisfied) != 0 {
			needed[alias] = declarations[alias]
		}
		for kind := range satisfied {
			delete(kinds, kind)
		}
	}
}

func collectNeededLockedPackages(declarations map[string]schema.PluginPackage, state *config.Selections, file *pluginlock.File, kinds map[string]bool, needed map[string]schema.PluginPackage) {
	aliases := slices.Sorted(maps.Keys(declarations))
	var unlocked []string
	for _, alias := range aliases {
		declaration := declarations[alias]
		if _, local := state.Packages[alias]; local || !enabled(&declaration) {
			continue
		}
		entry, locked := file.Spec.Packages[alias]
		if !locked {
			unlocked = append(unlocked, alias)
			continue
		}
		if satisfied := satisfiedKinds(entry.Exports, kinds); len(satisfied) != 0 {
			needed[alias] = declaration
			for kind := range satisfied {
				delete(kinds, kind)
			}
		}
	}
	if len(kinds) != 0 {
		for _, alias := range unlocked {
			needed[alias] = declarations[alias]
		}
	}
}

// enabled reports whether a declaration is not switched off with enabled: false.
func enabled(declaration *schema.PluginPackage) bool {
	return declaration.Enabled == nil || *declaration.Enabled
}

// referencedKinds lists provider and identity kinds that no built-in implementation provides.
func referencedKinds(atmos *schema.AtmosConfiguration) map[string]bool {
	kinds := make(map[string]bool)
	// A fresh registry holds only built-ins, so temporary plugin registrations never hide a kind.
	builtins := authregistry.NewRegistry()
	for name := range atmos.Auth.Providers {
		if kind := atmos.Auth.Providers[name].Kind; kind != "" && !builtins.HasProvider(kind) {
			kinds[kind] = true
		}
	}
	for name := range atmos.Auth.Identities {
		if kind := atmos.Auth.Identities[name].Kind; kind != "" && !builtins.HasIdentity(kind) {
			kinds[kind] = true
		}
	}
	return kinds
}

func satisfiedKinds(exports []pluginlock.Export, kinds map[string]bool) map[string]bool {
	result := make(map[string]bool)
	for _, export := range exports {
		if (export.Capability == "auth.provider" || export.Capability == "auth.identity") && kinds[export.Name] {
			result[export.Name] = true
		}
	}
	return result
}

// localSatisfied lists the referenced kinds a path package exports and whether its manifest could be
// read. An unreadable manifest contributes nothing; activation reports its error.
func localSatisfied(selection *config.Selection, kinds map[string]bool) (satisfied map[string]bool, readable bool) {
	satisfied = make(map[string]bool)
	registry, err := Descriptors()
	if err != nil {
		return satisfied, false
	}
	definition, err := manifest.Load(selection.Path)
	if err != nil {
		return satisfied, false
	}
	bindings, err := registry.Discover(definition.Spec.Capabilities)
	if err != nil {
		return satisfied, false
	}
	for _, binding := range bindings {
		if (binding.Capability == "auth.provider" || binding.Capability == "auth.identity") && kinds[binding.Name] {
			satisfied[binding.Name] = true
		}
	}
	return satisfied, true
}

// selectedForUse activates the packages the command needs. Packages that nothing references are
// neither installed, locked, opened nor warned about.
func selectedForUse(ctx context.Context, atmos *schema.AtmosConfiguration) (*config.Selections, error) {
	state, err := readSelections(ctx, atmos)
	if err != nil {
		return nil, err
	}
	file, err := pluginlock.Load(ProjectDirectory(atmos))
	if err != nil {
		return nil, err
	}
	restricted := *atmos
	restricted.Plugins.Packages = neededPackages(atmos, state, file)
	// Disabled declarations stay so that selection can drop their selections.
	for alias, declaration := range atmos.Plugins.Packages {
		if declaration.Enabled != nil && !*declaration.Enabled {
			restricted.Plugins.Packages[alias] = declaration
		}
	}
	return selectedPackages(ctx, &restricted)
}
