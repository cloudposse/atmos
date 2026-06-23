package runner

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "terraform" {
		os.Exit(0)
	}
	os.Exit(m.Run())
}
