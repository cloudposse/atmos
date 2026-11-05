// Package extension registers extension contracts independently of package lifecycle code.
package extension

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// Binding describes one statically validated export in an installed package.
type Binding struct {
	Capability string
	Name       string
	Directory  string
	Metadata   map[string]any
}

// Activation binds a statically validated export to its verified native package.
type Activation struct {
	Binding
	Package     string
	Executable  string
	Description *pluginv1.DescribeResponse
	// Timeout bounds every call into the plugin; zero means no deadline.
	Timeout time.Duration
}

// Descriptor supplies extension-specific validation, activation, scaffolding,
// and conformance hooks. Discovery and installation use only static validation.
type Descriptor struct {
	Capability string
	Path       []string
	Version    uint32
	Validate   func(Binding) error
	Activate   func(context.Context, *Activation) error
	Scaffold   string
	Test       func(context.Context, Binding) error
}

// Registry owns descriptors for supported extension contracts.
type Registry struct {
	mu          sync.RWMutex
	descriptors map[string]Descriptor
}

// New creates an empty descriptor registry for explicit startup composition.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func New() *Registry { return &Registry{descriptors: make(map[string]Descriptor)} }

// Register adds a capability selector and rejects overlapping metadata paths.
//
//nolint:gocritic // Copies the registration before cloning its path; callers retain their original descriptor.
func (r *Registry) Register(descriptor Descriptor) error {
	defer perf.Track(nil, "extension.Registry.Register")()

	if err := validateDescriptor(&descriptor); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Visit descriptors in capability order so a conflict names the same owner on every run.
	for _, capability := range slices.Sorted(maps.Keys(r.descriptors)) {
		existing := r.descriptors[capability]
		if existing.Capability == descriptor.Capability || overlaps(existing.Path, descriptor.Path) {
			return fmt.Errorf("%w: %s conflicts with %s", errUtils.ErrDuplicatePluginExtension, descriptor.Capability, existing.Capability)
		}
	}
	descriptor.Path = slices.Clone(descriptor.Path)
	if r.descriptors == nil {
		r.descriptors = make(map[string]Descriptor)
	}
	r.descriptors[descriptor.Capability] = descriptor
	return nil
}

func overlaps(first, second []string) bool {
	shared := min(len(first), len(second))
	return slices.Equal(first[:shared], second[:shared])
}

// Get returns a descriptor with an isolated metadata path.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (r *Registry) Get(capability string) (Descriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, found := r.descriptors[capability]
	value.Path = slices.Clone(value.Path)
	return value, found
}

// Validate discovers every declared export and rejects unsupported metadata.
// A package is accepted as a whole before any adapter is activated.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (r *Registry) Validate(directory string, capabilities map[string]any) ([]Binding, error) {
	return r.discover(directory, capabilities, true)
}

// Discover reads export selectors for catalog metadata without resolving schemas.
// Discovery never implies that a package is installable or trusted.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func (r *Registry) Discover(capabilities map[string]any) ([]Binding, error) {
	return r.discover("", capabilities, false)
}

func (r *Registry) discover(directory string, capabilities map[string]any, validate bool) ([]Binding, error) {
	defer perf.Track(nil, "extension.Registry.Validate")()

	r.mu.RLock()
	descriptors := make([]Descriptor, 0, len(r.descriptors))
	for _, descriptor := range r.descriptors {
		descriptors = append(descriptors, descriptor)
	}
	r.mu.RUnlock()
	sort.Slice(descriptors, func(a, b int) bool { return descriptors[a].Capability < descriptors[b].Capability })
	remaining, err := merge.DeepCopyMap(capabilities)
	if err != nil {
		return nil, err
	}
	result := []Binding{}
	for _, descriptor := range descriptors {
		exports, exists, err := takeExports(remaining, descriptor.Path)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		bindings, err := descriptor.bindings(directory, exports, validate)
		if err != nil {
			return nil, err
		}
		result = append(result, bindings...)
	}
	if !emptyMap(remaining) {
		return nil, errUtils.Build(errUtils.ErrPluginUnsupportedExtension).
			WithExplanationf("plugin.yaml declares capabilities this Atmos does not support: %s.", strings.Join(slices.Sorted(maps.Keys(remaining)), ", ")).
			WithHint("Upgrade Atmos to a release that supports them, or remove them from `spec.capabilities`.").Err()
	}
	if len(result) == 0 {
		return nil, errUtils.Build(errUtils.ErrPluginInvalidConfig).
			WithExplanation("plugin.yaml declares no exports under `spec.capabilities`.").
			WithHint("Declare at least one export, for example `capabilities.commands.<name>`.").Err()
	}
	return result, nil
}

func takeExports(input map[string]any, path []string) (map[string]any, bool, error) {
	current := input
	for index, segment := range path {
		value, exists := current[segment]
		if !exists {
			return nil, false, nil
		}
		nested, ok := value.(map[string]any)
		if !ok {
			return nil, false, errUtils.Build(errUtils.ErrPluginInvalidConfig).
				WithExplanationf("`capabilities.%s` must be a mapping of export names.", strings.Join(path[:index+1], ".")).
				WithHint("Declare exports as named mappings under that key.").Err()
		}
		if index == len(path)-1 {
			delete(current, segment)
			return nested, true, nil
		}
		current = nested
	}
	return nil, false, nil
}

func emptyMap(input map[string]any) bool {
	for _, value := range input {
		nested, ok := value.(map[string]any)
		if !ok || !emptyMap(nested) {
			return false
		}
	}
	return true
}

func (d *Descriptor) bindings(directory string, exports map[string]any, validate bool) ([]Binding, error) {
	result := []Binding{}
	names := make([]string, 0, len(exports))
	for name := range exports {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		metadata, ok := exports[name].(map[string]any)
		if !ok || name == "" {
			return nil, errUtils.Build(errUtils.ErrPluginInvalidConfig).
				WithExplanationf("The %s export %q must be a non-empty name mapped to its settings.", d.Capability, name).
				WithHint("Write each export as `<name>: {…}`, using `{}` when it has no settings.").Err()
		}
		binding := Binding{Capability: d.Capability, Name: name, Directory: directory, Metadata: metadata}
		if validate {
			if err := d.Validate(binding); err != nil {
				return nil, fmt.Errorf("%w: %s %s: %w", errUtils.ErrPluginInvalidConfig, d.Capability, name, err)
			}
		}
		result = append(result, binding)
	}
	return result, nil
}

func validateDescriptor(descriptor *Descriptor) error {
	if descriptor.Capability == "" || len(descriptor.Path) == 0 || descriptor.Version == 0 || descriptor.Validate == nil {
		return errUtils.ErrPluginInvalidConfig
	}
	for _, part := range descriptor.Path {
		if part == "" || strings.Contains(part, ".") {
			return errUtils.ErrPluginInvalidConfig
		}
	}
	return nil
}
