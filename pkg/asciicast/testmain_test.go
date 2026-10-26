package asciicast

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	asciicastHelperEnv        = "_ATMOS_ASCIICAST_HELPER"
	asciicastSessionHelperEnv = "_ATMOS_ASCIICAST_SESSION_HELPER"
	asciicastExecHelperEnv    = "_ATMOS_ASCIICAST_EXEC_HELPER"
	// Env var that makes the session helper print sessionHelperBanner once it starts reading input.
	asciicastSessionBannerEnv = "_ATMOS_ASCIICAST_SESSION_BANNER"
	sessionHelperBanner       = "helper-up"
)

func TestMain(m *testing.M) {
	if os.Getenv(asciicastHelperEnv) == "1" {
		runAsciicastToolHelper()
		os.Exit(0)
	}
	if os.Getenv(asciicastSessionHelperEnv) == "1" {
		runAsciicastSessionHelper()
		os.Exit(0)
	}
	if mode := os.Getenv(asciicastExecHelperEnv); mode != "" {
		runAsciicastExecHelper(mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runAsciicastExecHelper lets exec tests use the test binary itself as a
// cross-platform recorded command.
func runAsciicastExecHelper(mode string) {
	switch mode {
	case "ok":
		_, _ = os.Stdout.WriteString("stdout line\n")
		_, _ = os.Stderr.WriteString("stderr line\n")
	case "fail":
		_, _ = os.Stdout.WriteString("about to fail\n")
		os.Exit(3)
	case "sleep":
		time.Sleep(2 * time.Second)
	}
}

func runAsciicastToolHelper() {
	switch strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") {
	case "agg":
		if len(os.Args) < 3 {
			os.Exit(2)
		}
		if err := os.WriteFile(os.Args[2], []byte("agg:"+os.Args[1]+":"+os.Args[2]), 0o644); err != nil {
			os.Exit(1)
		}
	case "ffmpeg":
		if len(os.Args) == 0 {
			os.Exit(2)
		}
		if err := os.WriteFile(os.Args[len(os.Args)-1], []byte("mp4"), 0o644); err != nil {
			os.Exit(1)
		}
	default:
		os.Exit(2)
	}
}

func runAsciicastSessionHelper() {
	// The test binary can emit terminal queries during startup that swallow
	// input typed before the helper starts reading. Tests that need their typed
	// line to reach the helper wait for this banner before typing.
	if os.Getenv(asciicastSessionBannerEnv) == "1" {
		_, _ = os.Stdout.WriteString(sessionHelperBanner + "\n")
	}
	reader := bufio.NewReader(os.Stdin)
	var line strings.Builder
	for {
		b, err := reader.ReadByte()
		if err != nil {
			// isExpectedSessionReadError (session.go) already classifies a closed
			// stdin pipe as a normal shutdown for the parent's stdout-read loop;
			// apply the same classification here. finishSession sends EOT then
			// closes the input pipe as part of ordinary teardown, and under CI
			// load that race can surface as io.ErrClosedPipe (or an "input/output
			// error" on some platforms) instead of a plain io.EOF -- exiting 1 in
			// that case turned a normal teardown race into a spurious test
			// failure ("exit status 1").
			if isExpectedSessionReadError(err) {
				return
			}
			os.Exit(1)
		}
		switch b {
		case 4:
			return
		case '\r', '\n':
			switch strings.TrimSpace(line.String()) {
			case "printf ready":
				_, _ = os.Stdout.WriteString("ready\n")
			case "print shellinit":
				env, envSet := os.LookupEnv("ENV")
				_, _ = fmt.Fprintf(os.Stdout, "shellinit env_set=%t env=%q silence=%q args=%q\n", envSet, env, os.Getenv("BASH_SILENCE_DEPRECATION_WARNING"), os.Args[1:])
			case "print context":
				cwd, _ := os.Getwd()
				_, _ = os.Stdout.WriteString("cwd=" + cwd + "\n")
				_, _ = os.Stdout.WriteString("marker=" + os.Getenv("ATMOS_CAST_SESSION_MARKER") + "\n")
			}
			line.Reset()
		default:
			_ = line.WriteByte(b)
		}
	}
}

func helperExecutableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
