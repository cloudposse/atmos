package utils

import (
	"bytes"
	"text/template"
)

// ProcessTmpl parses and executes Go templates
func ProcessTmpl(tmplName string, tmplValue string, tmplData any) (string, error) {
	t, err := template.New(tmplName).Parse(tmplValue)
	if err != nil {
		return "", err
	}
	var res bytes.Buffer
	err = t.Execute(&res, tmplData)
	if err != nil {
		return "", err
	}
	return res.String(), nil
}
