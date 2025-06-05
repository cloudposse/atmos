package utils

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"golang.org/x/term"
)

func TestHighlightCodeWithConfig(t *testing.T) {
	isTermPresent = true
	defer func() {
		isTermPresent = term.IsTerminal(int(os.Stdout.Fd()))
	}()
	code, err := HighlightCodeWithConfig(&schema.AtmosConfiguration{}, `{"code":"hello"}`, "json")
	assert.NoError(t, err)
	assert.Contains(t, code, "code")
	assert.Contains(t, code, "hello")
}
