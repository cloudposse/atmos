package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestDescribeStacksCmd(t *testing.T) {
	// Create a new command for testing
	cmd := &cobra.Command{Use: "test"}
	cmd.AddCommand(describeStacksCmd)

	t.Run("Show available JSON fields", func(t *testing.T) {
		b := bytes.NewBufferString("")
		cmd.SetOut(b)
		cmd.SetArgs([]string{"stacks", "--json"})
		err := cmd.Execute()
		assert.NoError(t, err)
		output := b.String()
		assert.Contains(t, output, "name")
		assert.Contains(t, output, "components")
		assert.Contains(t, output, "vars")
	})

	t.Run("Error when using --jq without --json", func(t *testing.T) {
		b := bytes.NewBufferString("")
		cmd.SetOut(b)
		cmd.SetArgs([]string{"stacks", "--jq", "."})
		err := cmd.Execute()
		assert.NoError(t, err)
		output := b.String()
		assert.Contains(t, output, "Error: --json flag is required when using --jq")
	})

	t.Run("Error when using --template without --json", func(t *testing.T) {
		b := bytes.NewBufferString("")
		cmd.SetOut(b)
		cmd.SetArgs([]string{"stacks", "--template", "{{.}}"})
		err := cmd.Execute()
		assert.NoError(t, err)
		output := b.String()
		assert.Contains(t, output, "Error: --json flag is required when using --template")
	})

	t.Run("Error when using both --jq and --template", func(t *testing.T) {
		b := bytes.NewBufferString("")
		cmd.SetOut(b)
		cmd.SetArgs([]string{"stacks", "--json", "name", "--jq", ".", "--template", "{{.}}"})
		err := cmd.Execute()
		assert.NoError(t, err)
		output := b.String()
		assert.Contains(t, output, "Error: cannot use both --jq and --template flags at the same time")
	})

	t.Run("JSON output with field selection", func(t *testing.T) {
		b := bytes.NewBufferString("")
		cmd.SetOut(b)
		cmd.SetArgs([]string{"stacks", "--json", "description"})
		err := cmd.Execute()
		assert.NoError(t, err)
		output := b.String()
		var result map[string]interface{}
		err = json.Unmarshal([]byte(output), &result)
		assert.NoError(t, err)
		// Add more specific assertions based on your test data
	})
}
