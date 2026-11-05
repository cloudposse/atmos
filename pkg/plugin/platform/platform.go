// Package platform defines native plugin build targets and compatible selections.
package platform

import (
	"fmt"
	"maps"
	"regexp"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/containerd/platforms"
	specs "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	partPattern  = regexp.MustCompile(`^[a-z0-9]+$`)
	arm64Variant = regexp.MustCompile(`^v(8(\.[0-9])?|9(\.[0-5])?)$`)
)

const (
	archAMD64 = "amd64"
	archARM   = "arm"
	archARM64 = "arm64"
	goARM64   = "GOARM64"
)

// Target is a canonical OS/architecture pair with an optional Go-supported ISA variant.
type Target struct {
	OS           string
	Architecture string
	Variant      string
}

// Parse validates explicit targets without architecture aliases or implicit host defaults.
func Parse(value string) (Target, error) {
	defer perf.Track(nil, "pluginplatform.Parse")()

	parts := strings.Split(value, "/")
	if len(parts) < 2 || len(parts) > 3 || !partPattern.MatchString(parts[0]) || !partPattern.MatchString(parts[1]) {
		return Target{}, errUtils.ErrPluginInvalidConfig
	}
	target := Target{OS: parts[0], Architecture: parts[1]}
	if len(parts) == 3 {
		target.Variant = parts[2]
		if !validVariant(target.Architecture, target.Variant) {
			return Target{}, errUtils.ErrPluginInvalidConfig
		}
	}
	return target, nil
}

func validVariant(arch, variant string) bool {
	switch arch {
	case archAMD64:
		return slices.Contains([]string{"v1", "v2", "v3", "v4"}, variant)
	case archARM:
		return slices.Contains([]string{"v5", "v6", "v7"}, variant)
	case archARM64:
		return arm64Variant.MatchString(variant)
	default:
		return false
	}
}

func (t Target) comparable() specs.Platform {
	p := specs.Platform{OS: t.OS, Architecture: t.Architecture, Variant: t.Variant}
	// OCI libraries conventionally default ARM to v7; our portable unqualified
	// ARM builds explicitly use Go's lowest supported ISA, v5, with software FP.
	if p.Architecture == archARM && p.Variant == "" {
		p.Variant = "v5"
	}
	return platforms.Normalize(p)
}

// Select chooses the best compatible declared payload, never crossing OS or architecture.
// Equivalent declarations prefer the exact spelling, then lexical order for determinism.
func Select[T any](available map[string]T, requested string) (string, error) {
	defer perf.Track(nil, "pluginplatform.Select")()

	target, err := Parse(requested)
	if err != nil {
		return "", err
	}
	matcher := platforms.Only(target.comparable())
	best := ""
	var selected specs.Platform
	for _, key := range slices.Sorted(maps.Keys(available)) {
		candidate, err := Parse(key)
		if err != nil {
			return "", err
		}
		p := candidate.comparable()
		if !compatible(target, &p, matcher) {
			continue
		}
		if best == "" || matcher.Less(p, selected) || key == requested {
			best, selected = key, p
		}
	}
	if best == "" {
		return "", fmt.Errorf("%w: requested %s; available: %s", errUtils.ErrPluginUnsupportedPlatform, requested, strings.Join(slices.Sorted(maps.Keys(available)), ", "))
	}
	return best, nil
}

func compatible(target Target, candidate *specs.Platform, matcher platforms.Matcher) bool {
	return candidate.OS == target.OS && candidate.Architecture == target.Architecture && matcher.Match(*candidate)
}

// Current uses the running Atmos binary's minimum ISA, never mutable environment
// variables or a different architecture supported through OS emulation.
func Current() string {
	defer perf.Track(nil, "pluginplatform.Current")()

	var settings []debug.BuildSetting
	if info, ok := debug.ReadBuildInfo(); ok {
		settings = info.Settings
	}
	return current(runtime.GOOS, runtime.GOARCH, settings)
}

func current(goos, arch string, settings []debug.BuildSetting) string {
	variant := buildVariant(arch, settings)
	if !validVariant(arch, variant) || baselineVariant(arch, variant) {
		return goos + "/" + arch
	}
	return goos + "/" + arch + "/" + variant
}

func buildVariant(arch string, settings []debug.BuildSetting) string {
	key := map[string]string{archAMD64: "GOAMD64", archARM: "GOARM", archARM64: goARM64}[arch]
	variant := ""
	for _, setting := range settings {
		if key != "" && setting.Key == key {
			variant, _, _ = strings.Cut(setting.Value, ",")
		}
	}
	if arch == archARM {
		variant = "v" + variant
		if !validVariant(arch, variant) {
			variant = "v5"
		}
	}
	return variant
}

func baselineVariant(arch, variant string) bool {
	switch arch {
	case archAMD64:
		return variant == "v1"
	case archARM64:
		return variant == "v8" || variant == "v8.0"
	default:
		return false
	}
}

// Environment pins the compiler target and portable ABI instead of inheriting
// host tuning. Authors needing CGO must build externally with a target toolchain.
func (t Target) Environment(inherited []string) []string {
	defer perf.Track(nil, "pluginplatform.Target.Environment")()

	values := map[string]string{
		"GOOS": t.OS, "GOARCH": t.Architecture, "CGO_ENABLED": "0",
		"GOAMD64": "v1", "GOARM": "5,softfloat", goARM64: "v8.0",
		"GO386": "sse2", "GOMIPS": "softfloat", "GOMIPS64": "softfloat",
		"GOPPC64": "power8", "GORISCV64": "rva20u64",
	}
	if t.Variant != "" {
		switch t.Architecture {
		case archAMD64:
			values["GOAMD64"] = t.Variant
		case archARM:
			values["GOARM"] = strings.TrimPrefix(t.Variant, "v") + ",softfloat"
		case archARM64:
			values[goARM64] = t.Variant
			if !strings.Contains(t.Variant, ".") {
				values[goARM64] += ".0"
			}
		}
	}
	result := slices.Clone(inherited)
	for _, key := range slices.Sorted(maps.Keys(values)) {
		result = env.UpdateEnvVar(result, key, values[key])
	}
	return result
}
