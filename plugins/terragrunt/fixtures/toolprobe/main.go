// Command toolprobe stands in for a version-pinned OpenTofu in acceptance tests. It records
// its arguments in invocations.log beside itself and then runs the real executable named in
// real-tool.txt, so a test can prove which commands used the pinned copy.
//
// Installed under the name terragrunt it stands in for the Terragrunt executable instead. It
// answers --version, records the working directory, the arguments, and the TG_* environment of every
// command in terragrunt.log, and runs the engine command that follows `--` in the directory named by
// --working-dir. No real Terragrunt runs, so a test proves how Atmos invokes the executable, not how
// Terragrunt behaves.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	logMode     = 0o600
	failureCode = 127
)

func main() {
	os.Exit(run())
}

func run() int {
	self, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	directory := filepath.Dir(self)
	real, err := os.ReadFile(filepath.Join(directory, "real-tool.txt"))
	if err != nil {
		return fail(err)
	}
	if strings.TrimSuffix(filepath.Base(self), ".exe") == "terragrunt" {
		return terragrunt(directory, strings.TrimSpace(string(real)), os.Args[1:])
	}
	if err := record(filepath.Join(directory, "invocations.log"), os.Args[1:]); err != nil {
		return fail(err)
	}
	return execute(strings.TrimSpace(string(real)), "", os.Args[1:])
}

// execute runs the real tool with the process streams.
func execute(tool, directory string, arguments []string) int {
	// The target is the path a test wrote beside this fixture, never external input.
	command := exec.Command(tool, arguments...) //nolint:gosec // Test fixture runs the tool it was told to wrap.
	command.Dir = directory
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode()
		}
		return fail(err)
	}
	return 0
}

// terragrunt answers like the executable: a version, or an engine command inside a unit.
func terragrunt(directory, tool string, arguments []string) int {
	if len(arguments) == 1 && arguments[0] == "--version" {
		version, err := os.ReadFile(filepath.Join(directory, "version.txt"))
		if err != nil {
			version = []byte("v9.9.9")
		}
		_, _ = os.Stdout.WriteString("terragrunt version " + strings.TrimSpace(string(version)) + "\n")
		return 0
	}
	working, engine := split(arguments)
	if err := recordTerragrunt(filepath.Join(directory, "terragrunt.log"), working, arguments); err != nil {
		return fail(err)
	}
	if _, err := os.Stat(filepath.Join(working, ".terraform")); err != nil && len(engine) > 0 && engine[0] != "init" { //nolint:gosec // The directory comes from the test that launched this fixture.
		if code := execute(tool, working, []string{"init", "-input=false", "-no-color"}); code != 0 {
			return code
		}
	}
	return execute(tool, working, engine)
}

// split returns the --working-dir value and the engine command that follows `--`.
func split(arguments []string) (working string, engine []string) {
	engine = arguments
	for index, argument := range arguments {
		if argument == "--working-dir" && index+1 < len(arguments) {
			working = arguments[index+1]
		}
		if argument == "--" {
			return working, arguments[index+1:]
		}
	}
	return working, engine
}

func recordTerragrunt(path, working string, arguments []string) error {
	environment := map[string]string{}
	for _, entry := range os.Environ() {
		if name, value, found := strings.Cut(entry, "="); found && strings.HasPrefix(name, "TG_") {
			environment[name] = value
		}
	}
	line, err := json.Marshal(map[string]any{"args": arguments, "dir": working, "env": environment})
	if err != nil {
		return err
	}
	return appendLine(path, string(line))
}

func appendLine(path, line string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, logMode)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintln(file, line)
	return err
}

func record(path string, arguments []string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, logMode)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintln(file, strings.Join(arguments, " "))
	return err
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "toolprobe:", err)
	return failureCode
}
