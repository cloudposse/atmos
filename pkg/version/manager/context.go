package manager

import (
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// RenderFunc renders template content with the given data. The command layer
// injects the Atmos template engine so this package never depends on
// internal/exec.
type RenderFunc func(atmosConfig *schema.AtmosConfiguration, name, content string, data map[string]any) (string, error)

// AddTemplateContext lazily injects the .version template context when the
// YAML content references it. The version map is loaded from the lock file for
// the given track. An existing "version" key in the context is left untouched.
func AddTemplateContext(
	atmosConfig *schema.AtmosConfiguration,
	yamlContent string,
	context map[string]any,
	track string,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "manager.AddTemplateContext")()

	if !strings.Contains(yamlContent, ".version") {
		return context, nil
	}
	if context == nil {
		context = map[string]any{}
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
