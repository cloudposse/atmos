package utils

import "fmt"

// NotifyDeprecatedField logs a deprecation warning for a configuration field
func NotifyDeprecatedField(oldPath string, newPath interface{}) {
	var message string
	if newPath != nil {
		message = fmt.Sprintf("`%s` is deprecated; use `%s` instead", oldPath, newPath)
	} else {
		message = fmt.Sprintf("`%s` is deprecated", oldPath)
	}
	LogWarning(message)
}