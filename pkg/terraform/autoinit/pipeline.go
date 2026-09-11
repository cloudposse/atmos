package autoinit

// pipeline.go wires this package's fingerprint/decision/diagnostic primitives (Compute, Decide,
// Classify, ShouldRecover) into the shape a terraform execution pipeline needs: translating
// schema.ConfigAndStacksInfo / schema.AtmosConfiguration into Inputs/Request values, and
// Decision/Recovery results back into concrete init-argument lists, marker persistence, and a
// full classify -> policy -> force-init -> retry-once recovery flow driven entirely by
// caller-supplied closures. Callers (internal/exec) never re-implement any of this policy
// themselves -- they only supply the closures that actually run a subprocess.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	pkgversion "github.com/cloudposse/atmos/pkg/version"
)

// lookupEnvList returns the value of key from list (a "KEY=value" slice, typically
// info.ComponentEnvList), or "" if absent. The last matching entry wins, matching
// os/exec.Cmd.Env semantics for duplicate keys.
func lookupEnvList(list []string, key string) string {
	prefix := key + "="
	result := ""
	for _, e := range list {
		if strings.HasPrefix(e, prefix) {
			result = e[len(prefix):]
		}
	}
	return result
}

// componentEnvLookup returns a lookup function modeling the subprocess environment a caller is
// about to launch terraform/tofu with: envList (typically info.ComponentEnvList) first, falling
// back to the current process environment.
func componentEnvLookup(envList []string) func(string) string {
	return func(key string) string {
		if v := lookupEnvList(envList, key); v != "" {
			return v
		}
		//nolint:forbidigo // key is a Terraform env var (e.g. TF_CLI_ARGS), not an Atmos config var.
		return os.Getenv(key)
	}
}

// InputsFromInfo builds the Inputs describing this invocation's init fingerprint inputs, or nil
// on a dry run -- there is no component on disk yet to fingerprint, and Decide treats a nil
// Inputs as "always run init" (ReasonNoInputs).
func InputsFromInfo(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath, varFile string) *Inputs {
	defer perf.Track(nil, "autoinit.InputsFromInfo")()

	if info.DryRun {
		return nil
	}

	resolvedVarFile := varFile
	if resolvedVarFile != "" && !filepath.IsAbs(resolvedVarFile) {
		resolvedVarFile = filepath.Join(componentPath, resolvedVarFile)
	}

	return &Inputs{
		ComponentPath: componentPath,
		VarFile:       resolvedVarFile,
		PassVars:      atmosConfig.Components.Terraform.Init.PassVars,
		Binary:        info.Command,
		EnvLookup:     componentEnvLookup(info.ComponentEnvList),
	}
}

// workdirReprovisioned reports whether the workdir provisioner wiped and re-downloaded the
// component's workdir this invocation (WorkdirReprovisionedKey set in info.ComponentSection),
// e.g. because the workdir TTL expired. A re-provisioned workdir always needs a forced init: any
// prior init marker belonged to the wiped directory, not this one.
func workdirReprovisioned(info *schema.ConfigAndStacksInfo) bool {
	_, ok := info.ComponentSection[provWorkdir.WorkdirReprovisionedKey]
	return ok
}

// RequestFromInfo resolves the atmos.yaml init policy plus caller-agnostic always-force signals
// (the workspace subcommand, a re-provisioned workdir) into a Request ready for Decide. Force is
// the caller's own signal, e.g. an explicit `atmos terraform init`.
func RequestFromInfo(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, in *Inputs, force bool) *Request {
	defer perf.Track(atmosConfig, "autoinit.RequestFromInfo")()

	tf := &atmosConfig.Components.Terraform
	return &Request{
		Mode:        tf.EffectiveInitMode(),
		Reconfigure: tf.EffectiveInitReconfigure(),
		Upgrade:     tf.EffectiveInitUpgrade(),
		Force:       force || info.SubCommand == "workspace" || workdirReprovisioned(info),
		Inputs:      in,
	}
}

