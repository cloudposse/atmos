package exec

import u "github.com/cloudposse/atmos/pkg/utils"

func LogInfo(message string) {
	u.PrintInfo(message)
}

func LogError(err error) {
	u.PrintError(err)
}
