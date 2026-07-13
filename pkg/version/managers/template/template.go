// Package template implements the template file manager: *.tmpl files are the
// human-edited source of truth and render to a sibling file with the .tmpl
// suffix stripped, using the .version context resolved from the lock. This
// covers comment-hostile formats (JSON) and any file where an explicit
// template beats in-place rewriting.
package template

import (
	"bytes"
	"context"
	"os"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/managers"
)

// Name is the manager's registry name.
const Name = "template"

// templateSuffix marks template sources; the rendered sibling drops it.
const templateSuffix = ".tmpl"

// Manager renders version templates to their sibling output files.
type Manager struct{}

// Name returns the manager's registry name.
func (Manager) Name() string {
	defer perf.Track(nil, "template.Manager.Name")()

	return Name
}

// DefaultPaths matches template files anywhere in the project tree.
func (Manager) DefaultPaths() []string {
	defer perf.Track(nil, "template.Manager.DefaultPaths")()

	return []string{"**/*" + templateSuffix}
}

// Plan renders each template with the .version context and returns the
// sibling files whose content would change.
func (m Manager) Plan(ctx context.Context, in *managers.Input) ([]managers.FileChange, error) {
	defer perf.Track(in.Config, "template.Manager.Plan")()

	if in.Render == nil {
		return nil, nil
	}
	paths := in.Paths
	if len(paths) == 0 {
		paths = m.DefaultPaths()
	}
	files, err := managers.ExpandPaths(in.Dir, paths)
	if err != nil {
		return nil, err
	}
	templateData := map[string]any{"version": in.Refs}
	var changes []managers.FileChange
	for _, file := range files {
		if !strings.HasSuffix(file, templateSuffix) {
			continue
		}
		change, changed, err := planFile(in, file, templateData)
		if err != nil {
			return nil, err
		}
		if changed {
			changes = append(changes, change)
		}
	}
	return changes, nil
}

// planFile renders one template and compares it to its sibling output file.
func planFile(in *managers.Input, file string, templateData map[string]any) (managers.FileChange, bool, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return managers.FileChange{}, false, err
	}
	rendered, err := in.Render(in.Config, file, string(content), templateData)
	if err != nil {
		return managers.FileChange{}, false, err
	}
	outputPath := strings.TrimSuffix(file, templateSuffix)
	current, err := os.ReadFile(outputPath)
	if err != nil && !os.IsNotExist(err) {
		return managers.FileChange{}, false, err
	}
	if err == nil && bytes.Equal(current, []byte(rendered)) {
		return managers.FileChange{}, false, nil
	}
	return managers.FileChange{Path: outputPath, Old: current, New: []byte(rendered)}, true, nil
}

func init() {
	managers.Register(Manager{})
}
