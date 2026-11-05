package guard

import (
	"context"
	"slices"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// PluginInput contains non-secret plugin provenance and statically inspected metadata.
// Source checks precede remote access; package checks precede installation or activation.
// Empty metadata in the source phase is unknown, never an assertion of compatibility.
type PluginInput struct {
	Operation    string   `json:"operation"`
	Phase        string   `json:"phase"`
	Origin       string   `json:"origin"`
	Source       string   `json:"source"`
	Registry     string   `json:"registry"`
	Repository   string   `json:"repository"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Digest       string   `json:"digest"`
	Runtime      string   `json:"runtime"`
	Capabilities []string `json:"capabilities"`
	Development  bool     `json:"development"`
	// Path is the plugin directory of a local build (origin development): a forward-slash path
	// relative to the project directory when it lies inside it, otherwise absolute. It never contains
	// `..`, so a relative allow-root cannot match a directory outside the project. Empty for every
	// other origin and for a build whose directory is unknown.
	Path string `json:"path"`
	// Profiles lists the active profile names exactly as typed, without duplicates, in activation
	// order; it is an empty list when none is active. Names are chosen by the developer and a rule
	// keyed on one can be avoided by renaming the profile, so gate trust on origin and path instead.
	Profiles []string `json:"profiles"`
}

var activeProfiles struct {
	sync.RWMutex
	names []string
}

// SetActiveProfiles records the profiles the loaded configuration applied, for plugin policy input.
// Blank names are dropped and duplicates keep their first position. Passing nil clears the list.
func SetActiveProfiles(names []string) {
	defer perf.Track(nil, "guard.SetActiveProfiles")()

	cleaned := make([]string, 0, len(names))
	for _, name := range names {
		if name != "" && !slices.Contains(cleaned, name) {
			cleaned = append(cleaned, name)
		}
	}
	activeProfiles.Lock()
	defer activeProfiles.Unlock()
	activeProfiles.names = cleaned
}

// ActiveProfiles returns a copy of the recorded profile names; it is never nil.
func ActiveProfiles() []string {
	defer perf.Track(nil, "guard.ActiveProfiles")()

	activeProfiles.RLock()
	defer activeProfiles.RUnlock()
	return append([]string{}, activeProfiles.names...)
}

// Plugin admits a plugin operation through the same evaluator as other capabilities.
func Plugin(ctx context.Context, input *PluginInput) error {
	defer perf.Track(nil, "guard.Plugin")()

	if input.Profiles == nil {
		input.Profiles = ActiveProfiles()
	}

	return evaluate(ctx, "plugin", input)
}
