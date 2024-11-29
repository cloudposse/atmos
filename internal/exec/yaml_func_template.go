package exec

import (
	"encoding/json"
	"strings"
)

func processTemplateTag(input string) any {
	part := strings.TrimPrefix(input, "!template")
	part = strings.TrimSpace(part)
	var decoded any
	if err := json.Unmarshal([]byte(part), &decoded); err != nil {
		return part
	}
	return decoded
}
