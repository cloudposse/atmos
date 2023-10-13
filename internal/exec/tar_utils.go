package exec

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// extractTarball extracts the tarball file into the destination directory
func extractTarball(cliConfig schema.CliConfiguration, sourceFile, extractPath string) error {
	file, err := os.Open(sourceFile)
	if err != nil {
		return err
	}

	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			u.LogError(fmt.Errorf("error closing the file '%s': %v", sourceFile, err))
		}
	}(file)

	var fileReader io.ReadCloser = file

	if strings.HasSuffix(sourceFile, ".gz") {
		if fileReader, err = gzip.NewReader(file); err != nil {
			return err
		}

		defer func(fileReader io.ReadCloser) {
			err := fileReader.Close()
			if err != nil {
				u.LogError(fmt.Errorf("error closing the file '%s': %v", sourceFile, err))
			}
		}(fileReader)
	}

	tarBallReader := tar.NewReader(fileReader)

	for {
		header, err := tarBallReader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		filename := filepath.Join(extractPath, filepath.FromSlash(header.Name))

		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(filename, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

		case tar.TypeReg:
			err := u.EnsureDir(filename)
			if err != nil {
				return err
			}

			writer, err := os.Create(filename)
			if err != nil {
				return err
			}

			_, err = io.Copy(writer, tarBallReader)
			if err != nil {
				return err
			}

			err = os.Chmod(filename, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			err = writer.Close()
			if err != nil {
				return err
			}

		default:
			u.LogTrace(cliConfig, fmt.Sprintf("the header '%s' in the tarball '%s' has unsupported header type '%v'. "+
				"Supported header types are 'Directory' and 'File'",
				header.Name, sourceFile, header.Typeflag))
		}
	}
	return nil
}
