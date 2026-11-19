package docs

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocsCmdOpensDefaultDocsURL(t *testing.T) {
	origOpen := openDocsURL
	t.Cleanup(func() { openDocsURL = origOpen })

	var openedURL string
	openDocsURL = func(url string) error {
		openedURL = url
		return nil
	}

	require.NoError(t, docsCmd.RunE(docsCmd, nil))
	assert.Equal(t, atmosDocsURL, openedURL)
}

func TestDocsCmdReturnsDefaultDocsOpenError(t *testing.T) {
	origOpen := openDocsURL
	t.Cleanup(func() { openDocsURL = origOpen })

	openErr := errors.New("browser unavailable")
	openDocsURL = func(string) error {
		return openErr
	}

	err := docsCmd.RunE(docsCmd, nil)
	require.ErrorIs(t, err, openErr)
	assert.ErrorContains(t, err, "open Atmos docs")
}

func TestDocsCommandProvider(t *testing.T) {
	provider := &DocsCommandProvider{}

	cmd := provider.GetCommand()
	require.NotNil(t, cmd)
	assert.Equal(t, "docs", cmd.Use)
	assert.Equal(t, "docs", provider.GetName())
	assert.Equal(t, "Configuration Management", provider.GetGroup())
	assert.Nil(t, provider.GetFlagsBuilder())
	assert.Nil(t, provider.GetPositionalArgsBuilder())
	assert.Nil(t, provider.GetCompatibilityFlags())
	assert.Nil(t, provider.GetAliases())
	assert.False(t, provider.IsExperimental())

	sub, _, err := cmd.Find([]string{"generate"})
	require.NoError(t, err)
	assert.Equal(t, "generate", sub.Name())
}
