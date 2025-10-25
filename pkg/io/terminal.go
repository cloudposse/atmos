package io

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// terminal implements the Terminal interface.
type terminal struct {
	config       *Config
	colorProfile ColorProfile
	originalTitle string
}

// newTerminal creates a new Terminal.
func newTerminal(config *Config) Terminal {
	t := &terminal{
		config: config,
	}

	// Detect color profile once at initialization
	isTTYOut := t.IsTTY(StreamOutput)
	t.colorProfile = config.DetectColorProfile(isTTYOut)

	return t
}

func (t *terminal) IsTTY(stream interface{}) bool {
	fd := t.streamToFd(stream)
	if fd < 0 {
		return false
	}
	return term.IsTerminal(fd)
}

func (t *terminal) ColorProfile() ColorProfile {
	return t.colorProfile
}

func (t *terminal) Width(stream interface{}) int {
	fd := t.streamToFd(stream)
	if fd < 0 {
		return 0
	}

	width, _, err := term.GetSize(fd)
	if err != nil {
		return 0
	}

	return width
}

func (t *terminal) Height(stream interface{}) int {
	fd := t.streamToFd(stream)
	if fd < 0 {
		return 0
	}

	_, height, err := term.GetSize(fd)
	if err != nil {
		return 0
	}

	return height
}

func (t *terminal) SetTitle(title string) {
	// Check if title setting is enabled
	if !t.config.AtmosConfig.Settings.Terminal.Title {
		return
	}

	// Only set title if stdout is a TTY
	if !t.IsTTY(StreamOutput) {
		return
	}

	// Use OSC sequence to set terminal title
	// OSC 0 ; <title> BEL
	// Works in most modern terminals
	fmt.Fprintf(os.Stderr, "\033]0;%s\007", title)
}

func (t *terminal) RestoreTitle() {
	if t.originalTitle != "" {
		t.SetTitle(t.originalTitle)
	}
}

func (t *terminal) Alert() {
	// Check if alerts are enabled
	if !t.config.AtmosConfig.Settings.Terminal.Alerts {
		return
	}

	// Only alert if stderr is a TTY
	if !t.IsTTY(StreamError) {
		return
	}

	// Emit BEL character
	fmt.Fprint(os.Stderr, "\007")
}

// streamToFd converts either Channel or StreamType to file descriptor.
// Returns -1 if the stream type is invalid.
func (t *terminal) streamToFd(stream interface{}) int {
	// Handle Channel type (new)
	if ch, ok := stream.(Channel); ok {
		switch ch {
		case DataChannel:
			return int(os.Stdout.Fd())
		case UIChannel:
			return int(os.Stderr.Fd())
		case InputChannel:
			return int(os.Stdin.Fd())
		default:
			return -1
		}
	}

	// Handle StreamType (legacy)
	if st, ok := stream.(StreamType); ok {
		switch st {
		case StreamInput:
			return int(os.Stdin.Fd())
		case StreamOutput:
			return int(os.Stdout.Fd())
		case StreamError:
			return int(os.Stderr.Fd())
		default:
			return -1
		}
	}

	return -1
}
