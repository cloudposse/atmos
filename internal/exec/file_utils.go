package exec

import (
	"os"

	log "github.com/cloudposse/atmos/pkg/logger"
)

func removeTempDir(path string) {
	err := os.RemoveAll(path)
	if err != nil {
		log.Warn(err.Error())
	}
}
