# ptyrunner

A generic PTY (pseudo-terminal) wrapper for running commands in a simulated terminal environment.

## Overview

`ptyrunner` wraps any command in a pseudo-terminal (PTY), which is useful for:

- **Testing TUI applications** in CI/headless environments where no real terminal exists
- **Running interactive commands** that require a TTY (like `vim`, `less`, `top`)
- **Capturing output** from programs that behave differently when connected to a terminal vs a pipe
- **Simulating terminal interactions** in automated tests

## Platform Support

- ✅ **Linux**: Full support
- ✅ **macOS**: Full support
- ✅ **Unix/BSD**: Full support
- ❌ **Windows**: Not supported (PTYs are a Unix concept; use ConPTY or WSL instead)

## Installation

```bash
go install github.com/cloudposse/atmos/tools/ptyrunner@latest
```

Or build from source:

```bash
cd tools/ptyrunner
go build -o ptyrunner .
```

## Usage

```bash
# Basic usage
ptyrunner <command> [args...]

# Examples
ptyrunner ls -la                    # Run ls with color output
ptyrunner gotcha stream ./...       # Run gotcha test runner in TUI mode
ptyrunner vim file.txt              # Run vim (interactive editor)
ptyrunner docker run -it ubuntu bash # Run interactive Docker container
```

## How It Works

1. Creates a pseudo-terminal (PTY) pair
2. Starts the specified command attached to the PTY
3. Handles terminal resize events (SIGWINCH)
4. Sets up raw mode for proper input handling
5. Copies input/output between the real terminal and the PTY
6. Preserves the command's exit code

## Use Cases

### CI/CD Testing
Test TUI applications in headless CI environments:
```bash
ptyrunner my-tui-app --test-mode
```

### Capturing Colored Output
Many commands disable color when output is piped. Use ptyrunner to preserve colors:
```bash
ptyrunner ls --color=auto | tee output.txt
```

### Testing Interactive Tools
Automated testing of tools that require terminal interaction:
```bash
echo "some input" | ptyrunner interactive-tool
```

## Limitations

- **Windows**: PTYs are not supported. Use Windows-specific solutions like ConPTY or WSL
- **Signal Handling**: Some signals may not propagate perfectly to the child process
- **Terminal Emulation**: This is a simple PTY wrapper, not a full terminal emulator

## Related Tools

- `script` - Unix utility for recording terminal sessions
- `expect` - Tool for automating interactive applications
- `tmux`/`screen` - Terminal multiplexers with PTY management
- `gotcha` - Go test runner that uses ptyrunner for TUI testing
