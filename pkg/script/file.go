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
	Path        string
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
	if args[0] == stdinMarker || args[0] == "--interpreter" || strings.HasPrefix(args[0], "--interpreter=") {
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
	return &File{Path: path, Interpreter: interpreter, Args: append([]string{}, args[1:]...)}, nil
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
	return &File{Path: stdinMarker, Stdin: true, Interpreter: interpreter, Args: append([]string{}, scriptArgs...)}, nil
}
