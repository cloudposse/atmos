// Package installer identifies the installation managing the running Atmos binary.
// Detection is advisory and never installs software or refreshes package indexes.
package installer

import (
	"context"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Kind identifies installation ownership, not the repository a package came from.
type Kind string

const (
	binaryName = "atmos"
	windowsOS  = "windows"
)

// Supported installation kinds.
const (
	Unknown  Kind = "unknown"
	Homebrew Kind = "homebrew"
	DEB      Kind = "deb"
	RPM      Kind = "rpm"
	APK      Kind = "apk"
	Scoop    Kind = "scoop"
	Mise     Kind = "mise"
	ASDF     Kind = "asdf"
	Aqua     Kind = "aqua"
	Nix      Kind = "nix"
	Go       Kind = "go"
	Native   Kind = "atmos"
)

// Installation describes ownership of the running executable.
type Installation struct {
	Kind        Kind
	Executable  string
	PackageName string
	// Manager is the available upgrade command, empty when it cannot be found.
	Manager string
	Global  bool
	// goPlatformKnown and goWindows preserve the detected shell platform for Go hints.
	goPlatformKnown bool
	goWindows       bool
}

type options struct {
	system      System
	nativeRoots []string
}

// Option configures detection dependencies or installation roots.
type Option func(*options)

// WithSystem replaces operating system operations, primarily for testing.
//
//nolint:lintroller // Pure option constructor; detection itself is tracked.
func WithSystem(system System) Option {
	return func(o *options) { o.system = system }
}

// WithNativeRoots supplies Atmos toolchain installation roots (above bin/).
//
//nolint:lintroller // Pure option constructor; detection itself is tracked.
func WithNativeRoots(roots ...string) Option {
	return func(o *options) { o.nativeRoots = append(o.nativeRoots, roots...) }
}

type detector struct {
	ctx         context.Context
	system      System
	executable  string
	home        string
	nativeRoots []string
}

type detectFunc func(*detector) Installation

// Detectors in a tier provide equally strong evidence; conflicting ownership is unknown.
var detectorRegistry = [][]detectFunc{
	{detectNative, detectHomebrew, detectMise, detectASDF, detectScoop, detectAqua, detectNix},
	{detectDEB, detectRPM, detectAPK},
	{detectGo},
}

// Detect identifies the running installation using local evidence and bounded probes.
func Detect(ctx context.Context, opts ...Option) Installation {
	defer perf.Track(nil, "installer.Detect")()

	o := options{system: osSystem{}}
	for _, opt := range opts {
		opt(&o)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	d := detector{ctx: ctx, system: o.system, nativeRoots: o.nativeRoots}
	unknown := Installation{Kind: Unknown}
	executable, err := d.system.Executable()
	if err != nil || executable == "" || ctx.Err() != nil {
		return unknown
	}
	// A failed resolution cannot safely establish ownership of the target.
	d.executable, err = d.system.EvalSymlinks(executable)
	if err != nil || d.executable == "" {
		return unknown
	}
	unknown.Executable = d.executable
	d.home, _ = d.system.UserHomeDir()
	for _, tier := range detectorRegistry {
		result, found := d.detectTier(tier)
		if ctx.Err() != nil {
			return unknown
		}
		if found {
			return result
		}
	}
	return unknown
}

// detectTier returns the strongest available evidence, rejecting conflicting ownership.
func (d *detector) detectTier(tier []detectFunc) (Installation, bool) {
	result := Installation{Kind: Unknown, Executable: d.executable}
	found := false
	for _, detect := range tier {
		if d.ctx.Err() != nil {
			break
		}
		candidate := detect(d)
		if candidate.Kind == "" {
			continue
		}
		if found && (result.Kind != candidate.Kind || result.Global != candidate.Global) {
			return Installation{Kind: Unknown, Executable: d.executable}, true
		}
		result, found = candidate, true
	}
	result.Executable = d.executable
	return result, found
}

// installation records the owner and checks whether its upgrade command is available.
func (d *detector) installation(kind Kind, manager string) Installation {
	result := Installation{Kind: kind}
	if manager != "" {
		if _, err := d.system.LookPath(manager); err == nil {
			result.Manager = manager
		}
	}
	return result
}

// probe runs a read-only manager query within the shared detection deadline.
func (d *detector) probe(name string, args ...string) (string, bool) {
	if d.ctx.Err() != nil {
		return "", false
	}
	executable, err := d.system.LookPath(name)
	if err != nil {
		return "", false
	}
	output, err := d.system.Output(d.ctx, executable, args...)
	return string(output), err == nil && d.ctx.Err() == nil
}
