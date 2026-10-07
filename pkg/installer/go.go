package installer

import "strings"

func detectGo(d *detector) Installation {
	info, ok := d.system.BuildInfo()
	if !ok || info == nil || info.Main.Path != "github.com/cloudposse/atmos" ||
		info.Main.Version == "" || info.Main.Version == "(devel)" {
		return Installation{}
	}
	roots := goBinRoots(d)
	for _, root := range roots {
		if rel, contained := d.relative(root); contained && d.isBinary(rel) {
			return d.installation(Go, "go")
		}
	}
	return Installation{}
}

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
