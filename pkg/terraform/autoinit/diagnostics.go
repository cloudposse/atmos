package autoinit

import (
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosansi "github.com/cloudposse/atmos/pkg/ansi"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Diagnosis summarizes what terraform/tofu output told us about the init state after a
// subcommand failed.
type Diagnosis struct {
	// InitRequired reports whether the output indicates `init` must run again.
	InitRequired bool
	// ReconfigureRequired reports whether the backend configuration changed.
	ReconfigureRequired bool
	// UpgradeRequired reports whether provider/module constraints require `-upgrade`.
	UpgradeRequired bool
	// Matched is the signature substring that produced this Diagnosis, for debug logs.
	Matched string
}

// signature pairs a known terraform/OpenTofu diagnostic substring with the Diagnosis it implies.
type signature struct {
	text string
	diag Diagnosis
}

// signatures is checked in order; the first match wins. Reconfigure/upgrade signatures also imply
// InitRequired, since resolving either requires running init again.
var signatures = []signature{
	{"must use terraform init -upgrade", Diagnosis{InitRequired: true, UpgradeRequired: true}},
	{"must use tofu init -upgrade", Diagnosis{InitRequired: true, UpgradeRequired: true}},
	{"Backend configuration changed", Diagnosis{InitRequired: true, ReconfigureRequired: true}},
	{`Backend initialization required, please run "terraform init"`, Diagnosis{InitRequired: true}},
	{`Backend initialization required, please run "tofu init"`, Diagnosis{InitRequired: true}},
	{"Required plugins are not installed", Diagnosis{InitRequired: true}},
	{"Inconsistent dependency lock file", Diagnosis{InitRequired: true}},
	{"Module not installed", Diagnosis{InitRequired: true}},
	{"Module source has changed", Diagnosis{InitRequired: true}},
	{"Module version requirements have changed", Diagnosis{InitRequired: true}},
}

// Classify inspects raw terraform/tofu output for a known init-related diagnostic and returns the
// corresponding Diagnosis; output is ANSI-stripped first, since captured subprocess output may
// still carry color codes. Returns a zero Diagnosis when nothing matches.
func Classify(output string) Diagnosis {
	defer perf.Track(nil, "autoinit.Classify")()

	plain := atmosansi.Strip(output)
	for _, sig := range signatures {
		if strings.Contains(plain, sig.text) {
			d := sig.diag
			d.Matched = sig.text
			return d
		}
	}
	return Diagnosis{}
}

// Recovery describes the init recovery Atmos should perform after Classify reports a Diagnosis.
type Recovery struct {
	// Run reports whether init should be re-run.
	Run bool
	// WithReconfigure reports whether the re-run should add `-reconfigure`.
	WithReconfigure bool
	// WithUpgrade reports whether the re-run should add `-upgrade`.
	WithUpgrade bool
}

// ShouldRecover applies Atmos's init recovery policy to d: no recovery when nothing was diagnosed,
// an error (never a silent init) when the caller explicitly opted out of implicit init, an error
// when a required upgrade or reconfigure is itself disabled by policy, and otherwise a concrete
// Recovery describing the re-run; optedOut reflects the caller's own opt-out signals (--skip-init,
// components.terraform.init.mode: never, deploy_run_init: false); mode is accepted for
// troubleshooting context in the debug log even though the opt-out itself is the caller's
// responsibility to compute.
func ShouldRecover(
	d Diagnosis,
	mode schema.TerraformInitMode,
	reconfigure schema.TerraformInitReconfigure,
	upgrade schema.TerraformInitUpgrade,
	optedOut bool,
) (Recovery, error) {
	defer perf.Track(nil, "autoinit.ShouldRecover")()

	log.Debug("autoinit: init recovery decision", "mode", string(mode), "opted_out", optedOut, "matched", d.Matched)

	if !d.InitRequired {
		return Recovery{}, nil
	}
	if optedOut {
		return Recovery{}, errUtils.Build(errUtils.ErrTerraformInitRequired).
			WithHint("Run `atmos terraform init <component> -s <stack>`, or remove `--skip-init` / set `components.terraform.init.mode: auto` so Atmos initializes automatically.").
			Err()
	}
	if d.UpgradeRequired && upgrade == schema.TerraformInitUpgradeNever {
		return Recovery{}, errUtils.Build(errUtils.ErrTerraformInitUpgradeRequired).
			WithHint("Run `atmos terraform init <component> -s <stack> -- -upgrade`, or set `components.terraform.init.upgrade: auto` (`--init-upgrade=auto`) so Atmos upgrades automatically.").
			Err()
	}
	if d.ReconfigureRequired && reconfigure == schema.TerraformInitReconfigureNever {
		return Recovery{}, errUtils.Build(errUtils.ErrTerraformInitReconfigureRequired).
			WithHint("Run `atmos terraform init <component> -s <stack> -- -reconfigure`, or set `components.terraform.init.reconfigure: auto` so Atmos reconfigures automatically.").
			Err()
	}

	return Recovery{
		Run:             true,
		WithUpgrade:     d.UpgradeRequired || upgrade == schema.TerraformInitUpgradeAlways,
		WithReconfigure: d.ReconfigureRequired || reconfigure == schema.TerraformInitReconfigureAlways,
	}, nil
}
