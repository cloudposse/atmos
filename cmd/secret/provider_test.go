package secret

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecretCommandProvider(t *testing.T) {
	p := &SecretCommandProvider{}

	assert.Equal(t, secretCmd, p.GetCommand())
	assert.Equal(t, "secret", p.GetName())
	assert.Equal(t, "Configuration", p.GetGroup())
	assert.Equal(t, secretParser, p.GetFlagsBuilder())
	assert.Nil(t, p.GetPositionalArgsBuilder())
	assert.Nil(t, p.GetCompatibilityFlags())
	assert.Nil(t, p.GetAliases())
	assert.True(t, p.IsExperimental())
}
