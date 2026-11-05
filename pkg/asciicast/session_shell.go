package asciicast

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/charmbracelet/x/ansi"
	"mvdan.cc/sh/v3/shell"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	// ShellInitClean skips the user's shell rc/profile files so a recording gets a predictable prompt and no plugins.
	ShellInitClean = "clean"
	// ShellInitUser runs the shell's normal interactive startup files.
	ShellInitUser = "user"

	shellNameBash = "bash"
	shellNameZsh  = "zsh"
	shellNameFish = "fish"

	// BASH_SILENCE_DEPRECATION_WARNING suppresses macOS bash's "default interactive shell is now zsh" banner.
	envBashSilenceDeprecation = "BASH_SILENCE_DEPRECATION_WARNING"
	// ZDOTDIR makes zsh read its rc files from another directory.
	envZshDotDir = "ZDOTDIR"
	// PS1 is the interactive prompt.
	envPrompt = "PS1"
	// ENV is the POSIX sh startup file variable.
	envShellStartupFile = "ENV"

	zshRCFileName   = ".zshrc"
	zshRCDirPattern = "atmos-cast-zdotdir-*"
	zshRCFileMode   = 0o600

	singleQuote = "'"
)

// zshQuietPromptArgs turn off zsh's PROMPT_SP, which otherwise prints an inverse "%" plus a full line of padding
// before the first prompt of a fresh terminal; in a recording that is visible noise.
var zshQuietPromptArgs = []string{"-o", "NO_PROMPT_SP"}

// sessionShellLaunch is the resolved startup configuration for a session shell.
type sessionShellLaunch struct {
	// Executable is the shell program to start; set by sessionShellCommand.
	Executable string
	// Args are the shell's arguments: those shell_init implies, and after sessionShellCommand also the author's own.
	Args []string
	// Env are environment overrides that win over SessionOptions.Env.
	Env map[string]string
	// cleanup removes resources created for the launch (for example a temporary ZDOTDIR); safe to call repeatedly.
	cleanup func()
	// shellName is the shell's base name, used to mark the prompt's escape sequences in that shell's syntax.
	shellName string
}

// release runs the launch cleanup, if any.
func (l *sessionShellLaunch) release() {
	if l != nil && l.cleanup != nil {
		l.cleanup()
	}
}

// environment overlays the launch environment on the caller-supplied one without mutating either map.
func (l *sessionShellLaunch) environment(base map[string]string) map[string]string {
	merged := make(map[string]string, len(base))
	maps.Copy(merged, base)
	if _, ok := merged[envBashSilenceDeprecation]; !ok {
		merged[envBashSilenceDeprecation] = "1"
	}
	maps.Copy(merged, l.Env)
	if prompt, ok := merged[envPrompt]; ok {
		merged[envPrompt] = markPromptEscapes(l.shellName, prompt)
	}
	return merged
}

// promptEscapePattern matches the ANSI escape sequences a styled prompt carries (SGR colors and the like).
var promptEscapePattern = regexp.MustCompile("\x1b\\[[0-9;:]*[A-Za-z]")

// markPromptEscapes wraps each escape sequence in a prompt with the shell's "zero-width" markers (bash \[ \],
// zsh %{ %}). Without them the line editor counts the invisible bytes as prompt width and miscalculates where
// typed text wraps, garbling redraws. A prompt that already uses the markers is left as is. For any other shell
// (sh may be bash or dash, so no marker syntax is safe) the Atmos default prompt drops its colors instead; a
// prompt the author set explicitly is never changed.
func markPromptEscapes(shellName, prompt string) string {
	var open, closing string
	switch shellName {
	case shellNameBash:
		open, closing = `\[`, `\]`
	case shellNameZsh:
		open, closing = "%{", "%}"
	default:
		if prompt == defaultSessionPrompt() {
			return ansi.Strip(prompt)
		}
		return prompt
	}
	if strings.Contains(prompt, open) {
		return prompt
	}
	return promptEscapePattern.ReplaceAllStringFunc(prompt, func(seq string) string { return open + seq + closing })
}

// shellBaseName returns the lowercase executable name of shell without a Windows ".exe" suffix.
func shellBaseName(shell string) string {
	return strings.TrimSuffix(strings.ToLower(filepath.Base(shell)), ".exe")
}

// sessionShellCommand resolves the session shell launch: the executable and the full argument list, which is the
// arguments shell_init implies (resolveSessionShellInit) followed by any the author wrote in the shell setting, so
// `shell: bash -o vi` keeps the clean startup and adds -o vi. Pair a shell command line that controls startup files
// (for example `bash --rcfile demo.rc`) with shell_init "user" to run it verbatim.
func sessionShellCommand(configured, shellInit string) (*sessionShellLaunch, error) {
	executable, authorArgs, err := splitSessionShell(configured)
	if err != nil {
		return nil, err
	}
	launch, err := resolveSessionShellInit(executable, shellInit)
	if err != nil {
		return nil, err
	}
	launch.Executable = executable
	launch.Args = slices.Concat(launch.Args, authorArgs)
	launch.shellName = shellBaseName(executable)
	return launch, nil
}

