// Package discovery builds a cached metadata index from OCI repositories and catalogs.
package discovery

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	pluginconfig "github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/schema"
)

const defaultTTL = 15 * time.Minute

// Result retains the configured source alias for unambiguous installation.
type Result struct {
	Registry      string `json:"registry" yaml:"registry"`
	catalog.Entry `yaml:",inline"`
}

// Request selects discovery sources and optionally bypasses cache freshness.
type Request struct {
	Registry string
	Filter   catalog.Filter
	Refresh  bool
}

// Service shares the installer's authenticated, policy-governed OCI transport.
type Service struct {
	Config      *schema.AtmosConfiguration
	Registry    atmosoci.Registry
	Descriptors *extension.Registry
}

type source struct {
	alias, reference, kind string
	optional               bool
}

// Search refreshes metadata only; it never downloads or executes plugin payloads.
func (s *Service) Search(ctx context.Context, request Request) ([]Result, error) {
	defer perf.Track(s.Config, "plugindiscovery.Search")()

	if err := pluginconfig.Validate(&s.Config.Plugins); err != nil {
		return nil, err
	}
	if request.Refresh && s.Config.Plugins.Policy.Offline {
		return nil, errUtils.ErrPluginOfflineArtifact
	}
	sources, err := s.sources(request.Registry)
	if err != nil {
		return nil, err
	}
	if len(sources) == 0 {
		return []Result{}, nil
	}
	cache, err := s.newCache()
	if err != nil {
		return nil, err
	}
	entries, err := s.collect(ctx, cache, sources, request.Refresh)
	if err != nil {
		return nil, err
	}
	return searchEntries(sources, entries, request.Filter)
}

func (s *Service) sources(selected string) ([]source, error) {
	registries := pluginconfig.EffectiveRegistries(&s.Config.Plugins)
	if _, ok := registries[selected]; selected != "" && !ok {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	result := []source{}
	for _, alias := range slices.Sorted(maps.Keys(registries)) {
		if selected != "" && alias != selected {
			continue
		}
		registry := registries[alias]
		entries, err := s.registrySources(alias, selected, &registry)
		if err != nil {
			return nil, err
		}
		result = append(result, entries...)
	}
	return result, nil
}

func (s *Service) registrySources(alias, selected string, registry *schema.PluginRegistry) ([]source, error) {
	result := []source{}
	if _, err := pluginconfig.RegistryHost(registry); err != nil {
		return nil, err
	}
	repositories := slices.Clone(registry.Repositories)
	slices.Sort(repositories)
	for _, repository := range slices.Compact(repositories) {
		result = append(result, source{alias: alias, reference: repository, kind: "repository"})
	}
	if registry.Catalog != "" {
		_, explicit := s.Config.Plugins.Registries[alias]
		optional := selected == "" && alias == pluginconfig.DefaultRegistryAlias && !explicit
		result = append(result, source{alias: alias, reference: registry.Catalog, kind: "catalog", optional: optional})
	}
	return result, nil
}

func (s *Service) collect(ctx context.Context, cache *indexCache, sources []source, refresh bool) ([][]catalog.Entry, error) {
	limit := s.Config.Plugins.MaxConcurrency
	if limit == 0 {
		limit = 4
	}
	group, workerContext := errgroup.WithContext(ctx)
	group.SetLimit(limit)
	results := make([][]catalog.Entry, len(sources))
	for i, item := range sources {
		group.Go(func() error {
			if err := workerContext.Err(); err != nil {
				return err
			}
			entries, err := s.loadSource(workerContext, cache, item, refresh)
			if err != nil {
				return fmt.Errorf("%w: discovery source %s", err, item.alias)
			}
			results[i] = entries
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return results, ctx.Err()
}

func searchEntries(sources []source, entries [][]catalog.Entry, filter catalog.Filter) ([]Result, error) {
	byAlias := make(map[string][]catalog.Entry)
	for i, items := range entries {
		for j := range items {
			entry := &items[j]
			merged, err := mergeEntry(byAlias[sources[i].alias], entry)
			if err != nil {
				return nil, err
			}
			byAlias[sources[i].alias] = merged
		}
	}
	result := []Result{}
	for _, alias := range slices.Sorted(maps.Keys(byAlias)) {
		file := catalog.New(alias)
		file.Spec.Entries = byAlias[alias]
		sort.Slice(file.Spec.Entries, func(i, j int) bool { return file.Spec.Entries[i].Name < file.Spec.Entries[j].Name })
		matched := catalog.Search(file, filter)
		for i := range matched {
			result = append(result, Result{Registry: alias, Entry: matched[i]})
		}
	}
	return result, nil
}
