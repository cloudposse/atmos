package prompter

import (
	"io"
	"os"
	"strings"
	"testing"
)

// fakePinentryEnv selects a fake pinentry behavior when the test binary is re-executed
// as a pinentry program. This keeps the subprocess tests cross-platform.
const fakePinentryEnv = "_SAML2AWS_TEST_FAKE_PINENTRY"

const expectedPinentryInput = "SETPROMPT Security token [000000]\nGETPIN\n"

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakePinentryEnv); mode != "" {
		os.Exit(runFakePinentry(mode))
	}
	os.Exit(m.Run())
}

// runFakePinentry speaks a minimal subset of the Assuan protocol used by pinentry.
func runFakePinentry(mode string) int {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return 3
	}
	if len(os.Args) != 3 || os.Args[1] != "--ttyname" || os.Args[2] != "/dev/tty" {
		_, _ = os.Stdout.WriteString("ERR 1 unexpected arguments\n")
		return 0
	}
	if string(input) != expectedPinentryInput {
		_, _ = os.Stdout.WriteString("ERR 2 unexpected input\n")
		return 0
	}

	switch mode {
	case "ok":
		_, _ = os.Stdout.WriteString("OK Pleased to meet you\nOK\nD 424242\nOK\n")
		return 0
	case "cancel":
		_, _ = os.Stdout.WriteString("OK Pleased to meet you\nOK\nERR 83886179 Operation cancelled\n")
		return 0
	case "err-flood":
		// An error followed by more output than a pipe holds: the reader stops at the error and
		// leaves the rest unread.
		_, _ = os.Stdout.WriteString("ERR 1 failed\n")
		chunk := strings.Repeat("x", 1023) + "\n"
		for range 4096 {
			if _, err := os.Stdout.WriteString(chunk); err != nil {
				break
			}
		}
		return 0
	case "crash":
		_, _ = os.Stdout.WriteString("OK Pleased to meet you\nOK\nD 424242\n")
		return 2
	default:
		return 4
	}
}
