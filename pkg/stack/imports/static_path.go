package imports

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/delimiters"
)

// ValidateStaticImportPath rejects dynamic paths before optional-import handling
// can omit them. Invalid delimiter settings cannot establish that a path is static.
func ValidateStaticImportPath(atmosConfig *schema.AtmosConfiguration, path, manifest string) error {
	defer perf.Track(atmosConfig, "imports.ValidateStaticImportPath")()

	var configured []string
	if atmosConfig != nil {
		configured = atmosConfig.Templates.Settings.Delimiters
	}
	delims, err := delimiters.Parse(configured)
	if err != nil {
		return fmt.Errorf("cannot inspect import %q in %q: %w", path, manifest, err)
	}
	if delims.Contains(path) {
		return fmt.Errorf("%w: cannot inspect templated import %q in %q without evaluating dynamic expressions", errUtils.ErrInvalidImport, path, manifest)
	}
	return nil
}
