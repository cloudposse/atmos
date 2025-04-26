package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	log "github.com/charmbracelet/log"
	"github.com/spf13/viper"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

var (
	ErrMaxShellDepthExceeded = errors.New("ATMOS_SHLVL exceeds maximum allowed depth. Infinite recursion?")
	ErrConvertingShellLevel  = errors.New("converting ATMOS_SHLVL to number error")
	ErrBindingShellLevelEnv  = errors.New("binding ATMOS_SHLVL env var error")
)

// MaxShellDepth is the maximum number of nested shell commands that can be executed .
const MaxShellDepth = 10

func ProcessTagExec(
	input string,
) (any, error) {
	log.Info("Executing Atmos YAML function", "input", input)
	str, err := getStringAfterTag(input, AtmosYamlFuncExec)
	if err != nil {
		return nil, err
	}

	res, err := ExecuteShellAndReturnOutput(str, input, ".", nil, false)
	if err != nil {
		return nil, err
	}

	var decoded any
	if err := json.Unmarshal([]byte(res), &decoded); err != nil {
		log.Debug("Unmarshalling error", "error", err)
		decoded = res
	}

	return decoded, nil
}

// ExecuteShellAndReturnOutput runs a shell script and capture its standard output .
func ExecuteShellAndReturnOutput(
	command string,
	name string,
	dir string,
	env []string,
	dryRun bool,
) (string, error) {
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
	parser, err := syntax.NewParser().Parse(strings.NewReader(command), name)
	if err != nil {
		return err
	}

	environ := append(os.Environ(), env...)
	listEnviron := expand.ListEnviron(environ...)
	runner, err := interp.New(
		interp.Dir(dir),
		interp.Env(listEnviron),
		interp.StdIO(os.Stdin, out, os.Stderr),
	)
	if err != nil {
		return err
	}

	return runner.Run(context.TODO(), parser)
}

// getNextShellLevel increments the ATMOS_SHLVL and returns the new value or an error if maximum depth is exceeded .
func GetNextShellLevel() (int, error) {
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
			return 0, fmt.Errorf("%w: %w", ErrConvertingShellLevel, err)
		}
		shellVal = val
	}

	shellVal++

	if shellVal > MaxShellDepth {
		return 0, fmt.Errorf("%w current=%d, max=%d", ErrMaxShellDepthExceeded, shellVal, MaxShellDepth)
	}
	return shellVal, nil
}
