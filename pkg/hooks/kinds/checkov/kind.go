// Package checkov registers the built-in `checkov` hook kind.
//
// Defaults invoke `checkov -d $ATMOS_COMPONENT_PATH -o sarif
// --output-file-path $ATMOS_OUTPUT_DIR`. Findings flow through the shared
// SARIF parser; the rendered markdown summary is identical across the
// terminal, Pro run page, PR comments, and step summaries.
package checkov

import (
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/cacerts"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/hooks/sarif"
)

const (
	kindName = "checkov"
	// The resultFileName is what checkov writes when invoked with -o sarif
	// --output-file-path <dir>. Despite its name, --output-file-path
	// takes a directory, and checkov writes results_<format>.<format>
	// inside it.
	resultFileName = "results_sarif.sarif"
)

func init() {
	if err := hooks.RegisterKind(&hooks.Kind{
		Name:    kindName,
		Command: "checkov",
		DefaultArgs: []string{
			"-d", "$ATMOS_COMPONENT_PATH",
			"-o", "sarif",
			"--output-file-path", "$ATMOS_OUTPUT_DIR",
			"--quiet",
			"--soft-fail",
		},
		// Checkov ships as a PyInstaller-bundled binary with a frozen
		// `certifi` CA bundle. The frozen bundle frequently can't
		// validate the TLS chain serving api0.prismacloud.io (the API
		// checkov hits at startup to fetch guideline mappings), and the
		// fallback path emits an unsightly Python traceback before
		// continuing. The official workaround for PyInstaller binaries
		// (pyinstaller/pyinstaller#7229) is to point SSL_CERT_FILE at a
		// fresh host CA bundle — pkg/cacerts.Env() returns just that
		// (cross-platform, with a sane Windows fallback to nil so we
		// don't override there).
		//
		// Doing this lets checkov fetch its Prisma mappings successfully
		// AND keeps the noise out of normal runs. If the host has no
		// detectable bundle, Env() returns nil and we make no change —
		// checkov falls back to its own certifi, same as before.
		DefaultEnv: cacerts.Env(),
		OnFailure:  hooks.OnFailureWarn,
		Engine:     &hooks.CommandEngine{},
		ResultHandler: sarif.NewResultHandler(sarif.HandlerOptions{
			Kind: kindName,
			OutputPath: func(ctx *hooks.ExecContext) string {
				if ctx == nil || ctx.OutputDir == "" {
					return ""
				}
				return filepath.Join(ctx.OutputDir, resultFileName)
			},
		}),
	}); err != nil {
		panic("failed to register built-in checkov kind: " + err.Error())
	}
}
