package utils

import (
	"fmt"
	"os/exec"
	"runtime"
)

func OpenUrl(URL string) error {
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
