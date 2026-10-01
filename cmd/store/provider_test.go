package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStoreCommandProvider(t *testing.T) {
	p := &StoreCommandProvider{}

	assert.Equal(t, storeCmd, p.GetCommand())
	assert.Equal(t, "store", p.GetName())
	assert.Equal(t, "Configuration", p.GetGroup())
	assert.Equal(t, storeParser, p.GetFlagsBuilder())
	assert.Nil(t, p.GetPositionalArgsBuilder())
	assert.Nil(t, p.GetCompatibilityFlags())
	assert.Nil(t, p.GetAliases())
	assert.True(t, p.IsExperimental())
}
