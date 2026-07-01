package asciicast

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	asciicastHelperEnv        = "_ATMOS_ASCIICAST_HELPER"
	asciicastSessionHelperEnv = "_ATMOS_ASCIICAST_SESSION_HELPER"
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
	os.Exit(m.Run())
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
	reader := bufio.NewReader(os.Stdin)
	var line strings.Builder
	for {
		b, err := reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				return
			}
			os.Exit(1)
		}
		switch b {
		case 4:
			return
		case '\r', '\n':
			if strings.TrimSpace(line.String()) == "printf ready" {
				_, _ = os.Stdout.WriteString("ready\n")
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
