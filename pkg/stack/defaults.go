package stack

import (
	"github.com/cloudposse/atmos/pkg/function"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/stack/loader"
	"github.com/cloudposse/atmos/pkg/stack/loader/hcl"
	"github.com/cloudposse/atmos/pkg/stack/loader/json"
	"github.com/cloudposse/atmos/pkg/stack/loader/yaml"
	"github.com/cloudposse/atmos/pkg/stack/processor"
)

// DefaultLoaderRegistry creates a new loader registry with all default loaders registered.
// This includes YAML, JSON, and HCL loaders.
func DefaultLoaderRegistry() *loader.Registry {
	defer perf.Track(nil, "stack.DefaultLoaderRegistry")()

	r := loader.NewRegistry()

	// Register all default loaders.
	_ = r.Register(yaml.New())
	_ = r.Register(json.New())
	_ = r.Register(hcl.New())

	return r
}

// DefaultProcessor creates a new Processor with default function and loader registries.
// The shellExecutor parameter provides the implementation for the !exec function.
// If shellExecutor is nil, the !exec function will not be available.
func DefaultProcessor(shellExecutor function.ShellExecutor) *processor.Processor {
	defer perf.Track(nil, "stack.DefaultProcessor")()

	return processor.New(
		function.DefaultRegistry(shellExecutor),
		DefaultLoaderRegistry(),
	)
}

// IsExtensionSupported returns true if the given extension is supported.
func IsExtensionSupported(ext string) bool {
	defer perf.Track(nil, "stack.IsExtensionSupported")()

	r := DefaultLoaderRegistry()
	return r.HasExtension(ext)
}
