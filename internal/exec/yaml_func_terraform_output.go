package exec

import (
	"strings"
)

func processTerraformOutputTag(input string) any {
	part := strings.TrimPrefix(input, "!terraform.output")
	part = strings.TrimSpace(part)
	return part
}
