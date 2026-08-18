package proexec

// syncAllowlist is the fixed, code-defined set of full Cobra command paths
// whose execution-record delivery blocks command completion (FR-007). Every
// other command is fire-and-forget (async). Both CaptureAsync and
// internal/exec's captureExecMetadataSync consult IsSyncCommand so the two
// delivery paths cannot independently drift out of sync — this is the single
// source of truth that replaced two separate, silently-diverging copies of
// this allowlist (research.md Decision 10).
//
//nolint:gochecknoglobals // Constant lookup table, not mutated after init.
var syncAllowlist = map[string]bool{
	"atmos terraform plan":    true,
	"atmos terraform apply":   true,
	"atmos terraform deploy":  true,
	"atmos describe affected": true,
}

// IsSyncCommand reports whether commandPath (a full Cobra command path, e.g.
// cmd.CommandPath() or "atmos "+"terraform "+subCommand) is on the
// synchronous execution-record delivery allowlist (FR-007).
func IsSyncCommand(commandPath string) bool {
	return syncAllowlist[commandPath]
}
