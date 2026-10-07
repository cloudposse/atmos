package installer

import "strings"

const (
	// InstallURL is the fallback for unrecognized or manually installed binaries.
	InstallURL = "https://atmos.tools/install"
	// ReleasesURL also offers native packages that do not require a repository.
	ReleasesURL = "https://github.com/cloudposse/atmos/releases"
)

// Hint contains presentation-independent upgrade guidance.
// Condition must be displayed before Command; it is never an unconditional action.
type Hint struct {
	Condition string
	Command   string
	Message   string
	URL       string
}

// UpgradeHint recommends an action without assuming package repository availability.
//
//nolint:lintroller // Pure guidance selection without I/O.
func (i Installation) UpgradeHint(latestVersion string) Hint {
	version := strings.TrimPrefix(latestVersion, "v")
	if i.Kind == Unknown || i.Kind == "" {
		return Hint{URL: InstallURL}
	}
	if i.Kind == DEB || i.Kind == RPM || i.Kind == APK {
		return i.packageHint()
	}
	if i.Kind == Nix {
		return Hint{Message: "Update Atmos in the controlling Nix configuration or input, then rebuild or reinstall.", URL: InstallURL}
	}
	if i.Manager == "" {
		return Hint{Message: "Update Atmos using " + string(i.Kind) + "; its command is not available on PATH.", URL: InstallURL}
	}
	return i.managerHint(version)
}

func (i Installation) managerHint(version string) Hint {
	hint := Hint{URL: InstallURL}
	switch i.Kind {
	case Homebrew:
		hint.Command = "brew upgrade atmos"
	case Scoop:
		hint.Command = "scoop update atmos"
		if i.Global {
			hint.Command += " --global"
		}
	case Mise:
		hint.Command = "mise install atmos@" + version
		hint.Message = "Also update the selected Atmos version in your mise configuration."
	case ASDF:
		hint.Command = "asdf install atmos " + version
		hint.Message = "Also update the Atmos version in .tool-versions."
	case Aqua:
		hint.Condition = "After updating the Atmos version in your aqua configuration"
		hint.Command = "aqua install"
	case Go:
		hint.Command = "go install github.com/cloudposse/atmos@v" + version
	case Native:
		hint.Command = "atmos version install " + version
		hint.Message = "If pinned, also update version.use, your version environment variable, or --use-version."
	}
	return hint
}

func (i Installation) packageHint() Hint {
	hint := Hint{
		Message: "Or download and install the newer ." + string(i.Kind) + " package from the releases page.",
		URL:     ReleasesURL,
	}
	if i.Manager == "" {
		hint.Message = "Download and install the newer ." + string(i.Kind) + " package from the releases page."
		return hint
	}
	hint.Condition = "If Atmos is available from your configured repositories"
	switch i.Kind {
	case DEB:
		hint.Command = "sudo apt-get update && sudo apt-get install --only-upgrade atmos"
	case RPM:
		if i.Manager == "dnf" {
			hint.Command = "sudo dnf upgrade atmos"
		} else {
			hint.Command = "sudo yum update atmos"
		}
	case APK:
		hint.Command = "apk upgrade atmos"
		hint.Condition += " (requires administrative privileges)"
	}
	return hint
}
