// Package planout recognizes the plan output flag of `terraform plan` and `tofu plan`.
//
// Terraform and OpenTofu parse their flags with Go's flag package, which accepts one or two
// leading dashes, so `-out X`, `-out=X`, `--out X` and `--out=X` all select the saved plan file.
// Every place Atmos detects a user-supplied plan file must recognize the same four forms; one
// that only knows `-out` adds a second destination next to the user's.
package planout

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// Flag is the single-dash form Atmos itself writes.
	Flag = "-out"
	// DoubleDashFlag is the equivalent double-dash form.
	DoubleDashFlag = "--out"
)

// Arg describes one plan output flag argument.
type Arg struct {
	// Name is the flag as written, "-out" or "--out".
	Name string
	// Value is the path given in the same argument after "=", empty when Joined is false.
	Value string
	// Joined reports that the value is part of the argument ("-out=X") rather than the next one.
	Joined bool
}

// Parse reports whether arg is a plan output flag, in any of its four forms.
func Parse(arg string) (Arg, bool) {
	defer perf.Track(nil, "planout.Parse")()

	for _, name := range []string{DoubleDashFlag, Flag} {
		if arg == name {
			return Arg{Name: name}, true
		}
		if value, found := strings.CutPrefix(arg, name+"="); found {
			return Arg{Name: name, Value: value, Joined: true}, true
		}
	}
	return Arg{}, false
}

// IsFlag reports whether arg is a plan output flag, with or without a value.
func IsFlag(arg string) bool {
	defer perf.Track(nil, "planout.IsFlag")()

	_, ok := Parse(arg)
	return ok
}

// Contains reports whether args hold any plan output flag.
func Contains(args []string) bool {
	defer perf.Track(nil, "planout.Contains")()

	for _, arg := range args {
		if IsFlag(arg) {
			return true
		}
	}
	return false
}

// Value returns the path selected by the first plan output flag that carries one, or "".
// A separate-form flag takes the following argument; one at the end of args has no value.
func Value(args []string) string {
	defer perf.Track(nil, "planout.Value")()

	for i, arg := range args {
		parsed, ok := Parse(arg)
		switch {
		case !ok:
			continue
		case parsed.Joined && parsed.Value != "":
			return parsed.Value
		case !parsed.Joined && i+1 < len(args) && args[i+1] != "":
			return args[i+1]
		}
	}
	return ""
}
