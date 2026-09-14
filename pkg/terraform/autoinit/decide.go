package autoinit

import (
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// shortHashLen is how many leading hex characters of a fingerprint hash are logged, enough to
// disambiguate in practice without cluttering debug logs with a full sha256 digest.
const shortHashLen = 12

// Reason explains why Decide chose to run (or skip) init.
type Reason string

// Reasons returned by Decide, in the order they are checked.
const (
	ReasonModeAlways          Reason = "init.mode is always"
	ReasonModeNever           Reason = "init.mode is never"
	ReasonForced              Reason = "init forced by caller"
	ReasonNoInputs            Reason = "no fingerprint inputs (dry run)"
	ReasonNoMarker            Reason = "no init marker found"
	ReasonSchemaVersion       Reason = "init marker schema version changed"
	ReasonFingerprintChanged  Reason = "init inputs changed"
	ReasonProvidersMissing    Reason = "provider plugins are not installed"
	ReasonModulesMissing      Reason = "modules are not installed"
	ReasonBackendStateMissing Reason = "backend is not initialized"
	ReasonFingerprintError    Reason = "fingerprint could not be computed"
	ReasonUpToDate            Reason = "init is up to date"
)

// Request captures everything Decide needs to determine whether init should run, and with which
// flags.
type Request struct {
	// Mode is the configured init mode; empty is treated as schema.TerraformInitModeAuto.
	Mode schema.TerraformInitMode
	// Reconfigure is the configured reconfigure policy; empty is treated as
	// schema.TerraformInitReconfigureAuto.
	Reconfigure schema.TerraformInitReconfigure
	// Upgrade is the configured upgrade policy; empty is treated as
	// schema.TerraformInitUpgradeAuto.
	Upgrade schema.TerraformInitUpgrade
	// Force short-circuits to RunInit=true regardless of the fingerprint, e.g. for the
	// workspace subcommand, a re-provisioned workdir, or an explicit `atmos terraform init`.
	Force bool
	// Inputs is nil on a dry run (no component on disk to fingerprint yet).
	Inputs *Inputs
}

// Decision is the outcome of Decide.
type Decision struct {
	// RunInit reports whether `terraform init` should run at all.
	RunInit bool
	// Reconfigure reports whether `-reconfigure` should be added.
	Reconfigure bool
	// Upgrade reports whether `-upgrade` should be added.
	Upgrade bool
	// Reason explains RunInit.
	Reason Reason
	// Fingerprint is the computed fingerprint; zero when it was never computed (e.g. dry run, or
	// init.mode is never).
	Fingerprint Fingerprint
	// Marker is the previously recorded marker; nil when absent, malformed, or never read.
	Marker *Marker
}

// Decide determines whether `terraform init` should run, and with which flags, given req. Decide
// never fails: any error while computing a fingerprint or reading the marker degrades to
// RunInit=true, so a fingerprinting bug can never permanently block a user from getting a working
// init.
func Decide(req *Request) Decision {
	defer perf.Track(nil, "autoinit.Decide")()

	d := Decision{Upgrade: req.Upgrade == schema.TerraformInitUpgradeAlways}

	if req.Mode == schema.TerraformInitModeNever {
		d.Reason = ReasonModeNever
		d.Reconfigure = req.Reconfigure == schema.TerraformInitReconfigureAlways
		logDecision(d)
		return d
	}

	fp, marker, fpErr := probeFingerprint(req)
	d.Fingerprint, d.Marker = fp, marker
	d.RunInit, d.Reason = decideRunInit(req, fp, marker, fpErr)
	d.Reconfigure = decideReconfigure(req, d)

	logDecision(d)
	return d
}

// probeFingerprint computes the current fingerprint and reads the last recorded marker, when
// req.Inputs is available; a marker read failure degrades to "no marker" rather than propagating
// as an error, matching ReadMarker's own contract for absence vs. Malformed content; only a
// fingerprint *compute* failure is treated as an error by the caller.
func probeFingerprint(req *Request) (Fingerprint, *Marker, error) {
	if req.Inputs == nil {
		return Fingerprint{}, nil, nil
	}

	fp, err := Compute(req.Inputs)
	if err != nil {
		log.Debug("autoinit: failed to compute init fingerprint", "component", req.Inputs.ComponentPath, "error", err)
		return Fingerprint{}, nil, err
	}

	marker, markerErr := ReadMarker(MarkerPath(req.Inputs.effectiveDataDir()))
	if markerErr != nil {
		log.Debug("autoinit: failed to read init marker", "error", markerErr)
		marker = nil
	}
	return fp, marker, nil
}

// decideRunInit implements the RunInit/Reason half of Decide, given an already-computed
// fingerprint probe.
func decideRunInit(req *Request, fp Fingerprint, marker *Marker, fpErr error) (bool, Reason) {
	switch {
	case req.Mode == schema.TerraformInitModeAlways:
		return true, ReasonModeAlways
	case req.Force:
		return true, ReasonForced
	case req.Inputs == nil:
		return true, ReasonNoInputs
	case fpErr != nil:
		return true, ReasonFingerprintError
	case marker == nil:
		return true, ReasonNoMarker
	case marker.SchemaVersion != MarkerSchemaVersion:
		return true, ReasonSchemaVersion
	default:
		return decideFromPreconditions(req.Inputs, marker, fp)
	}
}

// decideFromPreconditions checks the filesystem preconditions (providers, modules, backend state)
// and finally compares the recorded and current fingerprint hashes.
func decideFromPreconditions(in *Inputs, marker *Marker, fp Fingerprint) (bool, Reason) {
	dataDir := in.effectiveDataDir()
	switch {
	case providersMissing(dataDir, in.ComponentPath):
		return true, ReasonProvidersMissing
	case modulesMissing(dataDir, in.ComponentPath):
		return true, ReasonModulesMissing
	case backendStateMissing(dataDir, in.ComponentPath):
		return true, ReasonBackendStateMissing
	case marker.Fingerprint != fp.Hash:
		return true, ReasonFingerprintChanged
	default:
		return false, ReasonUpToDate
	}
}

// decideReconfigure implements the Reconfigure half of Decide: always/never are explicit
// overrides, auto reconfigures whenever init is forced, no marker was ever recorded, the backend
// fingerprint no longer matches, or the comparison cannot be made at all (no inputs, or the
// fingerprint could not be computed) -- reconfiguring is the conservative default when Atmos
// cannot tell whether the backend changed.
//
//nolint:gocritic // Decision (88 bytes) is passed by value throughout this package and its tests; not worth a pointer for a non-hot-path helper.
func decideReconfigure(req *Request, d Decision) bool {
	switch req.Reconfigure {
	case schema.TerraformInitReconfigureAlways:
		return true
	case schema.TerraformInitReconfigureNever:
		return false
	default:
		if req.Inputs == nil || d.Fingerprint.Hash == "" {
			return true
		}
		return req.Force || d.Marker == nil || d.Marker.BackendFingerprint != d.Fingerprint.BackendHash
	}
}

// logDecision emits a debug-level summary of d for troubleshooting init skip/run decisions.
//
//nolint:gocritic // Decision (88 bytes) is passed by value throughout this package and its tests; not worth a pointer for a non-hot-path helper.
func logDecision(d Decision) {
	log.Debug(
		"autoinit: init decision",
		"run_init", d.RunInit,
		"reconfigure", d.Reconfigure,
		"upgrade", d.Upgrade,
		"reason", string(d.Reason),
		"hash", shortHash(d.Fingerprint.Hash),
		"files", d.Fingerprint.Files,
	)
}

// shortHash returns the leading shortHashLen characters of hash, or hash itself when shorter.
func shortHash(hash string) string {
	if len(hash) <= shortHashLen {
		return hash
	}
	return hash[:shortHashLen]
}
