package registry

import (
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Platform identifies a single (GOOS, GOARCH) tuple that a tool advertises support for.
// This is what the toolchain lockfile records per-tool so that a lockfile generated on
// one machine remains usable on every advertised platform (the aqua model for reproducibility).
type Platform struct {
	GOOS   string
	GOARCH string
}

// String renders the platform as "<goos>_<goarch>" — the canonical key used in
// `toolchain.lock.yaml` (chosen over the registry's "goos/goarch" form because slashes
// don't play nicely as YAML map keys in some editors).
func (p Platform) String() string {
	defer perf.Track(nil, "registry.Platform.String")()

	return p.GOOS + "_" + p.GOARCH
}

// commonPlatforms is the default platform set we expand to when a tool advertises no
// platform constraints (i.e. empty SupportedEnvs and no Overrides). Matches aqua's
// default support matrix.
var commonPlatforms = []Platform{
	{GOOS: "darwin", GOARCH: "amd64"},
	{GOOS: "darwin", GOARCH: "arm64"},
	{GOOS: "linux", GOARCH: "amd64"},
	{GOOS: "linux", GOARCH: "arm64"},
	{GOOS: "windows", GOARCH: "amd64"},
	{GOOS: "windows", GOARCH: "arm64"},
}

// knownGOOS / knownGOARCH gate the "OS-only" / "arch-only" expansion in supported_envs.
// Centralized here so adding a new GOOS/GOARCH to expand requires touching one place.
var (
	knownGOOS = []string{"darwin", "linux", "windows", "freebsd", "openbsd", "netbsd"}

	knownGOARCH = map[string]bool{
		"amd64": true, "arm64": true, "386": true, "arm": true,
		"ppc64": true, "ppc64le": true, "mips": true, "mipsle": true,
		"mips64": true, "s390x": true, "riscv64": true,
	}
)

// SupportedPlatforms returns every (GOOS, GOARCH) tuple this tool advertises support for,
// de-duplicated and sorted. Used by `atmos toolchain lock` to know which platforms to
// fetch checksums for.
//
// Resolution rules (aqua-compatible):
//   - Entry "all" → expand to commonPlatforms (and short-circuit).
//   - Entry "<goos>/<goarch>" → exact match.
//   - Entry "<goos>" (e.g. "darwin") → fan out to every common arch for that OS.
//   - Entry "<goarch>" (e.g. "amd64") → fan out to every common OS for that arch.
//   - Every Override with explicit goos+goarch is included.
//   - Override.Envs entries follow the same rules as supported_envs.
//   - Empty SupportedEnvs AND empty Overrides → return commonPlatforms (tool advertises
//     no constraints, so we treat it as portable).
//
// The result is always non-empty for a valid registry entry; an empty result indicates
// every advertised env was malformed (caller can treat as "unknown" and fall back to
// the current platform only).
func (t *Tool) SupportedPlatforms() []Platform {
	defer perf.Track(nil, "registry.Tool.SupportedPlatforms")()

	set := make(map[Platform]struct{})

	for _, env := range t.SupportedEnvs {
		expandEnv(env, set)
	}

	for i := range t.Overrides {
		ov := &t.Overrides[i]
		// Explicit goos/goarch on the override always counts, even if SupportedEnvs is empty.
		// This matches aqua: an `overrides:` block for a platform is implicit support.
		if ov.GOOS != "" && ov.GOARCH != "" {
			set[Platform{GOOS: ov.GOOS, GOARCH: ov.GOARCH}] = struct{}{}
		}
		for _, env := range ov.Envs {
			expandEnv(env, set)
		}
	}

	// If the tool advertises no constraints anywhere, treat it as portable — return the
	// common set so a lockfile entry exists for every platform CI might run on.
	if len(set) == 0 {
		return append([]Platform(nil), commonPlatforms...)
	}

	out := make([]Platform, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GOOS != out[j].GOOS {
			return out[i].GOOS < out[j].GOOS
		}
		return out[i].GOARCH < out[j].GOARCH
	})
	return out
}

// expandEnv resolves a single supported_envs entry into one or more Platform values
// and merges them into set. Unknown / malformed entries are silently dropped — callers
// see them as missing platforms rather than parse errors, matching aqua's tolerant behavior.
func expandEnv(env string, set map[Platform]struct{}) {
	env = strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		return
	}
	if env == "all" {
		addCommonPlatforms(set)
		return
	}
	if strings.Contains(env, "/") {
		addExplicitPlatform(env, set)
		return
	}
	if knownGOARCH[env] {
		addByArch(env, set)
		return
	}
	addByOS(env, set)
}

// addCommonPlatforms merges every entry in commonPlatforms into set.
func addCommonPlatforms(set map[Platform]struct{}) {
	for _, p := range commonPlatforms {
		set[p] = struct{}{}
	}
}

// addExplicitPlatform parses "<goos>/<goarch>" and adds it to set. Tolerant: empty
// halves are dropped, anything else (even unknown OS/arch) is preserved verbatim.
func addExplicitPlatform(env string, set map[Platform]struct{}) {
	parts := strings.SplitN(env, "/", 2)
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		set[Platform{GOOS: parts[0], GOARCH: parts[1]}] = struct{}{}
	}
}

// addByArch fans out an arch-only entry (e.g. "amd64") to every common OS for that arch.
func addByArch(arch string, set map[Platform]struct{}) {
	for _, p := range commonPlatforms {
		if p.GOARCH == arch {
			set[p] = struct{}{}
		}
	}
}

// addByOS fans out an OS-only entry (e.g. "darwin") to every common arch for that OS.
// Unknown OS names are silently dropped, matching aqua's tolerance.
func addByOS(os string, set map[Platform]struct{}) {
	for _, known := range knownGOOS {
		if os == known {
			for _, p := range commonPlatforms {
				if p.GOOS == os {
					set[p] = struct{}{}
				}
			}
			return
		}
	}
}
