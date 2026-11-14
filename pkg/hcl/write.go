package hcl

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/hashicorp/hcl/v2/hclwrite"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/cloudposse/atmos/pkg/perf"
)

// EncodeAttributes renders the top-level keys of data as HCL attributes, the
// format Terraform expects in .tfvars and -backend-config files. Nested maps
// become object values and lists, including lists of maps, become tuples.
// Keys are written in sorted order.
func EncodeAttributes(data any) ([]byte, error) {
	defer perf.Track(nil, "hcl.EncodeAttributes")()

	j, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf(errWrapFmt, ErrHCLEncodeFailed, err)
	}

	ty, err := ctyjson.ImpliedType(j)
	if err != nil {
		return nil, fmt.Errorf(errWrapFmt, ErrHCLEncodeFailed, err)
	}
	if !ty.IsObjectType() {
		return nil, fmt.Errorf("%w: HCL attributes can only be generated from a map, got %s", ErrHCLEncodeFailed, ty.FriendlyName())
	}

	value, err := ctyjson.Unmarshal(j, ty)
	if err != nil {
		return nil, fmt.Errorf(errWrapFmt, ErrHCLEncodeFailed, err)
	}

	names := make([]string, 0, len(ty.AttributeTypes()))
	for name := range ty.AttributeTypes() {
		names = append(names, name)
	}
	sort.Strings(names)

	file := hclwrite.NewEmptyFile()
	body := file.Body()
	for _, name := range names {
		body.SetAttributeValue(name, value.GetAttr(name))
	}

	return hclwrite.Format(file.Bytes()), nil
}

// WriteAttributesFile encodes data with EncodeAttributes and writes it to
// path, replacing any previous content.
func WriteAttributesFile(path string, data any, mode os.FileMode) error {
	defer perf.Track(nil, "hcl.WriteAttributesFile")()

	content, err := EncodeAttributes(data)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, content, mode); err != nil {
		return fmt.Errorf(errWrapFmt, ErrHCLWriteFile, err)
	}
	return nil
}
