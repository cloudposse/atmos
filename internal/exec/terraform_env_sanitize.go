package exec

import "strings"

// terraformWorkspaceEnvBlocklist names env vars that must NOT reach
// `tofu workspace select` / `tofu workspace new` subprocesses.
//
// Why: OpenTofu prepends `TF_CLI_ARGS` to every subcommand's argv. When a CI
// workflow sets `TF_CLI_ARGS=-lock-timeout=10m` (sensible for plan/apply lock
// retries) and Atmos invokes `tofu workspace select` as a setup step, the
// workspace subcommand rejects the unknown flag with
// "flag provided but not defined: -lock-timeout" and the user's target
// subcommand never runs. `TF_CLI_ARGS_workspace` is the same hazard with a
// narrower scope and no per-sub-subcommand override.
//
// Per-subcommand variants (`TF_CLI_ARGS_plan`, `TF_CLI_ARGS_apply`,
// `TF_CLI_ARGS_init`, etc.) are intentionally NOT blocked — OpenTofu only
// applies them to their named subcommand, so they cannot affect
// `workspace select` / `workspace new`, and stripping them would silently
// drop user-intentional configuration before it reached its target command.
//
// See docs/fixes/2026-04-27-tf-cli-args-breaks-workspace-select.md.
var terraformWorkspaceEnvBlocklist = map[string]struct{}{
	"TF_CLI_ARGS":           {},
	"TF_CLI_ARGS_workspace": {},
}

// sanitizeTerraformWorkspaceEnv returns a copy of env with the variables in
// terraformWorkspaceEnvBlocklist removed. The env parameter is a slice of
// "KEY=VALUE" entries, matching the format of os.Environ().
//
// Entries without an '=' separator are preserved unchanged; we cannot classify
// them as belonging to a blocked key, and dropping them would diverge from
// os.Environ() semantics.
func sanitizeTerraformWorkspaceEnv(env []string) []string {
	if len(env) == 0 {
		return env
	}
	out := make([]string, 0, len(env))
	for _, kv := range env {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			out = append(out, kv)
			continue
		}
		if _, blocked := terraformWorkspaceEnvBlocklist[kv[:eq]]; blocked {
			continue
		}
		out = append(out, kv)
	}
	return out
}
