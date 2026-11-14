package configuration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	fileMode      = 0o644
	secretMode    = 0o600
	directoryMode = 0o755
	// Requests carry at most the permission, setuid, setgid, and sticky bits.
	maxRequestedMode = 0o7777
	// Granted modes keep owner rwx and group/other read-execute bits only.
	grantedModeMask = 0o755
	ownerReadWrite  = 0o600
)

// WriteFiles materializes evaluated bytes under the provisioned module, without templating.
func WriteFiles(directory string, files []schema.ComponentConfigurationFile) error {
	defer perf.Track(nil, "configuration.WriteFiles")()

	if len(files) == 0 {
		return nil
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrPluginInvalidConfig, err)
	}
	defer root.Close()
	for _, file := range files {
		if err := validateFile(file); err != nil {
			return err
		}
	}
	for _, file := range files {
		if err := writeFile(root, file); err != nil {
			return err
		}
	}
	return nil
}

func validateFile(file schema.ComponentConfigurationFile) error {
	if !filepath.IsLocal(file.Path) || file.Mode > maxRequestedMode {
		return errUtils.ErrPluginProtocol
	}
	if file.IfExists != "error" && file.IfExists != "skip" && file.IfExists != "overwrite" {
		return errUtils.ErrPluginUnsupportedExtension
	}
	return nil
}

func writeFile(root *os.Root, file schema.ComponentConfigurationFile) error {
	mode := writtenMode(file)
	if file.Sensitive {
		atmosio.RegisterSecretValue(string(file.Content))
	}
	if err := root.MkdirAll(filepath.Dir(file.Path), directoryMode); err != nil {
		return err
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if file.IfExists == "overwrite" {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	output, err := root.OpenFile(file.Path, flags, mode)
	if errors.Is(err, os.ErrExist) && file.IfExists == "skip" {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: generated file %s: %w", errUtils.ErrPluginInvalidConfig, file.Path, err)
	}
	// Open applies the process umask and ignores the mode of an existing file, so set it explicitly.
	chmodErr := applyMode(output, file, mode)
	if chmodErr != nil {
		return errors.Join(chmodErr, output.Close())
	}
	_, writeErr := output.Write(file.Content)
	return errors.Join(writeErr, output.Close())
}

// applyMode sets the permission bits explicitly because open applies the umask
// and leaves an existing file's mode alone. Windows keeps its ACLs.
func applyMode(output *os.File, file schema.ComponentConfigurationFile, mode os.FileMode) error {
	if (!file.Sensitive && file.Mode == 0) || runtime.GOOS == "windows" {
		return nil
	}
	return output.Chmod(mode)
}

// writtenMode returns the clamped permission bits for a generated file. An absent
// mode keeps the historical defaults. A requested mode keeps owner access (rw)
// and the read and execute bits, and drops everything that widens privilege.
// Sensitive files never grant group or other access.
func writtenMode(file schema.ComponentConfigurationFile) os.FileMode {
	if file.Sensitive {
		return os.FileMode(secretMode | file.Mode&0o100)
	}
	if file.Mode == 0 {
		return fileMode
	}
	return os.FileMode(file.Mode&grantedModeMask | ownerReadWrite)
}

// WatchPaths resolves provider-owned watches against the project
// root without putting checkout-specific absolute paths into stack comparisons.
// Existing YAML dependency path semantics remain unchanged. It reports whether
// the dependencies belong to a provider-discovered component.
func WatchPaths(config *schema.AtmosConfiguration, settings map[string]any, deps []schema.ComponentDependency) bool {
	defer perf.Track(nil, "configuration.WatchPaths")()
	if !schema.ConfigurationOwned(settings) {
		return false
	}
	for i := range deps {
		if !filepath.IsAbs(deps[i].Path) {
			deps[i].Path = filepath.Join(config.BasePathAbsolute, deps[i].Path)
		}
	}
	return true
}
