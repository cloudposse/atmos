package catalog

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/source"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/project/config"
)

// Resolve selects a scaffold with configured names overriding embedded/catalog
// names. Only the selected source is hydrated. The caller must call cleanup.
func Resolve(name, ref, exclude string) (*templates.Configuration, func(), error) {
	defer perf.Track(nil, "catalog.Resolve")()

	noop := func() {}
	section, err := config.ReadAtmosScaffoldSection(".")
	if err != nil {
		return nil, noop, err
	}
	selected, err := resolveConfigured(name, section, exclude)
	if err != nil {
		return nil, noop, err
	}
	if selected.Name == "" {
		selected, err = resolveDefault(name)
		if err != nil {
			return nil, noop, err
		}
	}
	selected.Source = source.WithRef(selected.Source, ref)
	cleanup, err := source.Hydrate(&selected, "")
	return &selected, cleanup, err
}

func resolveConfigured(name string, section map[string]any, exclude string) (templates.Configuration, error) {
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
	return FromConfig(name, entry, exclude)
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
