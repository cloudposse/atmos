package create

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/vendor"
)

// embeddedSource is the source recorded for scaffolds built into the binary.
const embeddedSource = "embedded"

// rejectRefForNonGit fails when --ref is given for a scaffold that is not fetched from Git. Embedded, local
// and configured-local scaffolds have no revision to select, so the flag would otherwise be silently ignored.
func rejectRefForNonGit(template *templates.Configuration, ref string) error {
	if ref == "" || vendor.IsGitURI(template.Source) {
		return nil
	}
	return errUtils.Build(fmt.Errorf("%w: --ref=%s selects a Git revision, but template %q %s", errUtils.ErrInvalidFlag, ref, template.Name, describeSource(template.Source))).
		WithHint("Remove `--ref`, or point `--template` (or `scaffold.templates` in `atmos.yaml`) at a Git repository").
		Err()
}

func describeSource(source string) string {
	if source == "" || source == embeddedSource {
		return "is built into Atmos"
	}
	return fmt.Sprintf("is not a Git source (%s)", source)
}