// splitSessionShell separates a shell setting into the executable and its arguments, splitting with shell word
// rules (quotes, $VARS). A value with no whitespace, or one naming an existing file (a Windows path such as
// C:\Program Files\PowerShell\7\pwsh.exe), is used as-is so backslashes and spaces in paths survive.
func splitSessionShell(configured string) (string, []string, error) {
	resolved := sessionShell(configured)
	if !strings.ContainsAny(resolved, " \t") {
		return resolved, nil, nil
	}
	if _, err := os.Stat(resolved); err == nil {
		return resolved, nil, nil
	}
	fields, err := shell.Fields(resolved, nil)
	if err != nil {
		return "", nil, fmt.Errorf("%w: parse %q: %w", errUtils.ErrInvalidCastShell, resolved, err)
	}
	if len(fields) == 0 {
		return "", nil, fmt.Errorf("%w: %q names no executable", errUtils.ErrInvalidCastShell, resolved)
	}
	return fields[0], fields[1:], nil
}

// resolveSessionShellInit maps a SessionOptions.ShellInit value to shell arguments and environment for the given
// shell: "" or "clean" skips the user's rc files, "user" keeps the shell's normal startup, and any other value is
// the path of an rc file the shell loads instead of the user's files.
func resolveSessionShellInit(shell, shellInit string) (*sessionShellLaunch, error) {
	mode := strings.TrimSpace(shellInit)
	switch {
	case mode == "" || strings.EqualFold(mode, ShellInitClean):
		return cleanShellLaunch(shell), nil
	case strings.EqualFold(mode, ShellInitUser):
		return &sessionShellLaunch{}, nil
	default:
		return rcFileShellLaunch(shell, mode)
	}
}

func cleanShellLaunch(shell string) *sessionShellLaunch {
	switch shellBaseName(shell) {
	case shellNameBash:
		return &sessionShellLaunch{Args: []string{"--noprofile", "--norc"}}
	case shellNameZsh:
		return &sessionShellLaunch{Args: append([]string{"-f"}, zshQuietPromptArgs...)}
	case shellNameFish:
		return &sessionShellLaunch{Args: []string{"--no-config"}}
	default:
		// POSIX sh, dash, and other shells read the file named by ENV for interactive sessions.
		return &sessionShellLaunch{Env: map[string]string{envShellStartupFile: ""}}
	}
}

func rcFileShellLaunch(shell, rcPath string) (*sessionShellLaunch, error) {
	name := shellBaseName(shell)
	if name != shellNameBash && name != shellNameZsh && name != shellNameFish {
		return nil, fmt.Errorf("%w: an rc file path requires bash, zsh, or fish but the session shell is %q", errUtils.ErrInvalidCastShellInit, shell)
	}
	abs, err := filepath.Abs(rcPath)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve rc file %q: %w", errUtils.ErrInvalidCastShellInit, rcPath, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("%w: rc file %q: %w", errUtils.ErrInvalidCastShellInit, abs, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%w: rc file %q is a directory", errUtils.ErrInvalidCastShellInit, abs)
	}
	switch name {
	case shellNameBash:
		return &sessionShellLaunch{Args: []string{"--noprofile", "--rcfile", abs}}, nil
	case shellNameFish:
		return &sessionShellLaunch{Args: []string{"--no-config", "-C", "source " + fishQuote(abs)}}, nil
	default:
		return zshRCFileLaunch(abs)
	}
}

// zshRCFileLaunch points zsh at a temporary ZDOTDIR whose .zshrc sources rcPath, skipping the user's own rc files and
// the global ones.
func zshRCFileLaunch(rcPath string) (*sessionShellLaunch, error) {
	dir, err := os.MkdirTemp("", zshRCDirPattern)
	if err != nil {
		return nil, fmt.Errorf("%w: create zsh startup directory: %w", errUtils.ErrInvalidCastShellInit, err)
	}
	content := "source " + posixQuote(rcPath) + "\n"
	if err := os.WriteFile(filepath.Join(dir, zshRCFileName), []byte(content), zshRCFileMode); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("%w: write zsh startup file: %w", errUtils.ErrInvalidCastShellInit, err)
	}
	var once sync.Once
	return &sessionShellLaunch{
		Args: append([]string{"-o", "NO_GLOBAL_RCS"}, zshQuietPromptArgs...),
		Env:  map[string]string{envZshDotDir: dir},
		cleanup: func() {
			once.Do(func() { _ = os.RemoveAll(dir) })
		},
	}, nil
}

// posixQuote single-quotes s for POSIX-like shells (bash, zsh).
func posixQuote(s string) string {
	return singleQuote + strings.ReplaceAll(s, singleQuote, `'\''`) + singleQuote
}

// fishQuote single-quotes s for fish, where only backslash and the single quote need escaping inside quotes.
func fishQuote(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, singleQuote, `\'`)
	return singleQuote + escaped + singleQuote
}