// OptedOut reports whether the caller explicitly disabled implicit init through any of Atmos's
// own opt-out signals: --skip-init, components.terraform.init.mode: never, or
// deploy_run_init: false on a deploy. Passed as Recover's OptedOut so a diagnosed init-required
// failure becomes a clear, actionable error (never a silent init) when the user asked not to
// initialize on their behalf.
func OptedOut(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) bool {
	defer perf.Track(atmosConfig, "autoinit.OptedOut")()

	if info.SkipInit {
		return true
	}
	if atmosConfig.Components.Terraform.EffectiveInitMode() == schema.TerraformInitModeNever {
		return true
	}
	if info.SubCommand == "deploy" && !atmosConfig.Components.Terraform.DeployRunInit {
		return true
	}
	return false
}

// RecordFromInfo best-effort records the init marker after a successful init. A failure only
// means smart init may unnecessarily re-run next time -- it never fails the (already
// successful) command that just completed. A nil in (dry run) is a no-op.
func RecordFromInfo(in *Inputs, initArgs []string) {
	defer perf.Track(nil, "autoinit.RecordFromInfo")()

	if in == nil {
		return
	}
	if err := Record(in, initArgs, pkgversion.Version); err != nil {
		log.Warn("Failed to record terraform init marker; smart init may re-run init unnecessarily next time", "error", err)
	}
}

// AnnounceSkipped tells the user (on the UI/stderr channel, never stdout) that a fresh
// `terraform init` was determined to be unnecessary this run.
func AnnounceSkipped(info *schema.ConfigAndStacksInfo, reason Reason) {
	defer perf.Track(nil, "autoinit.AnnounceSkipped")()

	ui.Info(fmt.Sprintf("Terraform init is up to date for %s in %s; skipping", info.ComponentFromArg, info.StackFromArg))
	log.Debug(
		"autoinit: skipped terraform init",
		"component", info.ComponentFromArg,
		"stack", info.StackFromArg,
		"reason", string(reason),
	)
}

// InitArgs builds the `terraform init` argument list from a Decision: `init`, then
// `-reconfigure` when d.Reconfigure, then `-upgrade` when d.Upgrade, then
// `-var-file <varFile>` when passVars is enabled.
//
//nolint:gocritic // Decision (88 bytes) is passed by value throughout this package and its tests; not worth a pointer for a non-hot-path helper.
func InitArgs(d Decision, passVars bool, varFile string) []string {
	defer perf.Track(nil, "autoinit.InitArgs")()

	args := []string{"init"}
	if d.Reconfigure {
		args = append(args, "-reconfigure")
	}
	if d.Upgrade {
		args = append(args, "-upgrade")
	}
	if passVars {
		args = append(args, "-var-file", varFile)
	}
	return args
}

// ApplyRecovery returns a copy of args with -upgrade and/or -reconfigure appended when rec calls
// for them and they aren't already present.
func ApplyRecovery(args []string, rec Recovery) []string {
	defer perf.Track(nil, "autoinit.ApplyRecovery")()

	out := slices.Clone(args)
	if rec.WithUpgrade && !slices.Contains(out, "-upgrade") {
		out = append(out, "-upgrade")
	}
	if rec.WithReconfigure && !slices.Contains(out, "-reconfigure") {
		out = append(out, "-reconfigure")
	}
	return out
}

// RecoverParams bundles what Recover needs to classify a failed main command's output and, if
// policy allows it, force one init re-run and retry the main command once.
type RecoverParams struct {
	// Output is the captured stdout+stderr (+ Err.Error()) from the failed main command.
	Output string
	// Err is the main command's own error; Recover never calls RunInit/Retry when Err is nil.
	Err error
	// Mode, Reconfigure, Upgrade are the caller's resolved atmos.yaml init policy (see
	// schema.Terraform's Effective* accessors).
	Mode        schema.TerraformInitMode
	Reconfigure schema.TerraformInitReconfigure
	Upgrade     schema.TerraformInitUpgrade
	// OptedOut reflects the caller's own opt-out signals (see OptedOut).
	OptedOut bool
	// Skip, when true, means recovery must not run at all (e.g. the command that failed was
	// itself `init` -- there is no "init required" fallback for init).
	Skip bool
	// RunInit forces one `terraform init` re-run with the flags rec calls for.
	RunInit func(rec Recovery) error
	// Retry re-runs the original failed command once, after RunInit succeeds.
	Retry func() error
	// Warn, if set, is called with a human-readable message before RunInit/Retry run.
	Warn func(string)
}

