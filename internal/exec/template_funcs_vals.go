package exec

import (
	"github.com/helmfile/vals"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

type valsLogWriter struct {
	cliConfig schema.CliConfiguration
}

func (w valsLogWriter) Write(p []byte) (int, error) {
	u.LogDebug(w.cliConfig, string(p))
	return len(p), nil
}

func valsFunc(cliConfig schema.CliConfiguration, ref string) (any, error) {
	vlw := valsLogWriter{cliConfig}
	return vals.Get(ref, vals.Options{LogOutput: vlw})
}
