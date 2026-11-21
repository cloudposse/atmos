package configuration

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

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
		if err := validateDestination(root, file.Path); err != nil {
			return generatedFileError(file.Path, err)
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
	if IsRuntimeArtifact(file.Path) {
		return generatedFileError(file.Path, errUtils.ErrPluginInvalidConfig)
	}
	return nil
}

func writeFile(root *os.Root, file schema.ComponentConfigurationFile) (result error) {
	if err := validateFile(file); err != nil {
		return err
	}
	directory, err := destinationDirectory(root, filepath.Dir(file.Path), true)
	if err != nil {
		return generatedFileError(file.Path, err)
	}
	defer directory.Close()
	mode := writtenMode(file)
	output, temporary, err := openGeneratedFile(directory, filepath.Base(file.Path), file.IfExists, mode, !file.Sensitive && file.Mode == 0)
	if err != nil {
		return generatedFileError(file.Path, err)
	}
	if output == nil {
		return nil
	}
	if temporary != "" {
		defer func() {
			result = errors.Join(result, generatedFileError(file.Path, removeGeneratedTemporary(directory, temporary)))
		}()
	}
	if err := writeGeneratedContent(output, file, mode); err != nil {
		return generatedFileError(file.Path, err)
	}
	if temporary != "" {
		if err := validateDestination(directory, filepath.Base(file.Path)); err != nil {
			return generatedFileError(file.Path, err)
		}
		return generatedFileError(file.Path, directory.Rename(temporary, filepath.Base(file.Path)))
	}
	return nil
}

func removeGeneratedTemporary(directory *os.Root, name string) error {
	err := directory.Remove(name)
	if errors.Is(err, os.ErrNotExist) {
		// A successful rename already consumed the temporary entry.
		return nil
	}
	return err
}

func writeGeneratedContent(output *os.File, file schema.ComponentConfigurationFile, mode os.FileMode) error {
	if file.Sensitive {
		atmosio.RegisterSecretValue(string(file.Content))
	}
	// Restrict permissions before writing any sensitive content.
	if err := applyMode(output, file, mode); err != nil {
		return errors.Join(err, output.Close())
	}
	_, writeErr := output.Write(file.Content)
	return errors.Join(writeErr, output.Close())
}

func generatedFileError(path string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: generated file %s: %w", errUtils.ErrPluginInvalidConfig, path, err)
}

// validateDestination checks existing ancestors and the leaf before the first
// write. Missing directories are created only during the subsequent write phase.
func validateDestination(root *os.Root, path string) error {
	directory, err := destinationDirectory(root, filepath.Dir(path), false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer directory.Close()
	info, err := directory.Lstat(filepath.Base(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errUtils.ErrPluginInvalidConfig
	}
	return nil
}

// destinationDirectory pins each ordinary directory by handle. Lstat/Stat identity
// checks reject symlinks introduced between inspection and opening an ancestor.
func destinationDirectory(root *os.Root, path string, create bool) (*os.Root, error) {
	current, err := root.OpenRoot(".")
	if err != nil {
		return nil, err
	}
	for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		if part == "." {
			continue
		}
		next, err := openDestinationChild(current, part, create)
		_ = current.Close()
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}

func openDestinationChild(root *os.Root, name string, create bool) (*os.Root, error) {
	if create {
		if err := root.Mkdir(name, directoryMode); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.IsDir() {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	child, err := root.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	after, err := child.Stat(".")
	if err != nil || !os.SameFile(before, after) {
		_ = child.Close()
		return nil, errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	return child, nil
}

// openGeneratedFile replaces existing entries through a fresh inode under the
// pinned parent. No chmod or write touches an inode shared by other hard links.
// Exclusive creation retains the no-clobber guarantee for new destinations.
func openGeneratedFile(root *os.Root, name, policy string, mode os.FileMode, preserveMode bool) (*os.File, string, error) {
	before, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		output, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		return output, "", err
	}
	if err != nil {
		return nil, "", err
	}
	if !before.Mode().IsRegular() {
		return nil, "", errUtils.ErrPluginInvalidConfig
	}
	if policy == "skip" {
		return nil, "", nil
	}
	if policy == "error" {
		return nil, "", os.ErrExist
	}
	if preserveMode {
		mode = before.Mode().Perm()
	}
	temporary := ".atmos-generated-" + rand.Text()
	output, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	return output, temporary, err
}

// IsRuntimeArtifact reserves Terraform state, lock files and workdir metadata.
// These names mirror the provisioner/workdir contract; importing that package
// here would create a configuration-loading cycle.
// Case folding keeps the same protection when a configuration moves between
// case-sensitive and case-insensitive filesystems.
//
//nolint:lintroller // Pure path validation with no I/O; exempt from performance tracking.
func IsRuntimeArtifact(path string) bool {
	path = strings.ToLower(filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))))
	if path == "terraform.tfstate" || path == "terraform.tfstate.backup" || path == ".terraform.tfstate.lock.info" || strings.HasSuffix(path, ".terraform.lock.hcl") {
		return true
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == ".terraform" || segment == "terraform.tfstate.d" || segment == ".atmos" {
			return true
		}
	}
	return false
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