// Recover is smart init's plan/apply-time safety net: when the main command a caller just ran
// failed, it classifies p.Output for a known "init is required" diagnostic (Classify) and, if
// policy allows it (ShouldRecover), calls p.RunInit with the flag(s) the diagnostic asked for,
// then p.Retry exactly once. Returns p.Err unchanged when p.Skip is set, when nothing was
// diagnosed, or when ShouldRecover declines to recover. Returns errors.Join(p.Err, policyErr)
// when ShouldRecover reports a policy error (e.g. the caller explicitly disabled implicit init)
// -- that policy error is never silently swallowed. Returns errors.Join(p.Err, initErr) when
// p.RunInit itself fails.
//
//nolint:gocritic // RecoverParams (112 bytes) is passed by value at its single call site and in tests; not worth a pointer for a non-hot-path helper.
func Recover(p RecoverParams) error {
	defer perf.Track(nil, "autoinit.Recover")()

	if p.Err == nil || p.Skip {
		return p.Err
	}

	rec, policyErr := ShouldRecover(Classify(p.Output), p.Mode, p.Reconfigure, p.Upgrade, p.OptedOut)
	if policyErr != nil {
		return errors.Join(p.Err, policyErr)
	}
	if !rec.Run {
		return p.Err
	}

	if p.Warn != nil {
		p.Warn(fmt.Sprintf("Terraform requires initialization (%s); running init and retrying", rec.summary()))
	}

	if err := p.RunInit(rec); err != nil {
		return errors.Join(p.Err, err)
	}

	return p.Retry()
}

// summary renders a short, human-readable description of which flag(s) a Recovery asks for, for
// Recover's warning message.
func (r Recovery) summary() string {
	switch {
	case r.WithReconfigure && r.WithUpgrade:
		return "backend configuration changed and provider/module constraints require -upgrade"
	case r.WithReconfigure:
		return "backend configuration changed"
	case r.WithUpgrade:
		return "provider/module constraints require -upgrade"
	default:
		return "init is required"
	}
}

// RecoverInitParams bundles what RecoverInit needs to classify a failed `terraform init`
// subprocess's own output and, if it asked for -upgrade or -reconfigure that weren't already
// passed, retry init exactly once with the missing flag(s) added.
type RecoverInitParams struct {
	// Output is the captured stdout+stderr (+ Err.Error()) from the failed init subprocess.
	Output string
	// Err is init's own error; RecoverInit never calls Rerun when Err is nil.
	Err         error
	Mode        schema.TerraformInitMode
	Reconfigure schema.TerraformInitReconfigure
	Upgrade     schema.TerraformInitUpgrade
	// Rerun retries init once with the recovery's flags.
	Rerun func(rec Recovery) error
	// Warn, if set, is called with a human-readable message (and structured key/value pairs,
	// mirroring log.Warn) before Rerun runs.
	Warn func(msg string, kv ...any)
}

// RecoverInit is init's own (not plan/apply's) recovery path: when init itself just failed, it
// classifies p.Output and, if it asked for -upgrade/-reconfigure, calls p.Rerun once with those
// flags, returning the Recovery that was applied (zero value when none was) alongside the
// resulting error. There is no opt-out check here -- the caller is already initializing, so
// init.mode: never / --skip-init are not in play; a policy error (e.g. init.upgrade: never but
// the diagnostic demands -upgrade) is still surfaced, joined with p.Err.
//
//nolint:gocritic // RecoverInitParams (96 bytes) is passed by value at its single call site and in tests; not worth a pointer for a non-hot-path helper.
func RecoverInit(p RecoverInitParams) (Recovery, error) {
	defer perf.Track(nil, "autoinit.RecoverInit")()

	if p.Err == nil {
		return Recovery{}, nil
	}

	rec, policyErr := ShouldRecover(Classify(p.Output), p.Mode, p.Reconfigure, p.Upgrade, false)
	if policyErr != nil {
		return Recovery{}, errors.Join(p.Err, policyErr)
	}
	if !rec.Run {
		return Recovery{}, p.Err
	}

	if p.Warn != nil {
		p.Warn("Terraform init requires additional flags; retrying", "reconfigure", rec.WithReconfigure, "upgrade", rec.WithUpgrade)
	}

	if err := p.Rerun(rec); err != nil {
		return rec, err
	}
	return rec, nil
}
