package local

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

// attrs is the on-disk attribute file stored beside an object when any attribute is supplied.
type attrs struct {
	ContentType string            `json:"content_type,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
}

func (a attrs) empty() bool {
	return a.ContentType == "" && len(a.Metadata) == 0 && len(a.Tags) == 0
}

func attrsPath(objectPath string) string {
	return objectPath + attrsSuffix
}

// readAttrs loads the attribute file of objectPath; a missing file yields empty attributes.
func readAttrs(objectPath string) (attrs, error) {
	var a attrs
	data, err := os.ReadFile(attrsPath(objectPath))
	if errors.Is(err, os.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return a, err
	}
	if err := json.Unmarshal(data, &a); err != nil {
		return attrs{}, err
	}
	return a, nil
}

// fromPut builds the attributes requested by a put.
func fromPut(opts blob.PutOptions) attrs {
	return attrs{ContentType: opts.ContentType, Metadata: blob.CloneMap(opts.Metadata), Tags: blob.CloneMap(opts.Tags)}
}
