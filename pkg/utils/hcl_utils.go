package utils

import (
	"os"
	"strings"

	"github.com/hashicorp/hcl/hcl/ast"
	"github.com/hashicorp/hcl/hcl/printer"
	jsonParser "github.com/hashicorp/hcl/json/parser"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"github.com/cloudposse/atmos/pkg/convert"
	"github.com/cloudposse/atmos/pkg/schema"
)

// PrintAsHcl prints the provided value as HCL (HashiCorp Language) document to the console
func PrintAsHcl(data any) error {
	astree, err := ConvertToHclAst(data)
	if err != nil {
		return err
	}

	err = printer.Fprint(os.Stdout, astree)
	if err != nil {
		return err
	}

	return nil
}

// WriteToFileAsHcl converts the provided value to HCL (HashiCorp Language) and writes it to the specified file
func WriteToFileAsHcl(
	cliConfig schema.CliConfiguration,
	filePath string,
	data any,
	fileMode os.FileMode,
) error {
	astree, err := ConvertToHclAst(data)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, fileMode)
	if err != nil {
		return err
	}

	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			LogWarning(cliConfig, err.Error())
		}
	}(f)

	err = printer.Fprint(f, astree)
	if err != nil {
		return err
	}

	return nil
}

// ConvertToHclAst converts the provided value to an HCL abstract syntax tree
func ConvertToHclAst(data any) (ast.Node, error) {
	j, err := ConvertToJSONFast(data)
	if err != nil {
		return nil, err
	}

	astree, err := jsonParser.Parse([]byte(j))
	if err != nil {
		return nil, err
	}

	// Remove the double quotes around the terraform variable names (the double quotes come from JSON keys)
	// since they will be written to the terraform varfiles and terraform does not like it
	if objectList, ok := astree.Node.(*ast.ObjectList); ok {
		for _, item := range objectList.Items {
			for i, key := range item.Keys {
				item.Keys[i].Token.Text = strings.Replace(key.Token.Text, "\"", "", -1)
			}
		}
	}

	return astree.Node, nil
}

// WriteTerraformBackendConfigToFileAsHcl writes the provided Terraform backend config to the specified file
// https://dev.to/pdcommunity/write-terraform-files-in-go-with-hclwrite-2e1j
// https://pkg.go.dev/github.com/hashicorp/hcl/v2/hclwrite
func WriteTerraformBackendConfigToFileAsHcl(
	cliConfig schema.CliConfiguration,
	filePath string,
	backendType string,
	backendConfig map[any]any,
) error {
	hclFile := hclwrite.NewEmptyFile()
	rootBody := hclFile.Body()
	tfBlock := rootBody.AppendNewBlock("terraform", nil)
	tfBlockBody := tfBlock.Body()
	backendBlock := tfBlockBody.AppendNewBlock("backend", []string{backendType})
	backendBlockBody := backendBlock.Body()

	backendConfigSortedKeys := StringKeysFromMap(convert.MapsOfInterfacesToMapsOfStrings(backendConfig))

	for _, name := range backendConfigSortedKeys {
		v := backendConfig[name]

		if v == nil {
			backendBlockBody.SetAttributeValue(name, cty.NilVal)
		} else if i, ok := v.(string); ok {
			backendBlockBody.SetAttributeValue(name, cty.StringVal(i))
		} else if i, ok := v.(bool); ok {
			backendBlockBody.SetAttributeValue(name, cty.BoolVal(i))
		} else if i, ok := v.(int64); ok {
			backendBlockBody.SetAttributeValue(name, cty.NumberIntVal(i))
		} else if i, ok := v.(uint64); ok {
			backendBlockBody.SetAttributeValue(name, cty.NumberUIntVal(i))
		} else if i, ok := v.(float64); ok {
			backendBlockBody.SetAttributeValue(name, cty.NumberFloatVal(i))
		}
	}

	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}

	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			LogWarning(cliConfig, err.Error())
		}
	}(f)

	_, err = f.Write(hclFile.Bytes())
	if err != nil {
		return err
	}

	return nil
}
