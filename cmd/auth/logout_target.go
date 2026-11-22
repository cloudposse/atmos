package auth

import (
	errUtils "github.com/cloudposse/atmos/errors"
)

// logoutSelection holds the target selectors given to `atmos auth logout`.
type logoutSelection struct {
	// Args are the positional args (an identity name in args[0]).
	args []string
	// Identity is the identity from --identity or ATMOS_IDENTITY. It may be
	// IdentityFlagSelectValue when --identity was given without a value.
	identity string
	provider string
	tags     []string
	all      bool
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
)

// logoutTarget is the resolved logout target.
type logoutTarget struct {
	kind logoutTargetKind
	// Identity is set for logoutTargetIdentity.
	identity string
}

// resolveLogoutTarget decides what to log out of from the selectors, in the
// order --all, --tags, --provider, positional identity, --identity/ATMOS_IDENTITY,
// and otherwise interactive selection.
func resolveLogoutTarget(sel *logoutSelection) (logoutTarget, error) {
	// --tags is mutually exclusive with --provider/--all/--identity/positional identity —
	// it's an alternative way to select a target, not a composable cross-cutting filter here.
	if len(sel.tags) > 0 && (sel.provider != "" || sel.all || sel.identity != "" || len(sel.args) > 0) {
		return logoutTarget{}, errUtils.ErrMutuallyExclusiveFlags
	}

	switch {
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
	// `--identity` without a value asks for interactive selection; the
	// IdentityFlagSelectValue sentinel is never an identity name.
	if sel.identity != "" && sel.identity != IdentityFlagSelectValue {
		return logoutTarget{kind: logoutTargetIdentity, identity: sel.identity}, nil
	}

	// Interactive mode: prompt user to choose.
	return logoutTarget{kind: logoutTargetInteractive}, nil
}
