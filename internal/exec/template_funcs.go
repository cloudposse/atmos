// https://forum.golangbridge.org/t/html-template-optional-argument-in-function/6080
// https://lkumarjain.blogspot.com/2020/11/deep-dive-into-go-template.html
// https://echorand.me/posts/golang-templates/
// https://www.practical-go-lessons.com/chap-32-templates
// https://docs.gofiber.io/template/next/html/TEMPLATES_CHEATSHEET/
// https://engineering.01cloud.com/2023/04/13/optional-function-parameter-pattern/

package exec

import (
	"context"
	"fmt"
	"text/template"

	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/samber/lo"
)

// FuncMap creates and returns a map of template functions
func FuncMap(ctx context.Context) template.FuncMap {
	atmosFuncs := &AtmosFuncs{ctx}

	return map[string]any{
		"atmos": func() any { return atmosFuncs },
	}
}

type AtmosFuncs struct {
	ctx context.Context
}

func (AtmosFuncs) Component(component string, stack string) (any, error) {
	return componentFunc(component, stack)
}

func componentFunc(component string, stack string) (any, error) {
	sections, err := ExecuteDescribeComponent(component, stack)
	if err != nil {
		return nil, err
	}

	executable, ok := sections["command"].(string)
	if !ok {
		return nil, fmt.Errorf("the component '%s' in the stack '%s' does not have 'command' (executable) defined", component, stack)
	}

	terraformWorkspace, ok := sections["workspace"].(string)
	if !ok {
		return nil, fmt.Errorf("the component '%s' in the stack '%s' does not have Terraform/OpenTofu workspace defined", component, stack)
	}

	componentInfo, ok := sections["component_info"]
	if !ok {
		return nil, fmt.Errorf("the component '%s' in the stack '%s' does not have 'component_info' defined", component, stack)
	}

	componentInfoMap, ok := componentInfo.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("the component '%s' in the stack '%s' has an invalid 'component_info' section", component, stack)
	}

	componentPath, ok := componentInfoMap["component_path"].(string)
	if !ok {
		return nil, fmt.Errorf("the component '%s' in the stack '%s' has an invalid 'component_info.component_path' section", component, stack)
	}

	tf, err := tfexec.NewTerraform(componentPath, executable)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	err = tf.Init(ctx, tfexec.Upgrade(false))
	if err != nil {
		return nil, err
	}

	err = tf.WorkspaceNew(ctx, terraformWorkspace)
	if err != nil {
		err = tf.WorkspaceSelect(ctx, terraformWorkspace)
		if err != nil {
			return nil, err
		}
	}

	outputMeta, err := tf.Output(ctx)
	if err != nil {
		return nil, err
	}

	outputMetaProcessed := lo.MapEntries(outputMeta, func(k string, v tfexec.OutputMeta) (string, any) {
		d, _ := utils.ConvertFromJSON(string(v.Value))
		return k, d
	})

	outputs := map[string]any{
		"outputs": outputMetaProcessed,
	}

	sections = lo.Assign(sections, outputs)

	return sections, nil
}
