package exec

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	u "github.com/cloudposse/atmos/pkg/utils"
)

// extractTarball extracts the tarball file into the destination directory
func extractTarball(sourceFile, extractPath string) error {
	file, err := os.Open(sourceFile)
	if err != nil {
		return err
	}

	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			u.LogError(fmt.Errorf("\nerror closing the file '%s': %v\n", sourceFile, err))
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
				u.LogError(fmt.Errorf("\nerror closing the file '%s': %v\n", sourceFile, err))
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
		}
	}
	return nil
}
