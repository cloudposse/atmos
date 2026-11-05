package deferred

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TargetSection returns the unevaluated target configuration for deferred authentication.
// Only raw describes (component templates and YAML functions disabled) may be
// supplied. Each caller receives its own copy before inherited authentication is
// applied. Stack locals can still recurse during loading, so misses must not wait
// on other in-flight describes: the existing lookup cycle detector owns recursion.
func TargetSection(ac *schema.AtmosConfiguration, component, stack string, describe func() (map[string]any, error)) (map[string]any, error) {
	defer perf.Track(ac, "stack.deferred.TargetSection")()

	if ac == nil || !authdeferred.IsDeferred(ac.AuthManager) {
		return describe()
	}
	key, err := targetSectionKey(ac, component, stack)
	if err != nil {
		return describe()
	}
	context := evaluationContext(ac)
	if cached, ok := context.TargetSections.Load(key); ok {
		return merge.DeepCopyMap(cached.(map[string]any))
	}
	section, err := describe()
	if err != nil || section == nil {
		return nil, err
	}
	if !deferred.IsCacheable(section) {
		return section, nil
	}
	// Own the stored map as well as each returned map. Neither the describer
	// nor a caller may change another reference's effective configuration.
	cached, err := merge.DeepCopyMap(section)
	if err != nil {
		return nil, err
	}
	context.TargetSections.LoadOrStore(key, cached)
	return merge.DeepCopyMap(cached)
}

func targetSectionKey(ac *schema.AtmosConfiguration, component, stack string) (string, error) {
	encoded, err := json.Marshal(struct {
		BasePath, ConfigPath, Component, Stack string
		Stacks                                 schema.Stacks
		Auth                                   schema.AuthConfig
	}{ac.BasePath, ac.CliConfigPath, component, stack, ac.Stacks, ac.Auth})
	if err != nil {
		return "", err
	}
	// Authentication configuration may contain credentials; retain only a digest.
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
