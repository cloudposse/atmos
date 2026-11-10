package link

import (
	"strconv"
	"strings"
)

// Environment is what hyperlink detection can see of the terminal.
type Environment struct {
	// IsTTY reports whether the stream the link is written to is a terminal.
	IsTTY bool
	// Lookup returns an environment variable, in the manner of os.LookupEnv.
	Lookup func(name string) (string, bool)
}

// Minimum versions at which each terminal gained OSC 8 hyperlink support.
const (
	minITermMajor, minITermMinor   = 3, 1
	minVSCodeMajor, minVSCodeMinor = 1, 72
	// VTE encodes 0.50.0 as 5000.
	minVTEVersion = 5000
	// Konsole encodes 20.12.0 as 201200.
	minKonsoleVersion = 201200
)

// SupportsHyperlinks reports whether the terminal is known to render OSC 8
// hyperlinks. Terminals offer no query for this, so detection is an allowlist
// of environment variables that identify a terminal. It deliberately errs
// towards false: a miss only costs the short label, while a false positive
// would hide the URL behind a label that cannot be clicked.
func SupportsHyperlinks(env Environment) bool {
	if !env.IsTTY || env.Lookup == nil {
		return false
	}
	if env.value("TERM") == "dumb" {
		return false
	}
	// A multiplexer sits between Atmos and the real terminal and passes OSC 8
	// through only in recent, explicitly configured versions, while variables
	// inherited from the outer terminal still claim support.
	if env.isSet("TMUX") || env.isSet("STY") {
		return false
	}
	return env.termProgramSupports() || env.terminalVariableSupports()
}

// termProgramSupports checks terminals that identify themselves with TERM_PROGRAM.
func (e Environment) termProgramSupports() bool {
	version := e.value("TERM_PROGRAM_VERSION")
	switch e.value("TERM_PROGRAM") {
	case "iTerm.app":
		return versionAtLeast(version, minITermMajor, minITermMinor)
	case "vscode":
		return versionAtLeast(version, minVSCodeMajor, minVSCodeMinor)
	case "WezTerm", "ghostty":
		return true
	default:
		return false
	}
}

// terminalVariableSupports checks terminals that identify themselves with their
// own variable or TERM value.
func (e Environment) terminalVariableSupports() bool {
	switch e.value("TERM") {
	case "xterm-kitty", "xterm-ghostty":
		return true
	}
	if e.isSet("KITTY_WINDOW_ID") || e.isSet("WT_SESSION") {
		return true
	}
	return e.intAtLeast("VTE_VERSION", minVTEVersion) || e.intAtLeast("KONSOLE_VERSION", minKonsoleVersion)
}

func (e Environment) value(name string) string {
	value, _ := e.Lookup(name)
	return value
}

func (e Environment) isSet(name string) bool {
	value, ok := e.Lookup(name)
	return ok && value != ""
}

func (e Environment) intAtLeast(name string, minimum int) bool {
	value, err := strconv.Atoi(e.value(name))
	return err == nil && value >= minimum
}

// versionAtLeast reports whether a dotted version is at least major.minor.
// An unparseable version is treated as too old.
func versionAtLeast(version string, major, minor int) bool {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false
	}
	gotMajor, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	gotMinor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	if gotMajor != major {
		return gotMajor > major
	}
	return gotMinor >= minor
}
