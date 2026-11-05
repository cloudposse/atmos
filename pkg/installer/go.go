package installer

import (
	"runtime"
	"strings"

	"al.essio.dev/pkg/shellescape"
)

// detectGo requires released Atmos build metadata and a matching Go binary directory.
func detectGo(d *detector) Installation {
	info, ok := d.system.BuildInfo()
	if !ok || info == nil || info.Main.Path != "github.com/cloudposse/atmos" ||
		info.Main.Version == "" || info.Main.Version == "(devel)" {
		return Installation{}
	}
	roots := goBinRoots(d)
	for _, root := range roots {
		if rel, contained := d.relative(root); contained && d.isBinary(rel) {
			result := d.installation(Go, "go")
			result.goPlatformKnown = true
			result.goWindows = d.system.GOOS() == windowsOS
			return result
		}
	}
	return Installation{}
}

// goBinRoots honors GOBIN before considering each platform-separated GOPATH entry.
func goBinRoots(d *detector) []string {
	roots := []string{d.system.Getenv("GOBIN")}
	if roots[0] == "" {
		separator := ":"
		if d.system.GOOS() == windowsOS {
			separator = ";"
		}
		for _, root := range strings.Split(d.envRoot("GOPATH", d.homePath("go")), separator) {
			roots = append(roots, joinRoot(root, "bin"))
		}
	}
	return roots
}

// goHint targets the running binary rather than Go's default first GOPATH entry.
func (i Installation) goHint(version string) Hint {
	hint := Hint{Command: "go install github.com/cloudposse/atmos@v" + version, URL: InstallURL}
	goos := runtime.GOOS
	if i.goPlatformKnown {
		goos = "linux" // POSIX path and shell rules apply to all supported Unix platforms.
		if i.goWindows {
			goos = windowsOS
		}
	}
	bin := goBinaryDirectory(i.Executable, goos)
	if bin == "" {
		return hint
	}
	if goos == windowsOS {
		hint.Condition = "In PowerShell"
		hint.Command = "$env:GOBIN = '" + strings.ReplaceAll(bin, "'", "''") + "'; " + hint.Command
		return hint
	}
	hint.Command = "GOBIN=" + shellescape.Quote(bin) + " " + hint.Command
	return hint
}

// goBinaryDirectory preserves target-platform separators and drive-root paths.
func goBinaryDirectory(executable, goos string) string {
	separators := "/"
	if goos == windowsOS {
		separators += `\`
	}
	index := strings.LastIndexAny(executable, separators)
	if index < 0 {
		return ""
	}
	if index == 0 || (goos == windowsOS && index == 2 && executable[1] == ':') {
		index++
	}
	return executable[:index]
}
