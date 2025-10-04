package utils

import (
	"fmt"

	log "github.com/charmbracelet/log"
)

// NotifyDeprecatedField logs a deprecation warning for a configuration field.
func NotifyDeprecatedField(oldPath string, newPath interface{}) {
	var message string
	if newPath != nil {
		message = fmt.Sprintf("`%s` is deprecated; use `%v` instead.", oldPath, newPath)
	} else {
		message = fmt.Sprintf("`%s` is deprecated.", oldPath)
	}
	log.Warn(message)
}
