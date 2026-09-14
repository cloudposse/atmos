package config

import (
	"os"
	"slices"
	"time"

	"go.yaml.in/yaml/v3"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ClaimExperimentalWarning records a feature's warning before it is printed.
// Each feature is eligible once every 24 hours across invocations sharing the
// cache. Cache failures allow the warning through without failing the command.
func ClaimExperimentalWarning(feature string) bool {
	defer perf.Track(nil, "config.ClaimExperimentalWarning")()

	return claimExperimentalWarningAt(feature, time.Now())
}

func claimExperimentalWarningAt(feature string, now time.Time) bool {
	show := false
	err := UpdateCache(func(cache *CacheConfig) {
		index := experimentalWarningIndex(cache.ExperimentalWarnings, feature)
		if index >= 0 {
			last := cache.ExperimentalWarnings[index].LastShown
			if now.Unix()-last < int64((24*time.Hour)/time.Second) && now.Unix() >= last {
				return
			}
			cache.ExperimentalWarnings[index].LastShown = now.Unix()
		} else {
			cache.ExperimentalWarnings = append(cache.ExperimentalWarnings, ExperimentalWarningState{
				Feature: feature, LastShown: now.Unix(),
			})
		}
		show = true
	})
	if err != nil {
		log.Trace("Unable to cache experimental warning", "error", err)
		return true
	}
	return show
}

// mergeExperimentalWarnings runs under SaveCache's exclusive lock. Other
// fields retain SaveCache's replacement semantics, including corruption repair.
func mergeExperimentalWarnings(path string, cfg *CacheConfig) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var persisted CacheConfig
	if yaml.Unmarshal(raw, &persisted) != nil || len(persisted.ExperimentalWarnings) == 0 {
		return
	}
	// Do not mutate the caller's slice while merging a stale snapshot.
	cfg.ExperimentalWarnings = slices.Clone(cfg.ExperimentalWarnings)
	for _, warning := range persisted.ExperimentalWarnings {
		index := experimentalWarningIndex(cfg.ExperimentalWarnings, warning.Feature)
		if index < 0 {
			cfg.ExperimentalWarnings = append(cfg.ExperimentalWarnings, warning)
		} else if warning.LastShown > cfg.ExperimentalWarnings[index].LastShown {
			cfg.ExperimentalWarnings[index] = warning
		}
	}
}

func experimentalWarningIndex(warnings []ExperimentalWarningState, feature string) int {
	return slices.IndexFunc(warnings, func(warning ExperimentalWarningState) bool {
		return warning.Feature == feature
	})
}
