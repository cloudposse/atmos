//go:build mage

package test

import (
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/testutil"
)

func TestMain(m *testing.M) { testutil.Main(m) }
