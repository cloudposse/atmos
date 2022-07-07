package utils_test

import (
	"os"
	"path"
	"testing"

	// "utils"

	c2 "github.com/cloudposse/atmos/pkg/component"
	c "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/utils"

	"github.com/stretchr/testify/assert"
)

func TestBackendConfig(t *testing.T) {
	var err error
	var component string
	var stack string

	// Add atmos.yaml for test config
	r, err := os.Open("../component/atmos.yaml")
	if err != nil {
		panic(err)
	}
	defer r.Close()
	w, err := os.Create("./atmos.yaml")
	if err != nil {
		panic(err)
	}
	defer w.Close()
	w.ReadFrom(r)

	var tenant1Ue2DevTestTestComponent map[string]any
	component = "test/test-component"
	stack = "tenant1-ue2-dev"

	tenant1Ue2DevTestTestComponent, err = c2.ProcessComponentInStack(component, stack)
	assert.Nil(t, err)
	assert.NotNil(t, tenant1Ue2DevTestTestComponent)

	var componentBackendConfig = map[string]any{
		"terraform": map[string]any{
			"backend": map[string]any{
				"s3": tenant1Ue2DevTestTestComponent["backend"],
			},
		},
	}

	var backendFilePath = path.Join(
		c.Config.BasePath,
		c.Config.Components.Terraform.BasePath,
		"test",           // info.ComponentFolderPrefix,
		"test-component", //  info.FinalComponent,
		"backend.tf.json",
	)

	err = utils.WriteToFileAsJSON(backendFilePath, componentBackendConfig, 0644)
	assert.Nil(t, err)

	data, err := os.ReadFile(backendFilePath)
	assert.Nil(t, err)

	assert.Equal(t, utils.RemoveWhitespace(`{
		"terraform": {
		  "backend": {
			"s3": {
			  "encrypt": true,
			  "key": "terraform.tfstate",
			  "region": "us-east-1",
			  "role_arn": null,
			  "workspace_key_prefix": "app",
			  "acl": "bucket-owner-full-control",
			  "bucket": "sts-gbl-tfstate-backend",
			  "dynamodb_table": "sts-gbl-tfstate-backend-lock"
			}
		  }
		}
	  }`), utils.RemoveWhitespace(string(data)))

	// Remove config file
	os.Remove("./atmos.yaml")
}
