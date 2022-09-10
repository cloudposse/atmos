package utils

import (
	"github.com/hashicorp/hcl/hcl/ast"
	"github.com/hashicorp/hcl/hcl/printer"
	jsonParser "github.com/hashicorp/hcl/json/parser"

	"os"
	"strings"
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

// WriteToFileAsHcl converts the provided value to HCL (HashiCorp Language) and writes it to the provided file
func WriteToFileAsHcl(filePath string, data any, fileMode os.FileMode) error {
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
			PrintError(err)
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
