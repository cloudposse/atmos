// Package configuration connects native configuration providers to Atmos stack definitions.
package configuration

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	adapter "github.com/cloudposse/atmos/pkg/plugin/adapter/component"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Provider is the typed component capability boundary used by a discovery session.
//
//go:generate go run go.uber.org/mock/mockgen -destination=mock_provider_test.go -package=configuration . Provider
type Provider interface {
	ComponentType() string
	Discover(context.Context, *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error)
	Prepare(context.Context, *pluginv1.ComponentRequest) (*pluginv1.PrepareComponentResponse, error)
}

// Loader resolves the requested configuration providers through package policy and locks.
type Loader func(context.Context, []string) (map[string]Provider, error)

// Session holds one command's discovery result; it is never a process-global cache.
type Session struct {
	ctx             context.Context
	root, command   string
	requests        []*pluginv1.DiscoverComponentsRequest
	loader          Loader
	once            sync.Once
	done            chan struct{}
	registry        map[string]Provider
	definitions     map[string]map[string]any
	instances       map[[3]string]*pluginv1.DiscoveredComponent
	failures        map[failedKey]*failedUnit
	err             error
	importMu        sync.Mutex
	importProviders map[string]*pluginv1.DiscoverComponentsRequest
	// importResults caches each resolved import by URI; imported indexes the same units by
	// source so Prepare can find a unit under whichever stack imported it.
	importResults map[string][]*pluginv1.DiscoveredComponent
	imported      map[string]*pluginv1.DiscoveredComponent
	started       bool
	loadMu        sync.Mutex
	// configErr holds a settings problem found while reading configuration. Reading never fails on it,
	// so `atmos config set` can always repair the file; the first use of the provider reports it.
	configErr error
	// options holds the provider-owned keys of each enabled section, by section name.
	options map[string]map[string]any
}

// New snapshots the explicitly enabled scope without starting plugins or scanning directories.
func New(ctx context.Context, config *schema.AtmosConfiguration, command string, loader Loader) (*Session, error) {
	defer perf.Track(config, "configurationadapter.New")()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root := config.BasePathAbsolute
	if !filepath.IsAbs(root) || loader == nil {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	s := &Session{ctx: ctx, root: root, command: command, loader: loader, done: make(chan struct{}), importProviders: make(map[string]*pluginv1.DiscoverComponentsRequest), importResults: make(map[string][]*pluginv1.DiscoveredComponent), imported: make(map[string]*pluginv1.DiscoveredComponent), options: make(map[string]map[string]any)}
	seen := make(map[string]bool)
	for _, name := range slices.Sorted(maps.Keys(config.Components.Terraform.ConfigurationProviders)) {
		settings := config.Components.Terraform.ConfigurationProviders[name]
		if !settings.Enabled {
			continue
		}

		scheme := name
		if settings.Provider != "" {
			name = settings.Provider
		}
		if seen[name] {
			s.configErr = errors.Join(s.configErr, errUtils.Build(errUtils.ErrDuplicatePluginExtension).
				WithExplanationf("More than one section under `components.terraform` enables the configuration provider `%s`.", name).
				WithContext("config_key", settingsKey(scheme)).
				WithHint("Keep one enabled section per provider, or point the other section at a different provider with `provider:`.").Err())
			continue
		}
		seen[name] = true
		if err := validateSettings(scheme, &settings); err != nil {
			s.configErr = errors.Join(s.configErr, err)
			continue
		}
		request, err := discoveryRequest(config, &settings, name, root, command)
		if err != nil {
			s.configErr = errors.Join(s.configErr, invalidSetting(settingsKey(scheme), "The settings cannot be read", "Remove keys under the section that repeat a built-in setting such as `enabled`, `provider`, `mode`, or `discovery`."))
			continue
		}
		s.options[scheme] = settings.Options
		s.importProviders[scheme] = proto.Clone(request).(*pluginv1.DiscoverComponentsRequest)
		if settings.Discovery.Enabled {
			s.requests = append(s.requests, request)
		}
	}
	return s, nil
}

// Discover returns isolated definitions and shares only immutable evaluator results within this invocation.
func (s *Session) Discover(ctx context.Context) (map[string]map[string]any, error) {
	defer perf.Track(nil, "configurationadapter.Discover")()

	if err := s.ensure(ctx); err != nil {
		return nil, err
	}
	result := make(map[string]map[string]any, len(s.definitions))
	for name, definition := range s.definitions {
		copy, err := merge.DeepCopyMap(definition)
		if err != nil {
			return nil, err
		}
		result[name] = copy
	}
	return result, nil
}

// providers loads every enabled provider once per session, so imports resolved while the
// stack manifests are read and the later discovery share one plugin activation.
func (s *Session) providers(ctx context.Context) (map[string]Provider, error) {
	s.loadMu.Lock()
	defer s.loadMu.Unlock()
	if s.configErr != nil {
		return nil, s.configErr
	}
	if s.registry != nil {
		return s.registry, nil
	}
	names := make([]string, 0, len(s.importProviders))
	for _, request := range s.importProviders {
		if !slices.Contains(names, request.Provider) {
			names = append(names, request.Provider)
		}
	}
	slices.Sort(names)
	registry, err := s.loader(ctx, names)
	if err != nil {
		return nil, err
	}
	if registry == nil {
		registry = map[string]Provider{}
	}
	s.registry = registry
	return registry, nil
}

func (s *Session) ensure(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.ctx.Err(); err != nil {
		return err
	}
	s.once.Do(func() {
		s.importMu.Lock()
		s.started = true
		s.importMu.Unlock()
		go func() {
			defer close(s.done)
			call, cancel := context.WithCancel(s.ctx)
			stop := context.AfterFunc(ctx, cancel)
			defer stop()
			defer cancel()
			s.err = s.discover(call)
		}()
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return s.ctx.Err()
	case <-s.done:
		return s.err
	}
}

func environment() map[string]string {
	result := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}

func discoveryRequest(config *schema.AtmosConfiguration, settings *schema.ComponentConfiguration, name, root, command string) (*pluginv1.DiscoverComponentsRequest, error) {
	value, err := adapter.EncodeObject(settings)
	if err != nil {
		return nil, err
	}
	includes := slices.Clone(settings.Discovery.IncludedPaths)
	switch {
	case !settings.Discovery.Enabled:
		includes = nil
	case len(includes) == 0:
		includes = slices.Clone(defaultIncludedPaths)
	}
	env := environment()
	env["ATMOS_TERRAFORM_BINARY"] = config.Components.Terraform.Command
	return &pluginv1.DiscoverComponentsRequest{Provider: name, RootDirectory: root, IncludePaths: includes, ExcludePaths: slices.Clone(settings.Discovery.ExcludedPaths), ProviderConfiguration: value, Operation: &pluginv1.ComponentOperation{Command: command, Mode: "native"}, Invocation: &pluginv1.Invocation{Environment: env}}, nil
}
