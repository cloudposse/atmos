package report

import (
	"embed"
	"io/fs"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// The componentType constant is the ci.templates key of artifact summaries.
	componentType = "artifacts"

	// The frameCommand constant names the frame template: ci.templates.artifacts.summary.
	frameCommand = "summary"

	// The templatesPrefix constant is the directory the template loader reads embedded defaults from.
	templatesPrefix = "templates/"

	// The templateSuffix constant is the extension of a template file.
	templateSuffix = ".md"

	// The frameFile and operationFile constants are the embedded default files.
	frameFile     = "templates/summary.md"
	operationFile = "templates/operation.md"
)

//go:embed templates/summary.md templates/operation.md
var defaultFiles embed.FS

// defaultTemplates serves the embedded defaults under the names the template loader asks for:
// templates/summary.md is the frame, and templates/<action>.md is the shared per-operation
// section for every known action. A custom template can replace the frame, or the section of one
// action, without touching the others.
type defaultTemplates struct{}

// Open implements fs.FS.
func (defaultTemplates) Open(name string) (fs.File, error) {
	defer perf.Track(nil, "report.defaultTemplates.Open")()

	command, found := strings.CutPrefix(name, templatesPrefix)
	command, hasSuffix := strings.CutSuffix(command, templateSuffix)
	if !found || !hasSuffix {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	switch {
	case command == frameCommand:
		return defaultFiles.Open(frameFile)
	case isKnownAction(command):
		return defaultFiles.Open(operationFile)
	default:
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
}

var _ fs.FS = defaultTemplates{}
