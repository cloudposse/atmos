package planfile

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// S3StoreType is the artifact store type for AWS S3.
	S3StoreType = "aws/s3"

	// GitHubStoreType is the artifact store type for GitHub Actions artifacts.
	GitHubStoreType = "github/artifacts"

	// LocalStoreType is the artifact store type for the local filesystem.
	LocalStoreType = "local/dir"

	// DefaultLocalPath is the directory used by the built-in local fallback store.
	DefaultLocalPath = ".atmos/planfiles"

	// DefaultGitHubPrefix is the artifact name prefix used by the environment-detected
	// GitHub Actions store.
	DefaultGitHubPrefix = "planfile"
)

// ambiguousStoreWarning keeps the "several stores, nothing selects one" warning to
// one emission per process, so a run over many components does not repeat it.
var ambiguousStoreWarning sync.Once

// StoreSource identifies where a resolved store candidate came from. It exists so
// callers (and log output) can explain *why* a particular backend was selected,
// which is the part users cannot infer from the store type alone.
type StoreSource string

const (
	// StoreSourceExplicit means the store was named on the command line (`--store`).
	StoreSourceExplicit StoreSource = "explicit"

	// StoreSourceDefault means the store came from `components.terraform.planfiles.default`.
	StoreSourceDefault StoreSource = "default"

	// StoreSourcePriority means the store came from `components.terraform.planfiles.priority`.
	StoreSourcePriority StoreSource = "priority"

	// StoreSourceOnlyStore means `components.terraform.planfiles.stores` defines exactly
	// one store and neither `default` nor `priority` selects one, so that store is used.
	StoreSourceOnlyStore StoreSource = "only-store"

	// StoreSourceEnvironment means the store was detected from environment variables.
	StoreSourceEnvironment StoreSource = "environment"

	// StoreSourceFallback means the built-in local store is used because nothing else matched.
	StoreSourceFallback StoreSource = "fallback"
)

// StoreCandidate is one planfile store that Atmos may use, along with where the
// selection came from.
type StoreCandidate struct {
	// Name is the configured store name (a key of `planfiles.stores`), the store
	// type when a type was named directly, or empty for environment-detected and
	// built-in fallback stores.
	Name string

	// Source records which setting produced this candidate.
	Source StoreSource

	// Options are the artifact store options used to construct the backend.
	Options StoreOptions
}

// Description returns a short human-readable identifier for log and error messages.
func (c StoreCandidate) Description() string {
	if c.Name != "" && c.Name != c.Options.Type {
		return fmt.Sprintf("%s (%s)", c.Name, c.Options.Type)
	}
	return c.Options.Type
}

// ResolveStoreCandidates returns the ordered list of planfile stores to try.
//
// Resolution order:
//
//  1. An explicitly requested store (`--store`), which overrides configuration.
//  2. `components.terraform.planfiles.default`, when set.
//  3. `components.terraform.planfiles.priority`, in the listed order.
//  4. The single entry of `components.terraform.planfiles.stores`, when exactly one
//     is defined and neither `default` nor `priority` selects one.
//  5. Environment detection (`ATMOS_PLANFILE_BUCKET`, then GitHub Actions).
//  6. The built-in local store.
//
// Configuration wins outright: when `default`, `priority`, or a single named store
// selects a backend, Atmos never silently falls back to environment detection —
// that fallback is what made an explicitly configured S3 store get replaced by
// GitHub Artifacts under GitHub Actions.
func ResolveStoreCandidates(atmosConfig *schema.AtmosConfiguration, storeName string) ([]StoreCandidate, error) {
	defer perf.Track(atmosConfig, "planfile.ResolveStoreCandidates")()

	var planfilesConfig schema.PlanfilesConfig
	if atmosConfig != nil {
		planfilesConfig = atmosConfig.Components.Terraform.Planfiles
	}

	// An explicit `--store` overrides everything in configuration.
	if storeName != "" {
		candidate, err := candidateFromName(storeName, planfilesConfig, StoreSourceExplicit, "--store")
		if err != nil {
			return nil, err
		}
		return withAtmosConfig([]StoreCandidate{candidate}, atmosConfig), nil
	}

	configured, err := configuredCandidates(planfilesConfig)
	if err != nil {
		return nil, err
	}
	if len(configured) > 0 {
		return withAtmosConfig(configured, atmosConfig), nil
	}

	// Nothing in configuration selects a store: detect from the environment and
	// fall back to local storage.
	candidates := environmentCandidates()
	candidates = append(candidates, localFallbackCandidate())
	return withAtmosConfig(candidates, atmosConfig), nil
}

