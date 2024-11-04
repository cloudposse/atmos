package exec

import (
	"fmt"
	"strings"
	"sync"

	"github.com/helmfile/vals"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	valsOnce sync.Once
	valsInst *vals.Runtime
	valsErr  error
)

// valsLogWriter implements io.Writer interface to provide logging capabilities
// during vals operations using the provided CLI configuration
type valsLogWriter struct {
	cliConfig schema.CliConfiguration
}

func (w valsLogWriter) Write(p []byte) (int, error) {
	u.LogDebug(w.cliConfig, string(p))
	return len(p), nil
}

// Vals processes the given reference string and returns the corresponding value.
// The reference format should follow ref+BACKEND://PATH[?PARAMS][#FRAGMENT][+] URI-like expression.
// Example: ref+op://vault/item/field
// Returns the processed value or an error if the reference is invalid.
func valsFunc(cliConfig schema.CliConfiguration, ref string) (any, error) {
	if ref == "" {
		return nil, fmt.Errorf("vals reference cannot be empty")
	}

	// validate reference format
	if !strings.HasPrefix(ref, "ref+") {
		return nil, fmt.Errorf("vals invalid reference format: must start with 'ref+'")
	}

	vrt, err := valsRuntime(cliConfig)
	if err != nil {
		return nil, fmt.Errorf("vals failed to initialize runtime: %w", err)
	}

	res, err := vrt.Get(ref)
	if err != nil {
		return nil, fmt.Errorf("vals failed to get value for reference %q: %w", ref, err)
	}

	return res, nil
}

// vals singleton runtime to support builtin LRU cache and avoid multiple initialization
func valsRuntime(cliConfig schema.CliConfiguration) (*vals.Runtime, error) {
	valsOnce.Do(func() {
		vlw := valsLogWriter{cliConfig}
		valsInst, valsErr = vals.New(vals.Options{LogOutput: vlw})
	})
	return valsInst, valsErr
}
