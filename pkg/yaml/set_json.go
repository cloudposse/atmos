package yaml

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// SetJSON assigns a JSON value as data, preserving its string escapes and the
// surrounding YAML comments. JSON is decoded rather than evaluated as yq code.
func SetJSON(content []byte, path string, value []byte) ([]byte, error) {
	defer perf.Track(nil, "yaml.SetJSON")()

	if !json.Valid(value) {
		return nil, fmt.Errorf("%w: invalid JSON value", errUtils.ErrInvalidArgumentError)
	}
	// Keep the expression parser from interpreting JSON escapes before the JSON
	// decoder sees them. The encoded literal contains no yq syntax.
	encoded := base64.StdEncoding.EncodeToString(value)
	return SetRaw(content, path, "(\""+encoded+"\" | @base64d | from_json)")
}
