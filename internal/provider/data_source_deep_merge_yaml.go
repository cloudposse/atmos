package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"gopkg.in/yaml.v2"

	c "github.com/cloudposse/terraform-provider-utils/internal/convert"
	m "github.com/cloudposse/terraform-provider-utils/internal/merge"
)

func dataSourceDeepMergeYAML() *schema.Resource {
	return &schema.Resource{
		Description: "The `deep_merge_yaml` data source accepts a list of YAML strings as input and deep merges into a single YAML string as output.",

		ReadContext: dataSourceDeepMergeYAMLRead,

		Schema: map[string]*schema.Schema{
			"input": {
				Description: "A list of YAML strings that is deep merged into the `output` attribute.",
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Required:    true,
			},
			"output": {
				Description: "The deep-merged output.",
				Type:        schema.TypeString,
				Computed:    true,
			},
		},
	}
}

func dataSourceDeepMergeYAMLRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	input := d.Get("input")

	data, err := c.YAMLSliceOfInterfaceToSliceOfMaps(input.([]interface{}))
	if err != nil {
		return diag.FromErr(err)
	}

	merged, err := m.Merge(data)
	if err != nil {
		return diag.FromErr(err)
	}

	// Convert result to YAML
	yamlResult, err := yaml.Marshal(merged)
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("output", string(yamlResult))
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(c.MakeId(yamlResult))

	return nil
}
