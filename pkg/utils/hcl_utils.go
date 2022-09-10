package utils

import (
	"github.com/hashicorp/hcl/hcl/ast"
	"github.com/hashicorp/hcl/hcl/printer"
	jsonParser "github.com/hashicorp/hcl/json/parser"
	"os"
)

// PrintAsHcl prints the provided value as HCL (HashiCorp Language) document to the console
func PrintAsHcl(data any) error {
	astree, err := ConvertToHclAst(data)
	if err != nil {
		return err
	}

	err = printer.Fprint(os.Stdout, astree.Node)
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

	err = printer.Fprint(f, astree.Node)
	if err != nil {
		return err
	}

	return nil
}

// ConvertToHclAst converts the provided value to an HCL abstract syntax tree
func ConvertToHclAst(data any) (*ast.File, error) {
	j, err := ConvertToJSON(data)
	if err != nil {
		return nil, err
	}

	astree, err := jsonParser.Parse([]byte(j))
	if err != nil {
		return nil, err
	}

	return astree, nil
}
