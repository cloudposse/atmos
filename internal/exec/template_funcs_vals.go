package exec

import (
	"fmt"

	"github.com/helmfile/vals"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
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

func valsFunc(cliConfig schema.CliConfiguration, ref string) (any, error) {
	if ref == "" {
		return nil, fmt.Errorf("empty vals ref code is provided")
	}

	vlw := valsLogWriter{cliConfig}
	res, err := vals.Get(ref, vals.Options{LogOutput: vlw})
	if err != nil {
		return nil, fmt.Errorf("failed to get value for ref %q: %w", ref, err)
	}

	return res, nil
}
