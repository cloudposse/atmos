package test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/qdm12/reprint"
	"github.com/stretchr/testify/assert"
)

type NLContext struct {
	AdditionalTagMap  map[string]string `json:"additional_tag_map"`
	Attributes        []string          `json:"attributes"`
	Delimiter         interface{}       `json:"delimiter"`
	Enabled           bool              `json:"enabled"`
	Environment       interface{}       `json:"environment"`
	LabelOrder        []string          `json:"label_order"`
	Name              interface{}       `json:"name"`
	Namespace         interface{}       `json:"namespace"`
	RegexReplaceChars interface{}       `json:"regex_replace_chars"`
	Stage             interface{}       `json:"stage"`
	Tags              map[string]string `json:"tags"`
	Tenant            interface{}       `json:"tenant"`
}

// Test the Terraform module in examples/complete using Terratest.
func TestExamplesComplete(t *testing.T) {
	t.Parallel()

	terraformOptions := &terraform.Options{
		// The path to where our Terraform code is located
		TerraformDir: "../../examples/complete",
		Upgrade:      true,
	}

	// At the end of the test, run `terraform destroy` to clean up any resources that were created
	defer terraform.Destroy(t, terraformOptions)

	// This will run `terraform init` and `terraform apply` and fail the test if there are any errors
	terraform.InitAndApply(t, terraformOptions)

	compatible := terraform.Output(t, terraformOptions, "compatible")
	assert.Equal(t, "true", compatible)

	expectedDescriptorAccountName := "bild-hrh"
	expectedDescriptorStack := "hrh-uat-bild"
	descriptorAccountName := terraform.Output(t, terraformOptions, "descriptor_account_name")
	descriptorStack := terraform.Output(t, terraformOptions, "descriptor_stack")
	assert.Equal(t, expectedDescriptorAccountName, descriptorAccountName)
	assert.Equal(t, expectedDescriptorStack, descriptorStack)

	chainedDescriptorAccountName := terraform.Output(t, terraformOptions, "chained_descriptor_account_name")
	chainedDescriptorStack := terraform.Output(t, terraformOptions, "chained_descriptor_stack")
	assert.Equal(t, descriptorAccountName, chainedDescriptorAccountName, "Chained module should output same descriptors")
	assert.Equal(t, descriptorStack, chainedDescriptorStack, "Chained module should output same descriptors")

	expectedLabel1Context := NLContext{
		Enabled:     true,
		Namespace:   "CloudPosse",
		Tenant:      "H.R.H",
		Environment: "UAT",
		Stage:       "build",
		Name:        "Winston Churchroom",
		Attributes:  []string{"fire", "water", "earth", "air"},
		Delimiter:   nil,
		LabelOrder:  []string{"name", "tenant", "environment", "stage", "attributes"},
		Tags: map[string]string{
			"City":        "Dublin",
			"Environment": "Private",
		},
		AdditionalTagMap: map[string]string{},
	}

	var expectedLabel1NormalizedContext NLContext
	_ = reprint.FromTo(&expectedLabel1Context, &expectedLabel1NormalizedContext)
	expectedLabel1NormalizedContext.Namespace = "cloudposse"
	expectedLabel1NormalizedContext.Tenant = "hrh"
	expectedLabel1NormalizedContext.Environment = "uat"
	expectedLabel1NormalizedContext.Name = "winstonchurchroom"
	expectedLabel1NormalizedContext.Delimiter = "-"
	expectedLabel1NormalizedContext.RegexReplaceChars = "/[^-a-zA-Z0-9]/"
	expectedLabel1NormalizedContext.Tags = map[string]string{
		"City":        "Dublin",
		"Environment": "Private",
		"Namespace":   "cloudposse",
		"Stage":       "build",
		"Tenant":      "hrh",
		"Name":        "winstonchurchroom-hrh-uat-build-fire-water-earth-air",
		"Attributes":  "fire-water-earth-air",
	}

	var label1NormalizedContext, label1Context NLContext
	// Run `terraform output` to get the value of an output variable
	label1 := terraform.OutputMap(t, terraformOptions, "label1")
	label1Tags := terraform.OutputMap(t, terraformOptions, "label1_tags")
	terraform.OutputStruct(t, terraformOptions, "label1_normalized_context", &label1NormalizedContext)
	terraform.OutputStruct(t, terraformOptions, "label1_context", &label1Context)

	// Verify we're getting back the outputs we expect
	assert.Equal(t, "winstonchurchroom-hrh-uat-build-fire-water-earth-air", label1["id"])
	assert.Equal(t, "winstonchurchroom-hrh-uat-build-fire-water-earth-air", label1Tags["Name"])
	assert.Equal(t, "Dublin", label1Tags["City"])
	assert.Equal(t, "Private", label1Tags["Environment"])
	assert.Equal(t, expectedLabel1NormalizedContext, label1NormalizedContext)
	assert.Equal(t, expectedLabel1Context, label1Context)

	label1t1 := terraform.OutputMap(t, terraformOptions, "label1t1")
	label1t1Tags := terraform.OutputMap(t, terraformOptions, "label1t1_tags")
	assert.Equal(t, "winstonchurchroom-hrh-uat-6403d8", label1t1["id"],
		"Extra hash character should be added when trailing delimiter is removed")
	assert.Equal(t, label1["id"], label1t1["id_full"], "id_full should not be truncated")
	assert.Equal(t, label1t1["id"], label1t1Tags["Name"], "Name tag should match ID")

	label1t2 := terraform.OutputMap(t, terraformOptions, "label1t2")
	label1t2Tags := terraform.OutputMap(t, terraformOptions, "label1t2_tags")
	assert.Equal(t, "winstonchurchroom-hrh-uat-b-6403d", label1t2["id"])
	assert.Equal(t, label1t2["id"], label1t2Tags["Name"], "Name tag should match ID")

	// Run `terraform output` to get the value of an output variable
	label2 := terraform.OutputMap(t, terraformOptions, "label2")
	label2Tags := terraform.OutputMap(t, terraformOptions, "label2_tags")

	// Verify we're getting back the outputs we expect
	assert.Equal(t, "charlie+uat+test+fire+water+earth+air", label2["id"])
	assert.Equal(t, "charlie+uat+test+fire+water+earth+air", label2Tags["Name"])
	assert.Equal(t, "London", label2Tags["City"])
	assert.Equal(t, "Public", label2Tags["Environment"])

	var expectedLabel3cContext, label3cContext NLContext
	_ = reprint.FromTo(&expectedLabel1Context, &expectedLabel3cContext)
	expectedLabel3cContext.Name = "Starfish"
	expectedLabel3cContext.Stage = "release"
	expectedLabel3cContext.Delimiter = "."
	expectedLabel3cContext.RegexReplaceChars = "/[^-a-zA-Z0-9.]/"
	expectedLabel3cContext.Tags["Eat"] = "Carrot"
	expectedLabel3cContext.Tags["Animal"] = "Rabbit"

	// Run `terraform output` to get the value of an output variable
	label3c := terraform.OutputMap(t, terraformOptions, "label3c")
	label3cTags := terraform.OutputMap(t, terraformOptions, "label3c_tags")
	terraform.OutputStruct(t, terraformOptions, "label3c_context", &label3cContext)

	// Verify we're getting back the outputs we expect
	assert.Equal(t, "starfish.h.r.h.uat.release.fire.water.earth.air", label3c["id"])
	assert.Equal(t, "starfish.h.r.h.uat.release.fire.water.earth.air", label3cTags["Name"])
	assert.Equal(t, expectedLabel3cContext, label3cContext)

	var expectedLabel3nContext, label3nContext NLContext
	_ = reprint.FromTo(&expectedLabel1NormalizedContext, &expectedLabel3nContext)
	expectedLabel3nContext.Name = "Starfish"
	expectedLabel3nContext.Stage = "release"
	expectedLabel3nContext.Delimiter = "."
	expectedLabel3nContext.RegexReplaceChars = "/[^-a-zA-Z0-9.]/"
	expectedLabel3nContext.Tags["Eat"] = "Carrot"
	expectedLabel3nContext.Tags["Animal"] = "Rabbit"

	// Run `terraform output` to get the value of an output variable
	label3n := terraform.OutputMap(t, terraformOptions, "label3n")
	label3nTags := terraform.OutputMap(t, terraformOptions, "label3n_tags")
	terraform.OutputStruct(t, terraformOptions, "label3n_context", &label3nContext)

	// Verify we're getting back the outputs we expect
	// The tenant from normalized label1 should be "hrh" not "h.r.h."
	assert.Equal(t, "starfish.hrh.uat.release.fire.water.earth.air", label3n["id"])
	assert.Equal(t, label1Tags["Name"], label3nTags["Name"],
		"Tag from label1 normalized context should overwrite label3n generated tag")
	assert.Equal(t, expectedLabel3nContext, label3nContext)

	// Run `terraform output` to get the value of an output variable
	label4 := terraform.OutputMap(t, terraformOptions, "label4")
	label4Tags := terraform.OutputMap(t, terraformOptions, "label4_tags")

	// Verify we're getting back the outputs we expect
	assert.Equal(t, "cloudposse-uat-big-fat-honking-cluster", label4["id"])
	assert.Equal(t, "cloudposse-uat-big-fat-honking-cluster", label4Tags["Name"])

	// Run `terraform output` to get the value of an output variable
	label5 := terraform.OutputMap(t, terraformOptions, "label5")

	// Verify we're getting back the outputs we expect
	assert.Equal(t, "", label5["id"])

	label6f := terraform.OutputMap(t, terraformOptions, "label6f")
	label6fTags := terraform.OutputMap(t, terraformOptions, "label6f_tags")
	// Test of setting var.label_key_case = "lower", var.label_value_case = "upper"
	assert.Equal(t, "CP~UW2~PRD~NULL-LABEL", label6f["id_full"])
	assert.Equal(t, label6f["id_full"], label6f["id"], "id should not be truncated")
	assert.Equal(t, label6f["id"], label6fTags["name"], "Name tag should match ID")

	label6t := terraform.OutputMap(t, terraformOptions, "label6t")
	label6tTags := terraform.OutputMap(t, terraformOptions, "label6t_tags")
	assert.Equal(t, "CPUW2PRDNULL-LABEL", label6t["id_full"])
	assert.NotEqual(t, label6t["id_full"], label6t["id"], "id should be truncated")
	assert.Equal(t, label6t["id"], label6tTags["name"], "Name tag should match ID")
	assert.Equal(t, label6t["id_length_limit"], fmt.Sprintf("%d", len(label6t["id"])),
		"Truncated ID length should equal length limit")

	label7 := terraform.OutputMap(t, terraformOptions, "label7")
	assert.Equal(t, "eg-demo-blue-cluster-nodegroup", label7["id"], "var.attributes should be appended after context.attributes")

	// Verify that apply with `label_key_case=title`, `label_value_case=lower`, `delimiter=""` returns expected value of id, context id
	label8dndID := terraform.Output(t, terraformOptions, "label8dnd_id")
	label8dndContextID := terraform.Output(t, terraformOptions, "label8dnd_context_id")
	assert.Equal(t, "egdemobluecluster", label8dndID)
	assert.Equal(t, label8dndID, label8dndContextID, "ID and context ID should be equal")

	// Verify that apply with `label_key_case=title`, `label_value_case=lower`, `delimiter="x"` returns expected value of id, context id
	label8dcdID := terraform.Output(t, terraformOptions, "label8dcd_id")
	label8dcdContextID := terraform.Output(t, terraformOptions, "label8dcd_context_id")
	assert.Equal(t, "egxdemoxbluexcluster", label8dcdID)
	assert.Equal(t, label8dcdID, label8dcdContextID, "ID and context ID should be equal")

	// Verify that apply with `label_key_case=title` and `label_value_case=lower` returns expected values of id, tags, context tags
	label8dExpectedTags := map[string]string{
		"Attributes":  "cluster",
		"Environment": "demo",
		"Name":        "eg-demo-blue-cluster",
		// Suppressed by labels_as_tags: "Namespace":              "eg",
		"kubernetes.io/cluster/": "shared",
	}

	label8dID := terraform.Output(t, terraformOptions, "label8d_id")
	label8dContextID := terraform.Output(t, terraformOptions, "label8d_context_id")
	label8dChained := terraform.Output(t, terraformOptions, "label8d_chained_context_labels_as_tags")
	assert.Equal(t, "eg-demo-blue-cluster", label8dID)
	assert.Equal(t, label8dID, label8dContextID, "ID and context ID should be equal")
	assert.Equal(t, "attributes-environment-name-stage", label8dChained)

	label8dTags := terraform.OutputMap(t, terraformOptions, "label8d_tags")
	label8dContextTags := terraform.OutputMap(t, terraformOptions, "label8d_context_tags")

	assert.Exactly(t, label8dExpectedTags, label8dTags, "generated tags are different from expected")
	assert.Exactly(t, label8dTags, label8dContextTags, "tags and context tags should be equal")

	// Verify that apply with `label_key_case=lower` and  `label_value_case=lower` returns expected values of id, tags, context tags
	label8lExpectedTags := map[string]string{
		"attributes":             "cluster",
		"environment":            "demo",
		"name":                   "eg-demo-blue-cluster",
		"namespace":              "eg",
		"kubernetes.io/cluster/": "shared",
		"upperTEST":              "testUPPER",
	}

	label8lID := terraform.Output(t, terraformOptions, "label8l_id")
	label8lContextID := terraform.Output(t, terraformOptions, "label8l_context_id")
	assert.Equal(t, "eg-demo-blue-cluster", label8lID)
	assert.Equal(t, label8lID, label8lContextID, "ID and context ID should be equal")

	label8lTags := terraform.OutputMap(t, terraformOptions, "label8l_tags")
	label8lContextTags := terraform.OutputMap(t, terraformOptions, "label8l_context_tags")

	assert.Exactly(t, label8lExpectedTags, label8lTags, "generated tags are different from expected")
	assert.Exactly(t, label8lTags, label8lContextTags, "tags and context tags should be equal")

	// Verify that apply with `label_key_case=title` and  `label_value_case=title` returns expected values of id, tags, context tags
	label8tExpectedTags := map[string]string{
		"Attributes":             "Eks-Cluster",
		"Environment":            "Demo",
		"Name":                   "Eg-Demo-Blue-Eks-Cluster",
		"Namespace":              "Eg",
		"kubernetes.io/cluster/": "shared",
	}

	label8tID := terraform.Output(t, terraformOptions, "label8t_id")
	label8tContextID := terraform.Output(t, terraformOptions, "label8t_context_id")
	assert.Equal(t, "Eg-Demo-Blue-Eks-Cluster", label8tID)
	assert.Equal(t, label8tID, label8tContextID, "ID and context ID should be equal")

	label8tTags := terraform.OutputMap(t, terraformOptions, "label8t_tags")
	label8tContextTags := terraform.OutputMap(t, terraformOptions, "label8t_context_tags")

	assert.Exactly(t, label8tExpectedTags, label8tTags, "generated tags are different from expected")
	assert.Exactly(t, label8tTags, label8tContextTags, "tags and context tags should be equal")

	// Verify that apply with `label_key_case=upper` and  `label_value_case=upper` returns expected values of id, tags, context tags
	label8uExpectedTags := map[string]string{
		"ATTRIBUTES":             "CLUSTER",
		"ENVIRONMENT":            "DEMO",
		"NAME":                   "EG-DEMO-BLUE-CLUSTER",
		"NAMESPACE":              "EG",
		"kubernetes.io/cluster/": "shared",
	}

	label8uID := terraform.Output(t, terraformOptions, "label8u_id")
	label8uContextID := terraform.Output(t, terraformOptions, "label8u_context_id")
	assert.Equal(t, "EG-DEMO-BLUE-CLUSTER", label8uID)
	assert.Equal(t, label8uID, label8uContextID, "ID and context ID should be equal")

	label8uTags := terraform.OutputMap(t, terraformOptions, "label8u_tags")
	label8uContextTags := terraform.OutputMap(t, terraformOptions, "label8u_context_tags")

	assert.Exactly(t, label8uExpectedTags, label8uTags, "generated tags are different from expected")
	assert.Exactly(t, label8uTags, label8uContextTags, "tags and context tags should be equal")

	// Verify that apply with `label_key_case=title` and  `label_value_case=none` returns expected values of id, tags, context tags
	label8nExpectedTags := map[string]string{
		"Attributes":             "eks-ClusteR",
		"Environment":            "demo",
		"Name":                   "EG-demo-blue-eks-ClusteR",
		"Namespace":              "EG",
		"kubernetes.io/cluster/": "shared",
	}

	label8nID := terraform.Output(t, terraformOptions, "label8n_id")
	label8nContextID := terraform.Output(t, terraformOptions, "label8n_context_id")
	assert.Equal(t, "EG-demo-blue-eks-ClusteR", label8nID)
	assert.Equal(t, label8nID, label8nContextID, "ID and context ID should be equal")

	label8nTags := terraform.OutputMap(t, terraformOptions, "label8n_tags")
	label8nContextTags := terraform.OutputMap(t, terraformOptions, "label8n_context_tags")

	assert.Exactly(t, label8nExpectedTags, label8nTags, "generated tags are different from expected")
	assert.Exactly(t, label8nTags, label8nContextTags, "tags and context tags should be equal")
}
