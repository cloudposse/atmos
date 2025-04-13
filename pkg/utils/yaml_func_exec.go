package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	log "github.com/charmbracelet/log"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

func ProcessTagExec(
	input string,
) any {
	log.Info("Executing Atmos YAML function", "input", input)
	str, err := getStringAfterTag(input, AtmosYamlFuncExec)
	if err != nil {
		log.Fatal(err)
	}

	res, err := ExecuteShellAndReturnOutput(str, input, ".", nil, false)
	if err != nil {
		log.Fatal(err)
	}

	var decoded any
	if err = json.Unmarshal([]byte(res), &decoded); err != nil {
		return res
	}

	return decoded
}

// ExecuteShellAndReturnOutput runs a shell script and capture its standard output
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
	updatedEnv := append(env, fmt.Sprintf("ATMOS_SHLVL=%d", newShellLevel))

	LogDebug("\nExecuting command:")
	LogDebug(command)

	if dryRun {
		return "", nil
	}

	err = shellRunner(command, name, dir, updatedEnv, &b)
	if err != nil {
		return "", err
	}

	return b.String(), nil
}

// shellRunner uses mvdan.cc/sh/v3's parser and interpreter to run a shell script and divert its stdout
func shellRunner(command string, name string, dir string, env []string, out io.Writer) error {
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
