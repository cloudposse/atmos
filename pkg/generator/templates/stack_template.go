package templates

import (
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

// scaffoldManifestFile is the template file holding a scaffold's metadata and questionnaire.
const scaffoldManifestFile = "scaffold.yaml"

// IsStackTemplate reports whether the template declares `spec.stack` in its scaffold.yaml. Such a
// template generates a single stack manifest for `atmos stack create`, not a project, so it is
// not offered as a project template by `atmos init`. It stays available to `atmos scaffold list`,
// `atmos scaffold generate`, and `atmos stack create`.
func (c *Configuration) IsStackTemplate() bool {
	defer perf.Track(nil, "templates.Configuration.IsStackTemplate")()

	for i := range c.Files {
		if c.Files[i].Path != scaffoldManifestFile || c.Files[i].IsDirectory {
			continue
		}
		var manifest struct {
			Spec struct {
				Stack *yaml.Node `yaml:"stack"`
			} `yaml:"spec"`
		}
		if err := yaml.Unmarshal([]byte(c.Files[i].Content), &manifest); err != nil {
			return false
		}
		return manifest.Spec.Stack != nil
	}
	return false
}

// WithoutStackTemplates returns a copy of configs omitting every template that declares
// `spec.stack` (see IsStackTemplate). The input map is not modified.
func WithoutStackTemplates(configs map[string]Configuration) map[string]Configuration {
	defer perf.Track(nil, "templates.WithoutStackTemplates")()

	filtered := make(map[string]Configuration, len(configs))
	for name := range configs {
		config := configs[name]
		if config.IsStackTemplate() {
			continue
		}
		filtered[name] = config
	}
	return filtered
}
