package schema

// ToolchainInstall is the `toolchain.install` policy. It decides which tools Atmos
// installs automatically, without an explicit `atmos toolchain install`.
//
// Each level installs everything the previous level does and more.
type ToolchainInstall string

const (
	// ToolchainInstallNever installs nothing. Explicit `dependencies.tools` and
	// `.tool-versions` tools are used only when they are already installed.
	ToolchainInstallNever ToolchainInstall = "never"
	// ToolchainInstallDeclared installs only explicit `dependencies.tools`. Workflows
	// also install every `.tool-versions` tool. Other commands inherit already-installed
	// versions selected by `.tool-versions` without installing missing defaults.
	ToolchainInstallDeclared ToolchainInstall = "declared"
	// ToolchainInstallAuto installs explicit `dependencies.tools` plus the run's own
	// executable from `.tool-versions` when it is pinned there. Other `.tool-versions`
	// tools join PATH only when already installed. Workflows install every
	// `.tool-versions` tool.
	ToolchainInstallAuto ToolchainInstall = "auto"
	// ToolchainInstallAlways installs every `.tool-versions` tool for every run.
	ToolchainInstallAlways ToolchainInstall = "always"
)

// ToolchainInstallValues lists the valid non-empty policies, least to most eager.
var ToolchainInstallValues = []ToolchainInstall{
	ToolchainInstallNever,
	ToolchainInstallDeclared,
	ToolchainInstallAuto,
	ToolchainInstallAlways,
}

// IsValid reports whether the policy is empty (unset) or one of the known values.
func (p ToolchainInstall) IsValid() bool {
	switch p {
	case "", ToolchainInstallNever, ToolchainInstallDeclared, ToolchainInstallAuto, ToolchainInstallAlways:
		return true
	default:
		return false
	}
}

// EffectiveInstall returns the configured install policy, defaulting to auto when unset.
//
// The default normally arrives through the config defaults layer, where an edition pin
// can roll it back. The fallback here covers an AtmosConfiguration built directly in Go,
// bypassing config loading entirely.
func (t *Toolchain) EffectiveInstall() ToolchainInstall {
	if t.Install == "" {
		return ToolchainInstallAuto
	}
	return t.Install
}
