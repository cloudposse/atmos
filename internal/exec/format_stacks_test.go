package exec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatStacksOutput(t *testing.T) {
	// Test data
	testData := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"myapp": map[string]any{
						"vars": map[string]any{
							"location": "Stockholm",
							"stage":    "dev",
						},
					},
				},
			},
			"description": "Development stack",
		},
		"prod": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"myapp": map[string]any{
						"vars": map[string]any{
							"location": "Los Angeles",
							"stage":    "prod",
						},
					},
				},
			},
			"description": "Production stack",
		},
	}

	t.Run("No formatting options returns JSON", func(t *testing.T) {
		output, err := FormatStacksOutput(testData, "", "", "")
		assert.NoError(t, err)
		var result map[string]interface{}
		err = json.Unmarshal([]byte(output), &result)
		assert.NoError(t, err)
		assert.Equal(t, testData, result)
	})

	t.Run("JSON field filtering", func(t *testing.T) {
		output, err := FormatStacksOutput(testData, "description", "", "")
		assert.NoError(t, err)
		var result map[string]interface{}
		err = json.Unmarshal([]byte(output), &result)
		assert.NoError(t, err)
		assert.Equal(t, "Development stack", result["dev"].(map[string]interface{})["description"])
		assert.NotContains(t, result["dev"].(map[string]interface{}), "components")
	})

	t.Run("JQ query transformation", func(t *testing.T) {
		output, err := FormatStacksOutput(testData, "", "to_entries | map({name: .key})", "")
		assert.NoError(t, err)
		var result []map[string]string
		err = json.Unmarshal([]byte(output), &result)
		assert.NoError(t, err)
		assert.Contains(t, []string{result[0]["name"], result[1]["name"]}, "dev")
		assert.Contains(t, []string{result[0]["name"], result[1]["name"]}, "prod")
	})

	t.Run("Go template formatting", func(t *testing.T) {
		template := `{{range $stack, $data := .}}{{$stack}}: {{$data.description}}{{"\n"}}{{end}}`
		output, err := FormatStacksOutput(testData, "", "", template)
		assert.NoError(t, err)
		assert.Contains(t, output, "dev: Development stack")
		assert.Contains(t, output, "prod: Production stack")
	})

	t.Run("JSON fields with JQ query", func(t *testing.T) {
		output, err := FormatStacksOutput(testData, "description", "to_entries | map({name: .key, desc: .value.description})", "")
		assert.NoError(t, err)
		var result []map[string]string
		err = json.Unmarshal([]byte(output), &result)
		assert.NoError(t, err)
		assert.Len(t, result, 2)
		for _, item := range result {
			if item["name"] == "dev" {
				assert.Equal(t, "Development stack", item["desc"])
			} else if item["name"] == "prod" {
				assert.Equal(t, "Production stack", item["desc"])
			}
		}
	})

	t.Run("JSON fields with Go template", func(t *testing.T) {
		template := `{{range $stack, $data := .}}{{tablerow $stack $data.description}}{{end}}`
		output, err := FormatStacksOutput(testData, "description", "", template)
		assert.NoError(t, err)
		assert.Contains(t, output, "dev  Development stack")
		assert.Contains(t, output, "prod  Production stack")
	})
} 
