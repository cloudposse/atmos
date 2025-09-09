package utils

import (
	"fmt"
	"os/exec"
	"runtime"

	log "github.com/charmbracelet/log"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/spf13/viper"
)

func OpenUrl(urlStr string) error {
	_ = viper.BindEnv("go.test", "GO_TEST")
	if viper.GetString("go.test") == "1" {
		log.Debug("Skipping browser launch in test environment")
		return nil
	}

	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", urlStr).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", urlStr).Start()
	case "darwin":
		err = exec.Command("open", urlStr).Start()
	default:
		err = fmt.Errorf("%w: %s", errUtils.ErrUnsupportedPlatform, runtime.GOOS)
	}

	if err != nil {
		return err
	}
	return nil
}
