package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// terraformSourceSuffixes name the Terraform and OpenTofu files of a module. Terragrunt lists the files of a
// local module source among the files it read, but it never evaluates them as Terragrunt configuration.
var terraformSourceSuffixes = []string{".tf", ".tf.json", ".tofu", ".tofu.json"}

// moduleSourceFile reports whether file is a Terraform or OpenTofu source file rather than Terragrunt
// configuration, so it declares no sensitive input. The unit itself is always configuration.
func moduleSourceFile(file, unit string) bool {
	if unit != "" && filepath.Clean(file) == filepath.Clean(unit) {
		return false
	}
	name := strings.ToLower(filepath.Base(file))
	return slices.ContainsFunc(terraformSourceSuffixes, func(suffix string) bool { return strings.HasSuffix(name, suffix) })
}

// readTarget finds the file a read_terragrunt_config call reads. Terragrunt evaluates the functions of an
// included file from the unit that includes it, so a parent-folder search in another file starts at the
// unit, and falls back to the file itself.
func (s *sensitivityScanner) readTarget(path string, parentSearch bool) (*hclsyntax.Body, string) {
	if parentSearch && s.unit != "" && s.unit != "." && s.unit != filepath.Clean(s.file) {
		if target, targetFile := readFile(path, parentSearch, s.unit, s.bodies); target != nil {
			return target, targetFile
		}
	}
	return readFile(path, parentSearch, s.file, s.bodies)
}

// errNotRead marks an existing file the evaluation never opened. Terragrunt lists the files of a local
// module source, and the targets of mark_as_read, among the files it read without opening them, so
// they hold no configuration it evaluated. It reads as missing to every check except for the unit itself.
var errNotRead = fmt.Errorf("the file was not opened while the unit was evaluated: %w", fs.ErrNotExist)

// notEvaluated reports whether a read error leaves the file's evaluated contents unknown. A file that was
// never opened declares nothing, except the unit: its evaluation always opens it.
func notEvaluated(err error, file, unit string) bool {
	return errors.Is(err, errNotEvaluated) || (unit != "" && file == unit && errors.Is(err, errNotRead))
}
