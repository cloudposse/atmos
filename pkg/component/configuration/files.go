package configuration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	fileMode      = 0o644
	secretMode    = 0o600
	directoryMode = 0o755
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
		if !filepath.IsLocal(file.Path) {
			return errUtils.ErrPluginProtocol
		}
		if !validFilePolicy(file.IfExists) {
			return errUtils.ErrPluginUnsupportedExtension
		}
	}
	for _, file := range files {
		if err := writeFile(root, file); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(root *os.Root, file schema.ComponentConfigurationFile) error {
	remove, err := checkFileOwnership(root, file)
	if err != nil {
		return err
	}
	if remove {
		err := root.Remove(file.Path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	mode := os.FileMode(fileMode)
	if file.Sensitive {
		mode = secretMode
		atmosio.RegisterSecretValue(string(file.Content))
	}
	if err := root.MkdirAll(filepath.Dir(file.Path), directoryMode); err != nil {
		return err
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if file.IfExists == "overwrite" || file.IfExists == "overwrite_terragrunt" {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	output, err := root.OpenFile(file.Path, flags, mode)
	if errors.Is(err, os.ErrExist) && file.IfExists == "skip" {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: generated file %s: %w", errUtils.ErrPluginInvalidConfig, file.Path, err)
	}
	if file.Sensitive {
		if err := output.Chmod(mode); err != nil {
			_ = output.Close()
			return err
		}
	}
	_, writeErr := output.Write(file.Content)
	return errors.Join(writeErr, output.Close())
}
