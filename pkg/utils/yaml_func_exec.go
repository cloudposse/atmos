package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ErrExecCommandNotAllowed is returned when an !exec command is blocked by the exec.allowed_commands allowlist.
var ErrExecCommandNotAllowed = errors.New("!exec command blocked by exec.allowed_commands allowlist")

// sensitiveEnvPrefixes contains environment variable name prefixes whose values are always stripped
// before a shell command is executed via !exec.
var sensitiveEnvPrefixes = []string{
	"AWS_SECRET_",
}

// sensitiveEnvSuffixes contains environment variable name suffixes whose values are always stripped
// before a shell command is executed via !exec.
var sensitiveEnvSuffixes = []string{
	"_TOKEN",
	"_SECRET",
	"_PASSWORD",
	"_PASSWD",
	"_API_KEY",
	"_PRIVATE_KEY",
}

// isSensitiveEnvVar reports whether an environment variable name matches a known
// credential-bearing pattern and should be excluded from the !exec shell environment.
func isSensitiveEnvVar(name string) bool {
	upper := strings.ToUpper(name)
	for _, prefix := range sensitiveEnvPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return true
		}
	}
	for _, suffix := range sensitiveEnvSuffixes {
		if strings.HasSuffix(upper, suffix) {
			return true
		}
	}
	return false
}

// sanitizeEnv returns a copy of env with credential-bearing variables removed.
func sanitizeEnv(env []string) []string {
	result := make([]string, 0, len(env))
	for _, e := range env {
		name, _, _ := strings.Cut(e, "=")
		if isSensitiveEnvVar(name) {
			log.Debug("Stripping sensitive environment variable from !exec environment", "var", name)
			continue
		}
		result = append(result, e)
	}
	return result
}

// ProcessTagExec executes the shell command specified in an !exec YAML tag and returns
// its output.  When atmosConfig is non-nil and exec.allowed_commands is non-empty, only
// commands whose executable name appears in the allowlist are permitted; any other command
// (including dynamically constructed names) is rejected with ErrExecCommandNotAllowed.
// Credential-bearing environment variables are always stripped before the command runs.
func ProcessTagExec(
	input string,
	atmosConfig *schema.AtmosConfiguration,
) (any, error) {
	defer perf.Track(nil, "utils.ProcessTagExec")()

	log.Debug("Executing Atmos YAML function", "function", input)
	str, err := getStringAfterTag(input, AtmosYamlFuncExec)
	if err != nil {
		return nil, err
	}

	// Enforce the exec.allowed_commands allowlist when configured.
	if atmosConfig != nil && len(atmosConfig.Exec.AllowedCommands) > 0 {
		names, allLiteral, parseErr := extractCommandNamesFromShell(str)
		if parseErr != nil {
			return nil, fmt.Errorf("%w: failed to parse command: %v", ErrExecCommandNotAllowed, parseErr)
		}
		if !allLiteral {
			return nil, fmt.Errorf("%w: dynamic command names are not permitted when exec.allowed_commands is configured", ErrExecCommandNotAllowed)
		}
		allowed := make(map[string]struct{}, len(atmosConfig.Exec.AllowedCommands))
		for _, c := range atmosConfig.Exec.AllowedCommands {
			allowed[c] = struct{}{}
		}
		for _, name := range names {
			if _, ok := allowed[name]; !ok {
				return nil, fmt.Errorf("%w: %q is not in the allowlist", ErrExecCommandNotAllowed, name)
			}
		}
	}

	res, err := ExecuteShellAndReturnOutput(str, input, ".", sanitizeEnv(os.Environ()), false)
	if err != nil {
		return nil, err
	}

	// Strip trailing newlines to match shell command substitution behavior.
	// In shell, $(cmd) strips trailing newlines from the output.
	res = strings.TrimRight(res, "\n")

	var decoded any
	if err = json.Unmarshal([]byte(res), &decoded); err != nil {
		log.Debug("Unmarshalling error", "error", err)
		decoded = res
	}

	return decoded, nil
}
