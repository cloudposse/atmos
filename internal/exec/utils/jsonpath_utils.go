// https://github.com/PaesslerAG/jsonpath?tab=readme-ov-file
// https://pkg.go.dev/github.com/PaesslerAG/jsonpath
//
// `kubectl` supports JSONPath:
// https://kubernetes.io/docs/reference/kubectl/jsonpath/

package utils

import (
	"github.com/PaesslerAG/jsonpath"
)

// EvaluateJsonPath evaluate a JSONPath expression
func EvaluateJsonPath(expression string, data any) (any, error) {
	return jsonpath.Get(expression, data)
}
