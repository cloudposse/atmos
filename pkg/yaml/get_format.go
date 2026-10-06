package yaml

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// GetFormatted reads a value as raw text or JSON. JSON encoding operates on the
// original YAML node so string values such as "false" retain their type.
func GetFormatted(content []byte, path, format string) (string, error) {
	defer perf.Track(nil, "yaml.GetFormatted")()

	if format != "raw" && format != "json" {
		return "", fmt.Errorf("%w: unsupported get format %q (expected raw or json)", errUtils.ErrInvalidArgumentError, format)
	}
	value, err := Get(content, path)
	if err != nil || format == "raw" {
		return value, err
	}
	yqPath, err := DotPathToYqPath(path)
	if err != nil {
		return "", err
	}
	return Query(content, "("+yqPath+") | to_json")
}
