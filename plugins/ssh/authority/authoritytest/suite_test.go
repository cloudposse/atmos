package authoritytest

import (
	"testing"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
)

func TestSuiteAgainstFake(t *testing.T) {
	Run(t, func(*testing.T) *Environment { return NewFake("fake").Environment() })
}

func TestSuiteAgainstSingleUseFake(t *testing.T) {
	Run(t, func(*testing.T) *Environment {
		return NewFake("single", authority.UseSigning).Environment()
	})
}
