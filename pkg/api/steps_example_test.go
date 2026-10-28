package api_test

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/runner/step"
)

func ExampleStepLibrary() {
	library := step.NewAutomationLibrary(nil, nil)
	result, err := library.Run(context.Background(), &api.StepCall{
		Type:          "join",
		Configuration: map[string]any{"options": []string{"api", "worker"}, "separator": ","},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Value)
	// Output: api,worker
}