// configuredCandidates resolves the candidates selected by configuration.
// Returns an empty slice when configuration does not select any store.
func configuredCandidates(planfilesConfig schema.PlanfilesConfig) ([]StoreCandidate, error) {
	// `default` names a single store, so it takes precedence over the priority list.
	if planfilesConfig.Default != "" {
		if len(planfilesConfig.Priority) > 0 {
			log.Debug("Both planfiles.default and planfiles.priority are set; default takes precedence",
				"default", planfilesConfig.Default, "priority", planfilesConfig.Priority)
		}
		candidate, err := candidateFromName(planfilesConfig.Default, planfilesConfig, StoreSourceDefault,
			"components.terraform.planfiles.default")
		if err != nil {
			return nil, err
		}
		return []StoreCandidate{candidate}, nil
	}

	if len(planfilesConfig.Priority) > 0 {
		candidates := make([]StoreCandidate, 0, len(planfilesConfig.Priority))
		for i, name := range planfilesConfig.Priority {
			setting := fmt.Sprintf("components.terraform.planfiles.priority[%d]", i)
			candidate, err := candidateFromName(name, planfilesConfig, StoreSourcePriority, setting)
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, candidate)
		}
		return candidates, nil
	}

	// A single named store with no explicit selection is unambiguous: use it.
	if len(planfilesConfig.Stores) == 1 {
		for name := range planfilesConfig.Stores {
			candidate, err := candidateFromName(name, planfilesConfig, StoreSourceOnlyStore,
				"components.terraform.planfiles.stores")
			if err != nil {
				return nil, err
			}
			return []StoreCandidate{candidate}, nil
		}
	}

	// Several stores are defined but nothing picks one. Environment detection still
	// applies (that is the historical behavior), but it is worth saying out loud,
	// because the detected store is very likely not the one the user meant. Store
	// resolution runs once per component per hook, so the warning is emitted once
	// per process rather than once per resolution.
	if len(planfilesConfig.Stores) > 1 {
		ambiguousStoreWarning.Do(func() {
			log.Warn("Multiple planfile stores are defined but neither planfiles.default nor planfiles.priority selects one; falling back to environment detection",
				"stores", storeNames(planfilesConfig))
		})
	}

	return nil, nil
}

// candidateFromName resolves a store reference to a candidate. The reference is
// either a key of `planfiles.stores` or a store type (e.g. `aws/s3`), matching what
// `--store` has always accepted. Anything else is a configuration error: silently
// skipping an unresolvable name is how a configured store ends up being ignored.
func candidateFromName(name string, planfilesConfig schema.PlanfilesConfig, source StoreSource, setting string) (StoreCandidate, error) {
	if spec, ok := planfilesConfig.Stores[name]; ok {
		if spec.Type == "" {
			return StoreCandidate{}, fmt.Errorf("%w: store %q (referenced by %s) has no `type`; set one of %s, %s, or %s",
				errUtils.ErrPlanfileStoreInvalidArgs, name, setting, S3StoreType, GitHubStoreType, LocalStoreType)
		}
		// Clone the options so a backend (or a caller's decorator) cannot write
		// through into the loaded Atmos configuration, which is shared across every
		// component in a run.
		return StoreCandidate{
			Name:    name,
			Source:  source,
			Options: StoreOptions{Type: spec.Type, Options: maps.Clone(spec.Options)},
		}, nil
	}

	// Store types are always `<vendor>/<kind>`, while named stores are plain
	// identifiers, so the separator is what distinguishes the two.
	if strings.Contains(name, "/") {
		return StoreCandidate{
			Name:    name,
			Source:  source,
			Options: StoreOptions{Type: name, Options: map[string]any{}},
		}, nil
	}

	defined := storeNames(planfilesConfig)
	definedText := "none are defined"
	if len(defined) > 0 {
		definedText = fmt.Sprintf("defined stores: %s", strings.Join(defined, ", "))
	}

	return StoreCandidate{}, fmt.Errorf("%w: %s references %q, which is not defined in components.terraform.planfiles.stores (%s) and is not a store type such as %s, %s, or %s",
		errUtils.ErrPlanfileStoreNotFound, setting, name, definedText, S3StoreType, GitHubStoreType, LocalStoreType)
}

// environmentCandidates returns store candidates detected from environment variables,
// in precedence order: S3 configuration first, then GitHub Actions.
func environmentCandidates() []StoreCandidate {
	var candidates []StoreCandidate

	if bucket := os.Getenv("ATMOS_PLANFILE_BUCKET"); bucket != "" {
		candidates = append(candidates, StoreCandidate{
			Source: StoreSourceEnvironment,
			Options: StoreOptions{
				Type: S3StoreType,
				Options: map[string]any{
					"bucket": bucket,
					"prefix": os.Getenv("ATMOS_PLANFILE_PREFIX"),
					"region": os.Getenv("AWS_REGION"),
				},
			},
		})
	}

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		candidates = append(candidates, StoreCandidate{
			Source: StoreSourceEnvironment,
			Options: StoreOptions{
				Type: GitHubStoreType,
				Options: map[string]any{
					"prefix": DefaultGitHubPrefix,
				},
			},
		})
	}

	return candidates
}

// localFallbackCandidate returns the built-in local store candidate.
func localFallbackCandidate() StoreCandidate {
	return StoreCandidate{
		Source: StoreSourceFallback,
		Options: StoreOptions{
			Type: LocalStoreType,
			Options: map[string]any{
				"path": DefaultLocalPath,
			},
		},
	}
}

// withAtmosConfig attaches the Atmos configuration to every candidate's options.
func withAtmosConfig(candidates []StoreCandidate, atmosConfig *schema.AtmosConfiguration) []StoreCandidate {
	for i := range candidates {
		candidates[i].Options.AtmosConfig = atmosConfig
	}
	return candidates
}

// storeNames returns the sorted names of the configured stores.
func storeNames(planfilesConfig schema.PlanfilesConfig) []string {
	names := make([]string, 0, len(planfilesConfig.Stores))
	for name := range planfilesConfig.Stores {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
