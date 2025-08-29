package utils

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/charmbracelet/log"
)

func OpenUrl(URL string) error {
	if os.Getenv("GO_TEST") == "1" {
		log.Debug("Skipping browser launch in test environment")
		return nil
	}

	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", URL).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", URL).Start()
	case "darwin":
		err = exec.Command("open", URL).Start()
	default:
		err = fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	if err != nil {
		return err
	}
	return nil
}
