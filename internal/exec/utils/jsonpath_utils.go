// https://github.com/PaesslerAG/jsonpath?tab=readme-ov-file
// https://pkg.go.dev/github.com/PaesslerAG/jsonpath
//
// `kubectl` supports JSONPath:
// https://kubernetes.io/docs/reference/kubectl/jsonpath/

package utils

import (
	"github.com/PaesslerAG/jsonpath"
)

func evaluateJsonPath(query string, data any) (any, error) {
	return jsonpath.Get(query, data)
}
