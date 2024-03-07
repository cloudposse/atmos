package utils

import (
	"bytes"
	"text/template"
	"text/template/parse"

	"github.com/Masterminds/sprig/v3"
)

// ProcessTmpl parses and executes Go templates
func ProcessTmpl(tmplName string, tmplValue string, tmplData any, ignoreMissingTemplateValues bool) (string, error) {
	t, err := template.New(tmplName).Funcs(sprig.FuncMap()).Parse(tmplValue)
	if err != nil {
		return "", err
	}

	// Control the behavior during execution if a map is indexed with a key that is not present in the map
	// If the template context (`tmplData`) does not provide all the required variables, the following errors would be thrown:
	// template: catalog/terraform/eks_cluster_tmpl_hierarchical.yaml:17:12: executing "catalog/terraform/eks_cluster_tmpl_hierarchical.yaml" at <.flavor>: map has no entry for key "flavor"
	// template: catalog/terraform/eks_cluster_tmpl_hierarchical.yaml:12:36: executing "catalog/terraform/eks_cluster_tmpl_hierarchical.yaml" at <.stage>: map has no entry for key "stage"

	option := "missingkey=error"

	if ignoreMissingTemplateValues {
		option = "missingkey=default"
	}

	t.Option(option)

	var res bytes.Buffer
	err = t.Execute(&res, tmplData)
	if err != nil {
		return "", err
	}

	return res.String(), nil
}

// IsGolangTemplate checks if the provided string is a Go template
func IsGolangTemplate(str string) (bool, error) {
	t, err := template.New(str).Funcs(sprig.FuncMap()).Parse(str)
	if err != nil {
		return false, err
	}

	isGoTemplate := false

	// Iterate over all nodes in the template and check if any of them is of type `NodeAction` (field evaluation)
	for _, node := range t.Root.Nodes {
		if node.Type() == parse.NodeAction {
			isGoTemplate = true
			break
		}
	}

	return isGoTemplate, nil
}
