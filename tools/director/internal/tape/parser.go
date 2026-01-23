package tape

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Command represents an extracted command from a VHS tape file.
type Command struct {
	Line      int    // Line number in tape file (1-indexed)
	Text      string // The command text
	IsComment bool   // True if this is a comment line (starts with #)
}

// typeRegex matches Type "..." or Type `...` directives.
var typeRegex = regexp.MustCompile("^\\s*Type\\s+[\"'`](.+)[\"'`]\\s*$")

// sleepRegex matches Sleep directives (Sleep 1s, Sleep 500ms, etc.).
var sleepRegex = regexp.MustCompile(`^\s*Sleep\s+[\d.]+(?:ms|s)\s*$`)

// envRegex matches Env directives: Env NAME "value" or Env NAME 'value'.
var envRegex = regexp.MustCompile(`^\s*Env\s+(\w+)\s+["'](.+)["']\s*$`)

// TapeResult contains the parsed commands and environment variables from a tape file.
type TapeResult struct {
	Commands []Command
	EnvVars  map[string]string
}

// ParseCommands extracts executable commands from a VHS tape file.
// It finds Type directives followed by Enter and returns them as commands.
// Sleep directives between Type and Enter are allowed.
// Comments (lines starting with #) are marked but included for context.
func ParseCommands(tapePath string) ([]Command, error) {
	result, err := ParseTape(tapePath)
	if err != nil {
		return nil, err
	}
	return result.Commands, nil
}

// ParseTape extracts commands and environment variables from a VHS tape file.
// This is the full parser that returns both commands and Env directives.
func ParseTape(tapePath string) (*TapeResult, error) {
	defer perf.Track(nil, "tape.ParseTape")()

	file, err := os.Open(tapePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open tape file %s: %w", tapePath, err)
	}
	defer file.Close()

	result := &TapeResult{
		EnvVars: make(map[string]string),
	}
	var pendingType *Command

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Check for Env directive.
		if matches := envRegex.FindStringSubmatch(line); matches != nil {
			name := matches[1]
			value := matches[2]
			result.EnvVars[name] = value
			continue
		}

		// Check for Type directive.
		if matches := typeRegex.FindStringSubmatch(line); matches != nil {
			text := matches[1]
			// Unescape VHS-specific escape sequences.
			// VHS uses \$ to type a literal $ (preventing VHS variable expansion).
			// When we execute via bash, we want bash to expand $VAR, so unescape.
			text = strings.ReplaceAll(text, `\$`, "$")
			isComment := strings.HasPrefix(strings.TrimSpace(text), "#")

			pendingType = &Command{
				Line:      lineNum,
				Text:      text,
				IsComment: isComment,
			}
			continue
		}

		// Check for Enter directive.
		if trimmed == "Enter" && pendingType != nil {
			result.Commands = append(result.Commands, *pendingType)
			pendingType = nil
			continue
		}

		// Allow Sleep between Type and Enter.
		if sleepRegex.MatchString(line) {
			continue
		}

		// Empty lines and comments don't clear pending type.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Other directives clear pending type (Type without Enter is not a command).
		if pendingType != nil {
			pendingType = nil
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read tape file %s: %w", tapePath, err)
	}

	return result, nil
}

// FilterExecutable returns only commands that should be executed (non-comments).
func FilterExecutable(commands []Command) []Command {
	var executable []Command
	for _, cmd := range commands {
		if !cmd.IsComment {
			executable = append(executable, cmd)
		}
	}
	return executable
}
