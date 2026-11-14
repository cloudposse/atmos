//go:build !windows

package configuration

import (
	"errors"
	"os"

	"github.com/cloudposse/atmos/pkg/schema"
)

func createSensitiveTempFile(root *os.Root, name string) (*os.File, error) {
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, secretMode)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(secretMode); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}

func openGeneratedFile(root *os.Root, file schema.ComponentConfigurationFile, exists bool) (*os.File, error) {
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if exists {
		flags = os.O_WRONLY
	}
	output, err := root.OpenFile(file.Path, flags, secretMode)
	if err != nil {
		return nil, err
	}
	if err := output.Chmod(secretMode); err != nil {
		return nil, errors.Join(err, output.Close())
	}
	return output, nil
}

func restrictExistingGeneratedFile(root *os.Root, file schema.ComponentConfigurationFile) error {
	return root.Chmod(file.Path, secretMode)
}
