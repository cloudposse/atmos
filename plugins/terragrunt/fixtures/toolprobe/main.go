// Command toolprobe stands in for a version-pinned OpenTofu in acceptance tests. It records
// its arguments in invocations.log beside itself and then runs the real executable named in
// real-tool.txt, so a test can prove which commands used the pinned copy.
package main

import (
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
	if err := record(filepath.Join(directory, "invocations.log"), os.Args[1:]); err != nil {
		return fail(err)
	}
	// The target is the path a test wrote beside this fixture, never external input.
	command := exec.Command(strings.TrimSpace(string(real)), os.Args[1:]...) //nolint:gosec // Test fixture runs the tool it was told to wrap.
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
