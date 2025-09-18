//go:build test
// +build test

package exec

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

func test_FindStacksMap(ac *schema.AtmosConfiguration, deep bool) (map[string]any, map[string]any, error) {
	return findStacksMapFn(ac, deep)
}