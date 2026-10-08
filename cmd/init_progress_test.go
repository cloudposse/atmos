package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShowInitConfigProgress(t *testing.T) {
	originalArgs := os.Args
	os.Args = []string{"atmos"}
	t.Cleanup(func() { os.Args = originalArgs })
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"init", "examples/quick-start-simple"}, true},
		{[]string{"--chdir", "/tmp", "init", "./source"}, true},
		{[]string{"init", "--help"}, false},
		{[]string{"help", "init"}, false},
		{[]string{"git", "init"}, false},
		{[]string{"terraform", "init"}, false},
		{[]string{"list", "stacks"}, false},
		{[]string{"version"}, false},
		{[]string{"__complete", "init", ""}, false},
	} {
		assert.Equal(t, tc.want, showInitConfigProgress(RootCmd, tc.args), "%v", tc.args)
	}
}
