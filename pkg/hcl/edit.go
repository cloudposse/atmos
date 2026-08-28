package hcl

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/minamijoyo/hcledit/editor"

	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
)

// unnamedFile is passed to hcledit for content-based operations that have no
// real file backing. The filename is only used by hcledit to compose
// parse-error messages, never for I/O, so a placeholder is safe here;
// file-based operations pass the real path instead so errors name the real
// file.
const unnamedFile = "component.tf"

// defaultFileMode is the permission used for newly written files when the
// destination does not already exist.
const defaultFileMode os.FileMode = 0o644

// errWrapFmt is the format string for wrapping a sentinel error with an
// underlying error.
const errWrapFmt = "%w: %w"

// errNotFoundFmt is the format string for reporting a not-found address
// alongside its sentinel error.
const errNotFoundFmt = "%w: %s"

// Get reads the value at address, trying an attribute lookup first and
// falling back to a block lookup if the address doesn't resolve to an
// attribute. The withComments flag includes an attribute's inline trailing
// comment in the result; it has no effect on block results (a block's own
// comments are always part of its text). Returns ErrHCLAddressNotFound if
// address resolves to neither.
func Get(content []byte, address string, withComments bool) (string, error) {
	defer perf.Track(nil, "hcl.Get")()

	attrValue, err := deriveAttribute(content, address, withComments)
	if err != nil {
		return "", err
	}
	if attrValue != "" {
		return attrValue, nil
	}

	blockValue, err := getBlock(content, address)
	if err != nil {
		return "", err
	}
	if blockValue != "" {
		return blockValue, nil
	}

	return "", fmt.Errorf(errNotFoundFmt, ErrHCLAddressNotFound, address)
}

