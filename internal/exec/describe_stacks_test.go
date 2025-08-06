package exec

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/config"

	log "github.com/charmbracelet/log"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestDescribeStacksExec(t *testing.T) {
	ctrl := gomock.NewController(t)
	d := &describeStacksExec{
		pageCreator: pager.NewMockPageCreator(ctrl),
		isTTYSupportForStdout: func() bool {
			return false
		},
		printOrWriteToFile: func(atmosConfig *schema.AtmosConfiguration, format, file string, data any) error {
			return nil
		},
		executeDescribeStacks: func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components, componentTypes, sections []string, ignoreMissingFiles, processTemplates, processYamlFunctions, includeEmptyStacks bool, skip []string) (map[string]any, error) {
			return map[string]any{
				"hello": "test",
			}, nil
		},
	}
	err := d.Execute(&schema.AtmosConfiguration{}, &DescribeStacksArgs{})
	assert.NoError(t, err)
}

func TestExecuteDescribeStacks_Packer(t *testing.T) {
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_CLI_CONFIG_PATH': %v", err)
	}

	err = os.Unsetenv("ATMOS_BASE_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_BASE_PATH': %v", err)
	}

	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Change back to the original working directory after the test
		if err = os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/packer"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	atmosConfig, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	stacksMap, err := ExecuteDescribeStacks(
		&atmosConfig,
		"",
		nil,
		nil,
		nil,
		false,
		true,
		true,
		false,
		nil,
	)
	assert.Nil(t, err)

	val, err := u.EvaluateYqExpression(&atmosConfig, stacksMap, ".prod.components.packer.aws/bastion.vars.ami_tags.SourceAMI")
	assert.Nil(t, err)
	assert.Equal(t, "ami-0013ceeff668b979b", val)

	val, err = u.EvaluateYqExpression(&atmosConfig, stacksMap, ".nonprod.components.packer.aws/bastion.metadata.component")
	assert.Nil(t, err)
	assert.Equal(t, "aws/bastion", val)
}
