package utils

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/viper"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	ErrMaxShellDepthExceeded = errors.New("ATMOS_SHLVL exceeds maximum allowed depth. Infinite recursion?")
	ErrConvertingShellLevel  = errors.New("converting ATMOS_SHLVL to number error")
	ErrBindingShellLevelEnv  = errors.New("binding ATMOS_SHLVL env var error")
)

// MaxShellDepth is the maximum number of nested shell commands that can be executed .
const MaxShellDepth = 10

// ExecuteShellAndReturnOutput runs a shell script and capture its standard output .
func ExecuteShellAndReturnOutput(
	command string,
	name string,
	dir string,
	env []string,
	dryRun bool,
) (string, error) {
	defer perf.Track(nil, "utils.ExecuteShellAndReturnOutput")()

	var b bytes.Buffer

	newShellLevel, err := GetNextShellLevel()
	if err != nil {
		return "", err
	}
	env = append(env, fmt.Sprintf("ATMOS_SHLVL=%d", newShellLevel))

	log.Debug("Executing", "command", command)

	if dryRun {
		return "", nil
	}

	err = ShellRunner(command, name, dir, env, &b)
	if err != nil {
		return "", err
	}

	return b.String(), nil
}

// ShellRunner uses mvdan.cc/sh/v3's parser and interpreter to run a shell script and divert its stdout .
func ShellRunner(command string, name string, dir string, env []string, out io.Writer) error {
	defer perf.Track(nil, "utils.ShellRunner")()

	parser, err := syntax.NewParser().Parse(strings.NewReader(command), name)
	if err != nil {
		return err
	}

	// Use provided environment directly to preserve PATH modifications
	// If no environment provided, fall back to current process environment
	environ := env
	if len(environ) == 0 {
		environ = os.Environ()
	}

	listEnviron := expand.ListEnviron(environ...)
	runner, err := interp.New(
		interp.Dir(dir),
		interp.Env(listEnviron),
		interp.StdIO(os.Stdin, out, os.Stderr),
	)
	if err != nil {
		return err
	}

	err = runner.Run(context.Background(), parser)
	if err != nil {
		// Check if the error is an interp.ExitStatus and preserve the exit code.
		// Return a typed error that preserves the exit code.
		// main.go will check for this type and exit with the correct code.
		var exitErr interp.ExitStatus
		if errors.As(err, &exitErr) {
			return errUtils.ExitCodeError{Code: int(exitErr)}
		}
		return err
	}

	return nil
}

// extractCommandNamesFromShell parses a POSIX shell command string and returns all
// simple-command executable names found in the AST (including those nested in pipes,
// subshells, and command substitutions).
// The second return value indicates whether every command name was a static literal;
// dynamic names such as $VAR or "$(expr)" produce a false value.
func extractCommandNamesFromShell(cmd string) ([]string, bool, error) {
	f, err := syntax.NewParser().Parse(strings.NewReader(cmd), "")
	if err != nil {
		return nil, false, err
	}

	var names []string
	allLiteral := true

	syntax.Walk(f, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 || len(call.Args[0].Parts) == 0 {
			return true
		}
		lit, ok := call.Args[0].Parts[0].(*syntax.Lit)
		if !ok {
			allLiteral = false
			return true
		}
		names = append(names, lit.Value)
		return true
	})

	return names, allLiteral, nil
}

// GetNextShellLevel increments the ATMOS_SHLVL and returns the new value or an error if maximum depth is exceeded.
func GetNextShellLevel() (int, error) {
	defer perf.Track(nil, "utils.GetNextShellLevel")()

	// Create a new viper instance for this operation
	v := viper.New()
	if err := v.BindEnv("atmos_shell_level", "ATMOS_SHLVL"); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrBindingShellLevelEnv, err)
	}

	shellVal := 0
	atmosShellLvl := v.GetString("atmos_shell_level")
	if atmosShellLvl != "" {
		val, err := strconv.Atoi(atmosShellLvl)
		if err != nil {
			return 0, fmt.Errorf("%w: %s", ErrConvertingShellLevel, err)
		}
		shellVal = val
	}

	shellVal++

	if shellVal > MaxShellDepth {
		return 0, fmt.Errorf("%w current=%d, max=%d", ErrMaxShellDepthExceeded, shellVal, MaxShellDepth)
	}
	return shellVal, nil
}
