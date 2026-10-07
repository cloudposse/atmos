package installer

import (
	"path"
	"strings"
)

const brewCommand = "brew"

func detectHomebrew(d *detector) Installation {
	if d.system.GOOS() != "darwin" && d.system.GOOS() != "linux" {
		return Installation{}
	}
	roots := homebrewRoots(d)
	for _, root := range roots {
		if d.versionedBinary(joinRoot(root, "atmos")) {
			return d.installation(Homebrew, brewCommand)
		}
	}
	// Probe only plausible Cellar paths; unrelated installations should not spawn brew.
	if strings.Contains(d.normalize(d.executable), "/Cellar/atmos/") {
		if root, ok := d.probe(brewCommand, "--cellar"); ok && d.versionedBinary(joinRoot(strings.TrimSpace(root), "atmos")) {
			return d.installation(Homebrew, brewCommand)
		}
	}
	return Installation{}
}

func homebrewRoots(d *detector) []string {
	roots := []string{d.system.Getenv("HOMEBREW_CELLAR")}
	prefixes := []string{d.system.Getenv("HOMEBREW_PREFIX")}
	if d.system.GOOS() == "darwin" {
		prefixes = append(prefixes, "/opt/homebrew", "/usr/local")
	} else {
		prefixes = append(prefixes, "/home/linuxbrew/.linuxbrew", d.homePath(".linuxbrew"))
	}
	// The brew launcher normally lives at <prefix>/bin/brew, including custom prefixes.
	if brew, err := d.system.LookPath(brewCommand); err == nil {
		prefixes = append(prefixes, path.Dir(path.Dir(d.normalize(brew))))
	}
	for _, prefix := range prefixes {
		roots = append(roots, joinRoot(prefix, "Cellar"))
	}
	return roots
}
