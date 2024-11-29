package exec

import (
	"strings"
)

func processExecTag(input string) any {
	part := strings.TrimPrefix(input, "!exec")
	part = strings.TrimSpace(part)
	return part
}
