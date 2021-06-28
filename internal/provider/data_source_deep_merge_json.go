package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	c "github.com/cloudposse/terraform-provider-utils/internal/convert"
	m "github.com/cloudposse/terraform-provider-utils/internal/merge"
)

func dataSourceDeepMergeJSON() *schema.Resource {
	return &schema.Resource{
		Description: "The `deep_merge_json` data source accepts a list of JSON strings as input and deep merges into a single JSON string as output.",

		ReadContext: dataSourceDeepMergeJSONRead,

		Schema: map[string]*schema.Schema{
			"input": {
				Description: "A list of JSON strings that is deep merged into the `output` attribute.",
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

func dataSourceDeepMergeJSONRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	input := d.Get("input")

	data, err := c.JSONSliceOfInterfaceToSliceOfMaps(input.([]interface{}))
	if err != nil {
		return diag.FromErr(err)
	}

	merged, err := m.Merge(data)
	if err != nil {
		return diag.FromErr(err)
	}

	map2 := map[string]interface{}{}

	for k, v := range merged {
		map2[k.(string)] = v
	}

	// Convert result to JSON
	jsonResult, err := json.Marshal(map2)
	if err != nil {
		return diag.FromErr(err)
	}

	err = d.Set("output", string(jsonResult))
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(c.MakeId(jsonResult))

	return nil
}
