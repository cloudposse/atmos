package auth

import (
	errUtils "github.com/cloudposse/atmos/errors"
)

// logoutSelection holds the target selectors given to `atmos auth logout`.
type logoutSelection struct {
	// Args are the positional args (an identity name in args[0]).
	args []string
	// IdentityFlag is the explicit --identity value. It may be
	// IdentityFlagSelectValue when --identity was given without a value.
	identityFlag string
	// IdentityEnv is the identity from ATMOS_IDENTITY.
	identityEnv string
	provider    string
	tags        []string
	all         bool
	// AllRealms is --all-realms, which needs no identity.
	allRealms bool
}

// logoutTargetKind is what `atmos auth logout` logs out of.
type logoutTargetKind int

const (
	// The user chooses interactively.
	logoutTargetInteractive logoutTargetKind = iota
	// Every identity.
	logoutTargetAll
	// Every provider matching --tags.
	logoutTargetTags
	// The provider named by --provider.
	logoutTargetProvider
	// A single identity.
	logoutTargetIdentity
	// Every realm across all repositories.
	logoutTargetAllRealms
	// A single identity chosen with the identity selector (a bare `--identity`).
	logoutTargetSelectIdentity
)

// logoutTarget is the resolved logout target.
type logoutTarget struct {
	kind logoutTargetKind
	// Identity is set for logoutTargetIdentity.
	identity string
}

// resolveLogoutTarget decides what to log out of from the selectors, in the
// order --all, --tags, --provider, positional identity, --identity, ATMOS_IDENTITY,
// and otherwise interactive selection. A bare `--identity` selects an identity
// with the identity selector.
func resolveLogoutTarget(sel *logoutSelection) (logoutTarget, error) {
	if err := checkLogoutTagsConflict(sel); err != nil {
		return logoutTarget{}, err
	}

	switch {
	case sel.allRealms:
		// --all-realms does not depend on the ambient identity, so skip identity validation.
		return logoutTarget{kind: logoutTargetAllRealms}, nil
	case sel.all:
		return logoutTarget{kind: logoutTargetAll}, nil
	case len(sel.tags) > 0:
		return logoutTarget{kind: logoutTargetTags}, nil
	case sel.provider != "":
		return logoutTarget{kind: logoutTargetProvider}, nil
	}

	// Support both positional argument and --identity flag for consistency with other auth commands.
	// Positional argument takes precedence if both are provided.
	if len(sel.args) > 0 {
		return logoutTarget{kind: logoutTargetIdentity, identity: sel.args[0]}, nil
	}
	return resolveLogoutIdentityTarget(sel)
}

// resolveLogoutIdentityTarget resolves the target from --identity or ATMOS_IDENTITY

// checkLogoutTagsConflict rejects --tags combined with another explicit target selector.
// The --tags flag is mutually exclusive with --provider/--all/--identity/positional identity:
// it's an alternative way to select a target, not a composable cross-cutting filter here.
// ATMOS_IDENTITY is not an explicit selector (`atmos auth shell` exports it), so it
// never conflicts with --tags; it is only used when nothing else selects a target.
func checkLogoutTagsConflict(sel *logoutSelection) error {
	if len(sel.tags) == 0 {
		return nil
	}
	if sel.provider != "" || sel.all || sel.identityFlag != "" || len(sel.args) > 0 {
		return errUtils.ErrMutuallyExclusiveFlags
	}
	return nil
}

// when no positional identity, --all, --tags, or --provider selected one.
func resolveLogoutIdentityTarget(sel *logoutSelection) (logoutTarget, error) {
	// `--identity` without a value asks for interactive selection; the
	// IdentityFlagSelectValue sentinel is never an identity name.
	identity := sel.identityFlag
	if identity == "" {
		identity = sel.identityEnv
	}
	// `--identity=false` disables authentication, which leaves no identity to log out of.
	if err := checkIdentityNotDisabled("logout", identity); err != nil {
		return logoutTarget{}, err
	}
	if identity == IdentityFlagSelectValue {
		return logoutTarget{kind: logoutTargetSelectIdentity}, nil
	}
	if identity != "" {
		return logoutTarget{kind: logoutTargetIdentity, identity: identity}, nil
	}

	// Interactive mode: prompt user to choose.
	return logoutTarget{kind: logoutTargetInteractive}, nil
}
