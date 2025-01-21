package exec

import (
	"fmt"
	"io"
	"os"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func removeTempDir(atmosConfig schema.AtmosConfiguration, path string) {
	err := os.RemoveAll(path)
	if err != nil {
		u.LogWarning(err.Error())
	}
}

func closeFile(fileName string, file io.ReadCloser) {
	err := file.Close()
	if err != nil {
		u.LogError(fmt.Errorf("error closing the file '%s': %v", fileName, err))
	}
}

// printOrWriteToFile takes the output format (`yaml` or `json`) and a file name,
// and prints the data to the console or to a file (if file is specified)
func printOrWriteToFile(
	format string,
	file string,
	data any,
) error {
	switch format {
	case "yaml":
		if file == "" {
			err := u.PrintAsYAML(data)
			if err != nil {
				return err
			}
		} else {
			err := u.WriteToFileAsYAML(file, data, 0644)
			if err != nil {
				return err
			}
		}

	case "json":
		if file == "" {
			err := u.PrintAsJSON(data)
			if err != nil {
				return err
			}
		} else {
			err := u.WriteToFileAsJSON(file, data, 0644)
			if err != nil {
				return err
			}
		}

	default:
		return fmt.Errorf("invalid 'format': %s", format)
	}

	return nil
}
