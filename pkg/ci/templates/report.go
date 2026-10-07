package templates

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// reportExtension is appended to a template name that has no file with that exact name.
const reportExtension = ".md"

// RenderReport renders a report template for script-authored summaries and comments.
// The name is a file name under ci.templates.base_path (or an absolute path), and data is the
// template context. When no file has exactly that name, the name with a .md suffix is tried.
func RenderReport(atmosConfig *schema.AtmosConfiguration, name string, data any) (string, error) {
	defer perf.Track(atmosConfig, "templates.RenderReport")()

	loader := NewLoader(atmosConfig)
	path := loader.resolvePath(name)

	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && filepath.Ext(path) != reportExtension {
		path += reportExtension
		content, err = os.ReadFile(path)
	}
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", errUtils.Build(errUtils.ErrCITemplateNotFound).
				WithCause(err).
				WithExplanation(fmt.Sprintf("Report template %q does not exist", name)).
				WithContext("template", name).
				WithHint("Place the file under ci.templates.base_path or pass an absolute path").
				Err()
		}
		return "", fmt.Errorf("%w: %s: %w", errUtils.ErrReadFile, path, err)
	}

	rendered, err := loader.Render(string(content), data)
	if err != nil {
		return "", fmt.Errorf("report template %q: %w", name, err)
	}
	return rendered, nil
}
