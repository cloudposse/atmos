package utils

import (
	"fmt"
	"os/exec"
	"runtime"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/spf13/viper"
)

func OpenUrl(urlStr string) error {
	if err := viper.BindEnv("go.test", "GO_TEST"); err != nil {
		log.Trace("Failed to bind go.test environment variable", "error", err)
	}
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