// deriveAttribute returns the trimmed value of the attribute at address, or
// "" if address does not resolve to an attribute.
func deriveAttribute(content []byte, address string, withComments bool) (string, error) {
	var buf bytes.Buffer
	sink := editor.NewAttributeGetSink(address, withComments)
	if err := editor.DeriveStream(bytes.NewReader(content), &buf, unnamedFile, sink); err != nil {
		return "", fmt.Errorf(errWrapFmt, ErrHCLParseFailed, err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

// getBlock returns the trimmed text of the block(s) matching address, or ""
// if none match. Unlike write operations, a read does not need the
// post-edit validity guard, since nothing is persisted.
func getBlock(content []byte, address string) (string, error) {
	var buf bytes.Buffer
	filter := editor.NewBlockGetFilter(address)
	if err := editor.EditStream(bytes.NewReader(content), &buf, unnamedFile, filter); err != nil {
		return "", fmt.Errorf(errWrapFmt, ErrHCLParseFailed, err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

// SetAttribute assigns value (a raw HCL expression, e.g. `"t3.micro"` or
// `3`) to the attribute at address, preserving comments and formatting.
func SetAttribute(content []byte, address, value string) ([]byte, error) {
	defer perf.Track(nil, "hcl.SetAttribute")()

	return applyFilter(content, unnamedFile, editor.NewAttributeSetFilter(address, value))
}

// AppendAttribute adds a new attribute at address with value (a raw HCL
// expression). Fails if the attribute already exists.
func AppendAttribute(content []byte, address, value string, newline bool) ([]byte, error) {
	defer perf.Track(nil, "hcl.AppendAttribute")()

	return applyFilter(content, unnamedFile, editor.NewAttributeAppendFilter(address, value, newline))
}

// RemoveAttribute removes the attribute at address.
func RemoveAttribute(content []byte, address string) ([]byte, error) {
	defer perf.Track(nil, "hcl.RemoveAttribute")()

	return applyFilter(content, unnamedFile, editor.NewAttributeRemoveFilter(address))
}

// NewBlock creates a new empty block at address (type plus labels, e.g.
// "resource.aws_instance.web").
func NewBlock(content []byte, address string, newline bool) ([]byte, error) {
	defer perf.Track(nil, "hcl.NewBlock")()

	return applyFilter(content, unnamedFile, editor.NewBlockNewFilter(address, newline))
}

// AppendBlock creates a new empty child block (type plus labels, relative to
// parent) inside every block matching parent's address.
func AppendBlock(content []byte, parent, child string, newline bool) ([]byte, error) {
	defer perf.Track(nil, "hcl.AppendBlock")()

	return applyFilter(content, unnamedFile, editor.NewBlockAppendFilter(parent, child, newline))
}

// RemoveBlock removes every block matching address.
func RemoveBlock(content []byte, address string) ([]byte, error) {
	defer perf.Track(nil, "hcl.RemoveBlock")()

	return applyFilter(content, unnamedFile, editor.NewBlockRemoveFilter(address))
}

// applyFilter runs filter via hcledit's EditStream, then guards the result:
// it must still parse as valid HCL, or the edit is rejected instead of ever
// being returned to a caller that might persist it. This is HCL's analog of
// pkg/yaml's anchor guard -- since hcledit edits via hclwrite's AST rather
// than a text diff a broken result should be structurally impossible, but
// the check is cheap insurance against ever writing a broken .tf file.
func applyFilter(content []byte, filename string, filter editor.Filter) ([]byte, error) {
	var buf bytes.Buffer
	if err := editor.EditStream(bytes.NewReader(content), &buf, filename, filter); err != nil {
		return nil, fmt.Errorf(errWrapFmt, ErrHCLUpdateFailed, err)
	}
	out := buf.Bytes()

	if err := validateHCL(out, filename); err != nil {
		return nil, err
	}
	return out, nil
}

// validateHCL is the post-edit validity guard: it fails if content does not
// parse as valid HCL, so applyFilter never returns bytes a caller might
// persist as a broken .tf file.
func validateHCL(content []byte, filename string) error {
	parser := hclparse.NewParser()
	if _, diags := parser.ParseHCL(content, filename); diags.HasErrors() {
		return fmt.Errorf(errWrapFmt, ErrHCLInvalidResult, diags)
	}
	return nil
}

// --- File wrappers -----------------------------------------------------------

// GetFile reads a Terraform file and returns the value at address.
func GetFile(filePath, address string, withComments bool) (string, error) {
	defer perf.Track(nil, "hcl.GetFile")()

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf(errWrapFmt, ErrHCLReadFile, err)
	}
	return Get(content, address, withComments)
}

// SetAttributeFile sets an attribute's value at address in a file, writing
// the result back atomically while preserving the original file mode.
func SetAttributeFile(filePath, address, value string) error {
	defer perf.Track(nil, "hcl.SetAttributeFile")()

	return mutateFile(filePath, func(content []byte, filename string) ([]byte, error) {
		return applyFilter(content, filename, editor.NewAttributeSetFilter(address, value))
	})
}

// AppendAttributeFile adds a new attribute at address in a file.
func AppendAttributeFile(filePath, address, value string, newline bool) error {
	defer perf.Track(nil, "hcl.AppendAttributeFile")()

	return mutateFile(filePath, func(content []byte, filename string) ([]byte, error) {
		return applyFilter(content, filename, editor.NewAttributeAppendFilter(address, value, newline))
	})
}

// RemoveAttributeFile removes the attribute at address in a file.
func RemoveAttributeFile(filePath, address string) error {
	defer perf.Track(nil, "hcl.RemoveAttributeFile")()

	return mutateFile(filePath, func(content []byte, filename string) ([]byte, error) {
		return applyFilter(content, filename, editor.NewAttributeRemoveFilter(address))
	})
}

// NewBlockFile creates a new empty block at address in a file.
func NewBlockFile(filePath, address string, newline bool) error {
	defer perf.Track(nil, "hcl.NewBlockFile")()

	return mutateFile(filePath, func(content []byte, filename string) ([]byte, error) {
		return applyFilter(content, filename, editor.NewBlockNewFilter(address, newline))
	})
}

// AppendBlockFile creates a new empty child block inside every block
// matching parent's address in a file.
func AppendBlockFile(filePath, parent, child string, newline bool) error {
	defer perf.Track(nil, "hcl.AppendBlockFile")()

	return mutateFile(filePath, func(content []byte, filename string) ([]byte, error) {
		return applyFilter(content, filename, editor.NewBlockAppendFilter(parent, child, newline))
	})
}

// RemoveBlockFile removes every block matching address in a file.
func RemoveBlockFile(filePath, address string) error {
	defer perf.Track(nil, "hcl.RemoveBlockFile")()

	return mutateFile(filePath, func(content []byte, filename string) ([]byte, error) {
		return applyFilter(content, filename, editor.NewBlockRemoveFilter(address))
	})
}

// mutateFile reads a file, applies fn, and writes the result back atomically
// (temp file + rename), preserving the original file mode. Symlinks are
// resolved first so editing a symlinked file rewrites the target instead of
// replacing the link with a regular file.
func mutateFile(filePath string, fn func(content []byte, filename string) ([]byte, error)) error {
	if resolved, err := filepath.EvalSymlinks(filePath); err == nil {
		filePath = resolved
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf(errWrapFmt, ErrHCLReadFile, err)
	}

	out, err := fn(content, filePath)
	if err != nil {
		return err
	}

	return atomicWrite(filePath, out)
}

// atomicWrite writes data to filePath via the shared cross-platform atomic
// writer, preserving the destination's existing permissions when it already
// exists.
func atomicWrite(filePath string, data []byte) error {
	mode := defaultFileMode
	if info, statErr := os.Stat(filePath); statErr == nil {
		mode = info.Mode().Perm()
	}

	if err := filesystem.NewOSFileSystem().WriteFileAtomic(filePath, data, mode); err != nil {
		return fmt.Errorf(errWrapFmt, ErrHCLWriteFile, err)
	}
	return nil
}
