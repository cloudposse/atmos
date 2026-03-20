package describe

import (
	"fmt"
	"testing"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDebugStackCounts(t *testing.T) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		t.Fatal(err)
	}

	stacks, _ := ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, false)
	fmt.Printf("Without empty stacks: %d\n", len(stacks))
	for k := range stacks {
		fmt.Printf("  Stack: %q\n", k)
	}

	stacksWithEmpty, _ := ExecuteDescribeStacks(atmosConfig, "", nil, nil, nil, false, true)
	fmt.Printf("\nWith empty stacks: %d\n", len(stacksWithEmpty))
	for k := range stacksWithEmpty {
		fmt.Printf("  Stack: %q\n", k)
	}
}
