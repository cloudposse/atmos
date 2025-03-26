package exec

import (
	"context"
	"testing"

	"github.com/hairyhenderson/gomplate/v3/data"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestFuncMap(t *testing.T) {
	fm := FuncMap(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, context.TODO(), &data.Data{})
	keys := u.StringKeysFromMap(fm)
	assert.Equal(t, "atmos", keys[0])
}
