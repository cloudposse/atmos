package script

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	stdinMarker     = "-"
	atmosExecutable = "atmos"
)

// File describes an explicitly invoked standalone program.
type File struct {
	// Path is the absolute, symlink-resolved location of the script, or - for stdin.
	Path string
	// Invoked preserves the spelling used in usage hints.
	Invoked     string
	Args        []string
	Interpreter string
	// Stdin selects source from standard input; Path is the marker "-", not a filesystem path.
	Stdin bool
}

// DetectFile recognizes registered extensions, explicit paths with an Atmos shebang,
// and the standalone stdin marker (-), optionally preceded by --interpreter.
// It never infers script execution from terminal state or consumes script flags.
// Bare extensionless names remain CLI commands, avoiding command-name collisions.
func DetectFile(args []string) (*File, error) {
	defer perf.Track(nil, "script.DetectFile")()

	if len(args) == 0 {
		return nil, nil
	}
	if isStdinSelection(args[0]) {
		return detectStdin(args)
	}
	if strings.HasPrefix(args[0], "-") {
		return nil, nil
	}
	return detectScriptPath(args)
}

func detectScriptPath(args []string) (*File, error) {
	name := args[0]
	interpreter, registered := InterpreterForFile(name)
	if !registered && !strings.ContainsAny(name, `/\`) {
		return nil, nil
	}
	matches, err := matchesScriptFile(name, registered)
	if err != nil || !matches {
		return nil, err
	}
	path, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	if !registered {
		interpreter = shebangInterpreter()
		if interpreter == "" {
			return nil, fmt.Errorf("%w: no interpreter registered for Atmos shebang", errUtils.ErrScript)
		}
	}
	return &File{Path: path, Invoked: name, Interpreter: interpreter, Args: append([]string{}, args[1:]...)}, nil
}

// MayBeFile reports whether the first argument selects stdin, a registered extension,
// or an explicit script path. It looks only at the spelling, so callers can decide cheaply
// whether a file-system check is worthwhile.
func MayBeFile(args []string) bool {
	defer perf.Track(nil, "script.MayBeFile")()

	if len(args) == 0 {
		return false
	}
	if isStdinSelection(args[0]) {
		return true
	}
	if strings.HasPrefix(args[0], "-") {
		return false
	}
	name := args[0]
	_, registered := InterpreterForFile(name)
	return registered || strings.ContainsAny(name, `/\`)
}

// SplitGlobalFlags separates the leading Atmos global flags from the rest of the arguments, so a
// script path may follow them: `atmos --chdir=dir ./tool arg`. The takesValue callback reports
// whether a flag name (long without dashes, or one-letter shorthand) consumes the next argument
// when it is written without `=`, and found=false for flags Atmos does not define. A leading `--`
// ends the global flags. An unknown flag means the arguments are not a script invocation, so
// the arguments come back as rest with ok=false.
func SplitGlobalFlags(args []string, takesValue func(name string, short bool) (takes, found bool)) (globals, rest []string, ok bool) {
	defer perf.Track(nil, "script.SplitGlobalFlags")()

	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--":
			return args[:index], args[index+1:], true
		case !strings.HasPrefix(arg, "-") || isStdinSelection(arg):
			return args[:index], args[index:], true
		}
		consumed, known := globalFlagWidth(args, index, takesValue)
		if !known {
			return nil, args, false
		}
		index += consumed
	}
	return args, nil, true
}

// globalFlagWidth returns how many arguments after args[index] belong to the global flag at
// args[index]. The known result is false when the flag is not a global flag or lacks its value.
func globalFlagWidth(args []string, index int, takesValue func(string, bool) (bool, bool)) (extra int, known bool) {
	arg := args[index]
	if strings.HasPrefix(arg, "--") {
		name, _, hasValue := strings.Cut(arg[2:], "=")
		takes, found := takesValue(name, false)
		if !found {
			return 0, false
		}
		return nextWord(args, index, takes && !hasValue)
	}
	short := arg[1:2]
	takes, found := takesValue(short, true)
	if !found {
		return 0, false
	}
	// -Cvalue and -C=value carry the value in the same word.
	return nextWord(args, index, takes && len(arg) == 2)
}

func nextWord(args []string, index int, needed bool) (extra int, known bool) {
	if !needed {
		return 0, true
	}
	if index+1 >= len(args) {
		return 0, false
	}
	return 1, true
}

func matchesScriptFile(name string, registered bool) (bool, error) {
	info, err := os.Stat(name)
	if err != nil {
		if !registered {
			return false, nil
		}
		return false, fmt.Errorf("%w: cannot read script %q: %w", errUtils.ErrScript, name, err)
	}
	if !info.Mode().IsRegular() {
		if !registered {
			return false, nil
		}
		return false, fmt.Errorf("%w: script %q must be a regular file", errUtils.ErrScript, name)
	}
	if registered {
		return true, nil
	}
	return hasAtmosShebang(name)
}

func hasAtmosShebang(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("%w: open script: %w", errUtils.ErrScript, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return false, scanner.Err()
	}
	line := scanner.Text()
	if !strings.HasPrefix(line, "#!") {
		return false, nil
	}
	parts := strings.Fields(strings.TrimPrefix(line, "#!"))
	if len(parts) == 1 {
		return filepath.Base(parts[0]) == atmosExecutable, nil
	}
	if len(parts) == 2 {
		return filepath.Base(parts[0]) == "env" && parts[1] == atmosExecutable, nil
	}
	return len(parts) == 3 && filepath.Base(parts[0]) == "env" && parts[1] == "-S" && parts[2] == atmosExecutable, nil
}

// detectStdin parses only the host prefix. After -, flags belong to the script.
func detectStdin(args []string) (*File, error) {
	command := &cobra.Command{Use: atmosExecutable}
	parser := flags.NewStandardParser(flags.WithStringFlag("interpreter", "", shebangInterpreter(), "Embedded interpreter for stdin source"))
	parser.RegisterFlags(command)
	command.Flags().SetInterspersed(false)
	if err := command.ParseFlags(args); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrScript, err)
	}
	remaining := command.Flags().Args()
	if len(remaining) == 0 || remaining[0] != stdinMarker {
		return nil, fmt.Errorf("%w: --interpreter requires '-' to read a script from stdin", errUtils.ErrScript)
	}
	interpreter, err := command.Flags().GetString("interpreter")
	if err != nil {
		return nil, err
	}
	interpreter = strings.TrimSpace(interpreter)
	if _, ok := Get(interpreter); !ok {
		return nil, fmt.Errorf("%w: embedded interpreter %q is unavailable", errUtils.ErrScript, interpreter)
	}
	scriptArgs := remaining[1:]
	if len(scriptArgs) > 0 && scriptArgs[0] == "--" {
		scriptArgs = scriptArgs[1:]
	}
	return &File{Path: stdinMarker, Invoked: "atmos -", Stdin: true, Interpreter: interpreter, Args: append([]string{}, scriptArgs...)}, nil
}

func isStdinSelection(arg string) bool {
	return arg == stdinMarker || arg == "--interpreter" || strings.HasPrefix(arg, "--interpreter=")
}
