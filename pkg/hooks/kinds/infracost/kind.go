// Package infracost registers the built-in `infracost` hook kind.
//
// The kind defaults invoke `infracost breakdown --format json` against the
// component's terraform directory, capturing the structured cost report to
// $ATMOS_OUTPUT_FILE. A ResultHandler parses the JSON and produces a
// Summary envelope with a single markdown rendering used in every consumer
// (terminal, Pro run page, PR comments).
package infracost

import (
	"github.com/cloudposse/atmos/pkg/hooks"
)

// kindName is the registered identifier users select with `kind: infracost`.
const kindName = "infracost"

func init() {
	if err := hooks.RegisterKind(&hooks.Kind{
		Name:    kindName,
		Command: "infracost",
		DefaultArgs: []string{
			"breakdown",
			"--path", "$ATMOS_COMPONENT_PATH",
			"--format", "json",
			"--out-file", "$ATMOS_OUTPUT_FILE",
			"--no-color",
		},
		OnFailure:     hooks.OnFailureWarn,
		Engine:        &hooks.CommandEngine{},
		ResultHandler: ResultHandler,
	}); err != nil {
		panic("failed to register built-in infracost kind: " + err.Error())
	}
}
