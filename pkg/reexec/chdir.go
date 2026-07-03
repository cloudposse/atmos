// Package reexec provides shared helpers for re-executing Atmos (either
// self-replacing the current process or spawning a child) with consistent
// handling of the --chdir flag and ATMOS_CHDIR environment variable.
//
// Re-exec call sites (version switching, profile fallback, CLI aliases) all
// need to strip --chdir from argv and ATMOS_CHDIR from env before handing
// control to the new process — otherwise a relative chdir applied by the
// parent would be re-applied by the child relative to the already-changed
// cwd, producing an incorrect or non-existent path.
package reexec

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// AtmosChdirEnvVar is the environment-variable form of the --chdir flag.
const AtmosChdirEnvVar = "ATMOS_CHDIR"

// StripChdirArgs returns a copy of args with --chdir / -C flags and their
// values removed. It handles all five accepted forms: `--chdir VAL`,
// `--chdir=VAL`, `-C VAL`, `-C=VAL`, and `-CVAL` (concatenated).
//
// The POSIX `--` end-of-flags separator is respected: once encountered,
// the separator and every token that follows are forwarded verbatim so
// positional arguments intended for downstream tools (terraform, helmfile,
// packer, custom commands) are never mistaken for atmos flags.
//
// Use this before re-executing Atmos so a chdir that was already applied
// by the parent is not re-applied by the child against the already-changed
// working directory.
func StripChdirArgs(args []string) []string {
	defer perf.Track(nil, "reexec.StripChdirArgs")()

	filtered := make([]string, 0, len(args))
	skipNext := false

	for i, arg := range args {
		// Once we hit the POSIX end-of-flags separator, everything
		// that follows is positional — forward the separator and
		// all remaining tokens verbatim.
		if arg == "--" {
			filtered = append(filtered, args[i:]...)
			return filtered
		}

		if skipNext {
			skipNext = false
			continue
		}

		// Skip --chdir=value, -C=value, -C<value> (concatenated).
		if strings.HasPrefix(arg, "--chdir=") ||
			strings.HasPrefix(arg, "-C=") ||
			(strings.HasPrefix(arg, "-C") && len(arg) > 2) {
			continue
		}

		// Skip --chdir value or -C value (next arg is the value).
		if arg == "--chdir" || arg == "-C" {
			skipNext = true
			continue
		}

		// Keep all other args.
		filtered = append(filtered, arg)
	}

	return filtered
}

// FilterChdirEnv returns a copy of environ with ATMOS_CHDIR removed. When
// the original contained an ATMOS_CHDIR entry, an explicit empty override
// (`ATMOS_CHDIR=`) is appended so the child cannot inherit the parent's
// value via environment merging.
func FilterChdirEnv(environ []string) []string {
	defer perf.Track(nil, "reexec.FilterChdirEnv")()

	filtered := make([]string, 0, len(environ))
	foundAtmosChdir := false
	prefix := AtmosChdirEnvVar + "="
	for _, env := range environ {
		if strings.HasPrefix(env, prefix) {
			foundAtmosChdir = true
			continue
		}
		filtered = append(filtered, env)
	}
	// Add empty ATMOS_CHDIR to override parent's value in merged environment.
	if foundAtmosChdir {
		filtered = append(filtered, prefix)
	}
	return filtered
}
