package script

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// File describes an explicitly invoked standalone Starlark program.
type File struct {
	Path string
	Args []string
}

// DetectFile recognizes .star paths and explicit paths with an Atmos shebang.
// It never infers script execution from terminal state or consumes script flags.
// Bare extensionless names remain CLI commands, avoiding command-name collisions.
func DetectFile(args []string) (*File, error) {
	defer perf.Track(nil, "script.DetectFile")()

	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return nil, nil
	}
	name := args[0]
	star := filepath.Ext(name) == ".star"
	if !star && !strings.ContainsAny(name, `/\`) {
		return nil, nil
	}
	matches, err := matchesScriptFile(name, star)
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
	return &File{Path: path, Args: append([]string{}, args[1:]...)}, nil
}

func matchesScriptFile(name string, star bool) (bool, error) {
	info, err := os.Stat(name)
	if err != nil {
		if !star {
			return false, nil
		}
		return false, fmt.Errorf("%w: cannot read script %q: %w", errUtils.ErrStarlark, name, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%w: script %q must be a regular file", errUtils.ErrStarlark, name)
	}
	if star {
		return true, nil
	}
	return hasAtmosShebang(name)
}

func hasAtmosShebang(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("%w: open script: %w", errUtils.ErrStarlark, err)
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
		return filepath.Base(parts[0]) == "atmos", nil
	}
	if len(parts) == 2 {
		return filepath.Base(parts[0]) == "env" && parts[1] == "atmos", nil
	}
	return len(parts) == 3 && filepath.Base(parts[0]) == "env" && parts[1] == "-S" && parts[2] == "atmos", nil
}
