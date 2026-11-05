package catalog

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/source"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/project/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Resolve selects a scaffold with configured names overriding embedded/catalog
// names. The configDir argument is the directory containing the atmos.yaml that
// declares scaffold.templates, and relative local template sources resolve from
// it. Only the selected source is hydrated. The caller must call cleanup.
func Resolve(name, ref, configDir, exclude string) (*templates.Configuration, func(), error) {
	defer perf.Track(nil, "catalog.Resolve")()

	noop := func() {}
	section, err := config.ReadAtmosScaffoldSection(configDir)
	if err != nil {
		return nil, noop, err
	}
	selected, err := resolveConfigured(name, section, configDir, exclude)
	return hydrateSelected(name, ref, &selected, err)
}

// ResolveWithConfig selects a template from the effective invocation configuration.
// Each relative source is anchored to the file that supplied that source, even
// when a later profile overrides only its description.
func ResolveWithConfig(name, ref, exclude string, scaffold *schema.ScaffoldConfiguration) (*templates.Configuration, func(), error) {
	defer perf.Track(nil, "catalog.ResolveWithConfig")()

	var selected templates.Configuration
	var err error
	if scaffold != nil {
		if entry, ok := scaffold.Templates[name]; ok {
			selected, err = fromConfigAt(name, entry, scaffold.SourceDirs[name], exclude)
		}
	}
	return hydrateSelected(name, ref, &selected, err)
}

func hydrateSelected(name, ref string, selected *templates.Configuration, err error) (*templates.Configuration, func(), error) {
	noop := func() {}
	if err != nil {
		return nil, noop, err
	}
	if selected.Name == "" {
		*selected, err = resolveDefault(name)
		if err != nil {
			return nil, noop, err
		}
	}
	selected.Source = source.WithRef(selected.Source, ref)
	cleanup, err := source.Hydrate(selected, "")
	return selected, cleanup, err
}

func resolveConfigured(name string, section map[string]any, configDir, exclude string) (templates.Configuration, error) {
	raw, exists := section["templates"]
	if !exists {
		return templates.Configuration{}, nil
	}
	entries, ok := raw.(map[string]any)
	if !ok {
		return templates.Configuration{}, errUtils.ErrInvalidScaffoldConfig
	}
	entry, found := entries[name]
	if !found {
		return templates.Configuration{}, nil
	}
	return fromConfigAt(name, entry, configDir, exclude)
}

func resolveDefault(name string) (templates.Configuration, error) {
	embedded, err := templates.GetAvailableConfigurations()
	if err != nil {
		return templates.Configuration{}, err
	}
	if template, ok := embedded[name]; ok {
		return template, nil
	}
	catalog, err := templates.CatalogStubs("")
	if err != nil {
		return templates.Configuration{}, err
	}
	if template, ok := catalog[name]; ok {
		return template, nil
	}
	if source.IsTemplateSource(name) {
		return templates.Configuration{Name: name, Source: name}, nil
	}
	return templates.Configuration{}, fmt.Errorf("%w: %s", errUtils.ErrScaffoldNotFound, name)
}
