package tape

import (
	"bufio"
	"os"
	"regexp"
	"strings"
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

// ParseCommands extracts executable commands from a VHS tape file.
// It finds Type directives followed by Enter and returns them as commands.
// Sleep directives between Type and Enter are allowed.
// Comments (lines starting with #) are marked but included for context.
func ParseCommands(tapePath string) ([]Command, error) {
	file, err := os.Open(tapePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var commands []Command
	var pendingType *Command

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Check for Type directive.
		if matches := typeRegex.FindStringSubmatch(line); matches != nil {
			text := matches[1]
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
			commands = append(commands, *pendingType)
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
		return nil, err
	}

	return commands, nil
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
