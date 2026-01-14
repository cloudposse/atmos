package function

import (
	"sync"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

var registerOnce sync.Once

// RegisterDefaults registers all default function handlers with the global registry.
// This is called automatically when DefaultRegistry() is first accessed,
// but can also be called explicitly to ensure functions are registered.
func RegisterDefaults() {
	defer perf.Track(nil, "function.RegisterDefaults")()

	registerOnce.Do(func() {
		registry := DefaultRegistry()

		// PreMerge functions.
		mustRegister(registry, NewEnvFunction())
		mustRegister(registry, NewExecFunction())
		mustRegister(registry, NewRandomFunction())
		mustRegister(registry, NewTemplateFunction())
		mustRegister(registry, NewGitRootFunction())
		mustRegister(registry, NewIncludeFunction())
		mustRegister(registry, NewIncludeRawFunction())
		mustRegister(registry, NewLiteralFunction())

		// PostMerge functions.
		mustRegister(registry, NewStoreFunction())
		mustRegister(registry, NewStoreGetFunction())
		mustRegister(registry, NewTerraformOutputFunction())
		mustRegister(registry, NewTerraformStateFunction())
		mustRegister(registry, NewAwsAccountIDFunction())
		mustRegister(registry, NewAwsCallerIdentityArnFunction())
		mustRegister(registry, NewAwsCallerIdentityUserIDFunction())
		mustRegister(registry, NewAwsRegionFunction())
	})
}

// mustRegister registers a function and panics on error.
func mustRegister(registry *Registry, fn Function) {
	if err := registry.Register(fn); err != nil {
		log.Error("Failed to register function", "name", fn.Name(), "error", err)
		panic(err)
	}
}

// init automatically registers defaults when the package is imported.
func init() {
	RegisterDefaults()
}
