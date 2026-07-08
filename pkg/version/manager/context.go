package manager

import (
	"os"
	"regexp"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// versionRefPattern detects template references to the .version context. The
// word boundary prevents false positives on identifiers that merely start
// with "version", such as `{{ .vars.versioning_enabled }}`.
var versionRefPattern = regexp.MustCompile(`\.version\b`)

// RenderFunc renders template content with the given data. The command layer
// injects the Atmos template engine so this package never depends on
// internal/exec.
type RenderFunc func(atmosConfig *schema.AtmosConfiguration, name, content string, data map[string]any) (string, error)

// AddTemplateContext lazily injects the .version template context when the
// YAML content references it. The version map is loaded from the lock file for
// the given track. An existing "version" key in the context is left untouched.
//
// It never turns an empty context into a non-empty one: a non-empty context is
// what gates whole-file template processing in the stack processor, and
// enabling that early would evaluate templates (e.g. `{{ .vars.* }}` inside
// generate sections) that are meant for later processing stages. The .version
// context therefore piggybacks only on template processing that is already
// going to happen.
func AddTemplateContext(
	atmosConfig *schema.AtmosConfiguration,
	yamlContent string,
	context map[string]any,
	track string,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "manager.AddTemplateContext")()

	if len(context) == 0 {
		return context, nil
	}
	if !versionRefPattern.MatchString(yamlContent) {
		return context, nil
	}
	if _, exists := context["version"]; exists {
		return context, nil
	}
	versionMap, err := VersionMap(atmosConfig, track)
	if err != nil {
		return nil, err
	}
	context["version"] = versionMap
	return context, nil
}

// ResolveYAMLFunc resolves a `!version name` YAML function argument against the
// lock file, using the stack-asserted track when present.
func ResolveYAMLFunc(atmosConfig *schema.AtmosConfiguration, name string, stackInfo *schema.ConfigAndStacksInfo) (string, error) {
	defer perf.Track(atmosConfig, "manager.ResolveYAMLFunc")()

	track := EffectiveTrackFromStack(atmosConfig, stackInfo)
	return ResolveLocked(atmosConfig, track, name)
}

// RenderFile renders a template file with the .version context for a track.
// It returns the rendered content; writing (or check-mode comparison) is the
// caller's concern.
func RenderFile(atmosConfig *schema.AtmosConfiguration, track, file string, render RenderFunc) (string, error) {
	defer perf.Track(atmosConfig, "manager.RenderFile")()

	content, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	versionMap, err := VersionMap(atmosConfig, track)
	if err != nil {
		return "", err
	}
	return render(atmosConfig, file, string(content), map[string]any{"version": versionMap})
}
