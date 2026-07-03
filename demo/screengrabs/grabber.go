package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/asciicast"
)

const (
	artifactsDir     = "artifacts"
	castFilePerm     = 0o600
	artifactsDirPerm = 0o755
)

var (
	errMissingDependencies = errors.New("missing required dependencies")
	errCommandFailed       = errors.New("command failed")
)

// requiredTools are the binaries invoked by the manifest commands themselves.
// The pipeline no longer needs aha: HTML is rendered natively by pkg/asciicast.
var requiredTools = []string{"atmos", "bat", "tree", "terraform"}

// recordingEnv forces color output and disables paging for recorded commands.
var recordingEnv = map[string]string{
	"TERM":              "xterm-256color",
	"ATMOS_FORCE_COLOR": "true",
	"FORCE_COLOR":       "1",
	"CLICOLOR_FORCE":    "1",
	"LESS":              "-X",
	"ATMOS_PAGER":       "false",
}

// unsupportedCommandPattern detects help output from an older released Atmos
// that does not know a command yet; such manifest entries are skipped instead
// of failing the whole run.
var unsupportedCommandPattern = regexp.MustCompile(`(?i)unknown command|unknown subcommand|unrecognized command|unsupported command|invalid command`)

func run(manifest string) error {
	if err := checkDependencies(); err != nil {
		return err
	}
	commands, err := readManifest(manifest)
	if err != nil {
		return err
	}
	demo := strings.TrimSuffix(filepath.Base(manifest), ".txt")
	env := commandEnv()

	var skipped []string
	for _, command := range commands {
		wasSkipped, err := record(demo, command, env)
		if err != nil {
			return fmt.Errorf("screengrab %q: %w", command, err)
		}
		if wasSkipped {
			skipped = append(skipped, command)
		}
	}
	reportSkipped(skipped)
	return nil
}

func checkDependencies() error {
	var missing []string
	for _, tool := range requiredTools {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s (install them or use `make -C demo/screengrabs docker-all`)",
			errMissingDependencies, strings.Join(missing, ", "))
	}
	return nil
}

func readManifest(path string) ([]string, error) {
	content, err := os.ReadFile(path) //nolint:gosec // Local build tool reading a caller-supplied manifest path.
	if err != nil {
		return nil, err
	}
	var commands []string
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		commands = append(commands, line)
	}
	return commands, nil
}

func commandEnv() []string {
	env := os.Environ()
	for key, value := range recordingEnv {
		env = append(env, key+"="+value)
	}
	return env
}

// record captures one manifest command and renders its .html and .ascii
// artifacts. It returns true when the command was skipped because the
// installed Atmos does not support it yet.
func record(demo, command string, env []string) (bool, error) {
	slug := commandSlug(command)
	fmt.Fprintf(os.Stdout, "Screengrabbing %s → %s\n", command, filepath.Join(artifactsDir, slug+".html")) //nolint:gosec // Terminal progress output for a local build tool, not a web response.

	tmpDir, err := os.MkdirTemp("", "screengrab-*")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	castPath := filepath.Join(tmpDir, "raw.cast")
	result, err := asciicast.ExecRecord(context.Background(), &asciicast.ExecOptions{
		Command: strings.Fields(command),
		Dir:     commandDir(demo, command),
		Env:     env,
		Path:    castPath,
	})
	if err != nil {
		return false, err
	}

	output, err := castOutput(castPath)
	if err != nil {
		return false, err
	}
	if result.ExitCode != 0 {
		if isAtmosCommand(command) && unsupportedCommandPattern.MatchString(output) {
			fmt.Fprintf(os.Stdout, "Skipping unavailable Atmos command: %s\n", command) //nolint:gosec // Terminal progress output for a local build tool, not a web response.
			return true, nil
		}
		fmt.Fprint(os.Stderr, output)
		return false, fmt.Errorf("%w: exit code %d", errCommandFailed, result.ExitCode)
	}

	filteredCast := filepath.Join(tmpDir, "filtered.cast")
	if err := writeFilteredCast(castPath, filteredCast, filterNoise(output)); err != nil {
		return false, err
	}
	return false, renderArtifacts(filteredCast, slug)
}

// commandDir returns the working directory for a manifest command: shell
// scripts run from the screengrabs directory, everything else runs inside the
// demo's example project.
func commandDir(demo, command string) string {
	first := strings.Fields(command)[0]
	if strings.HasSuffix(first, ".sh") {
		return ""
	}
	return filepath.Join("..", "..", "examples", demo)
}

func isAtmosCommand(command string) bool {
	return command == "atmos" || strings.HasPrefix(command, "atmos ")
}

// castOutput concatenates a recording's output and error events.
func castOutput(castPath string) (string, error) {
	_, events, err := asciicast.ReadEvents(castPath)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, event := range events {
		if event.Stream == "o" || event.Stream == "e" {
			sb.WriteString(event.Data)
		}
	}
	return sb.String(), nil
}

// writeFilteredCast writes a single-event cast that reuses the original
// recording's header but replaces the output with noise-filtered content.
func writeFilteredCast(originalPath, outputPath, content string) error {
	header, _, err := asciicast.ReadEvents(originalPath)
	if err != nil {
		return err
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return err
	}
	eventJSON, err := json.Marshal([]any{0.0, "o", content})
	if err != nil {
		return err
	}
	data := string(headerJSON) + "\n" + string(eventJSON) + "\n"
	return os.WriteFile(outputPath, []byte(data), castFilePerm)
}

func renderArtifacts(castPath, slug string) error {
	htmlPath := filepath.Join(artifactsDir, slug+".html")
	asciiPath := filepath.Join(artifactsDir, slug+".ascii")
	if err := os.MkdirAll(filepath.Dir(htmlPath), artifactsDirPerm); err != nil { //nolint:gosec // Local build tool writing artifacts derived from the manifest.
		return err
	}
	// Regeneration overwrites previous artifacts.
	for _, path := range []string{htmlPath, asciiPath} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) { //nolint:gosec // Local build tool removing its own artifacts.
			return err
		}
	}
	return asciicast.Render(castPath, &asciicast.RenderOptions{HTML: htmlPath, ASCII: asciiPath})
}

func reportSkipped(skipped []string) {
	if len(skipped) == 0 {
		return
	}
	fmt.Fprintf(os.Stdout, "\nSkipped %d unreleased Atmos commands:\n", len(skipped)) //nolint:gosec // Terminal summary output for a local build tool, not a web response.
	for _, command := range skipped {
		fmt.Fprintf(os.Stdout, "  - %s\n", command) //nolint:gosec // Terminal summary output for a local build tool, not a web response.
	}
}
