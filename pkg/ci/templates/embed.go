package templates

import (
	"embed"
	"io/fs"

	"github.com/cloudposse/atmos/pkg/perf"
)

// containerDefaultFile is the embedded default template for container image summaries.
const containerDefaultFile = "container_default.md"

// containerImageTemplatePath is the path Loader.Load reads for the "image" command.
const containerImageTemplatePath = "templates/image.md"

//go:embed container_default.md
var containerDefaults embed.FS

// containerFS exposes the embedded container default under the path the Loader expects
// ("templates/<command>.md"), so the container component participates in the same
// override chain as the plugin-provided templates without a nested templates directory.
type containerFS struct{}

// Open implements fs.FS.
func (containerFS) Open(name string) (fs.File, error) {
	defer perf.Track(nil, "templates.containerFS.Open")()

	if name == containerImageTemplatePath {
		return containerDefaults.Open(containerDefaultFile)
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// ContainerDefaults returns the embedded default templates for the container component.
// Pass the result to Loader.LoadAndRender with component type "container" and command "image".
func ContainerDefaults() fs.FS {
	defer perf.Track(nil, "templates.ContainerDefaults")()

	return containerFS{}
}
