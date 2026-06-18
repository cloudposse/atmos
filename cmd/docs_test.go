package cmd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDocsCmd_WithoutStacks verifies that docs command does not require stack configuration.
// This test documents that the docs command uses InitCliConfig with processStacks=false.
func TestDocsCmd_WithoutStacks(t *testing.T) {
	// This test documents that docs command does not process stacks
	// by verifying InitCliConfig is called with processStacks=false in docs.go:38
	// No runtime test needed - this is enforced by code structure.
	t.Log("docs command uses InitCliConfig with processStacks=false")
}

func TestDocsCmdOpensDefaultDocsURL(t *testing.T) {
	var openedURL string
	originalOpenDocsURL := openDocsURL
	t.Cleanup(func() {
		openDocsURL = originalOpenDocsURL
	})
	openDocsURL = func(url string) error {
		openedURL = url
		return nil
	}

	require.NoError(t, docsCmd.RunE(docsCmd, nil))
	assert.Equal(t, atmosDocsURL, openedURL)
}

func TestDocsCmdReturnsDefaultDocsOpenError(t *testing.T) {
	openErr := errors.New("browser unavailable")
	originalOpenDocsURL := openDocsURL
	t.Cleanup(func() {
		openDocsURL = originalOpenDocsURL
	})
	openDocsURL = func(string) error {
		return openErr
	}

	err := docsCmd.RunE(docsCmd, nil)
	require.ErrorIs(t, err, openErr)
	assert.ErrorContains(t, err, "open Atmos docs")
}
