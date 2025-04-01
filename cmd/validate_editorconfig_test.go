package cmd

import "testing"

func TestInitConfig(t *testing.T) {
	// TODO: We need to enhance testing coverage.
	// The reason we are skipping this part is because there is already a validate-editorconfig.yaml
	// that tests the editorconfig command.
	// We have a ticket to address coverage of such test cases here:
	// https://linear.app/cloudposse/issue/DEV-3094/update-the-testclicommands
	initializeConfig(editorConfigCmd)
}
