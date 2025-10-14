package list

import (
	"fmt"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

// BenchmarkCollectInstances measures performance of instance collection from stacks map.
func BenchmarkCollectInstances(b *testing.B) {
	// Create a realistic stacks map with 10 stacks, each with 5 components.
	stacksMap := make(map[string]interface{})
	for i := 0; i < 10; i++ {
		stackName := fmt.Sprintf("stack-%d", i)
		stacksMap[stackName] = map[string]interface{}{
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"vpc":      map[string]interface{}{"metadata": map[string]interface{}{"type": "real"}},
					"eks":      map[string]interface{}{"metadata": map[string]interface{}{"type": "real"}},
					"rds":      map[string]interface{}{"metadata": map[string]interface{}{"type": "real"}},
					"s3":       map[string]interface{}{"metadata": map[string]interface{}{"type": "real"}},
					"dynamodb": map[string]interface{}{"metadata": map[string]interface{}{"type": "real"}},
				},
			},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = collectInstances(stacksMap)
	}
}

// BenchmarkSortInstances measures performance of instance sorting.
func BenchmarkSortInstances(b *testing.B) {
	// Create 100 instances with varied stack and component names.
	instances := make([]schema.Instance, 100)
	for i := 0; i < 100; i++ {
		instances[i] = schema.Instance{
			Component: fmt.Sprintf("component-%d", i%26),
			Stack:     fmt.Sprintf("stack-%d", i%10),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Make a copy to avoid sorting the same slice repeatedly.
		instancesCopy := make([]schema.Instance, len(instances))
		copy(instancesCopy, instances)
		_ = sortInstances(instancesCopy)
	}
}

// BenchmarkFilterProEnabledInstances measures performance of filtering Pro-enabled instances.
func BenchmarkFilterProEnabledInstances(b *testing.B) {
	// Create 100 instances, half with Pro enabled, half without.
	instances := make([]schema.Instance, 100)
	for i := 0; i < 100; i++ {
		instance := schema.Instance{
			Component: "component-" + string(rune('a'+i%26)),
			Stack:     "stack-" + string(rune('a'+i%10)),
			Settings:  make(map[string]any),
		}

		// Enable Pro for every other instance.
		if i%2 == 0 {
			instance.Settings["pro"] = map[string]any{
				"drift_detection": map[string]any{
					"enabled": true,
				},
			}
		}

		instances[i] = instance
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filterProEnabledInstances(instances)
	}
}

// BenchmarkProcessComponentConfig measures performance of component config processing.
func BenchmarkProcessComponentConfig(b *testing.B) {
	componentConfig := map[string]any{
		"settings": map[string]any{"key": "value"},
		"vars":     map[string]any{"region": "us-east-1"},
		"env":      map[string]any{"ENV": "dev"},
		"backend":  map[string]any{"bucket": "state-bucket"},
		"metadata": map[string]any{"type": "real"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = processComponentConfig("stack1", "vpc", "terraform", componentConfig)
	}
}

// BenchmarkCreateInstance measures performance of instance creation.
func BenchmarkCreateInstance(b *testing.B) {
	componentConfigMap := map[string]any{
		"settings": map[string]any{"key": "value"},
		"vars":     map[string]any{"region": "us-east-1"},
		"env":      map[string]any{"ENV": "dev"},
		"backend":  map[string]any{"bucket": "state-bucket"},
		"metadata": map[string]any{"type": "real"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = createInstance("stack1", "vpc", "terraform", componentConfigMap)
	}
}

// BenchmarkIsProDriftDetectionEnabled measures performance of Pro drift detection check.
func BenchmarkIsProDriftDetectionEnabled(b *testing.B) {
	instance := &schema.Instance{
		Component: "vpc",
		Stack:     "dev",
		Settings: map[string]interface{}{
			"pro": map[string]interface{}{
				"drift_detection": map[string]interface{}{
					"enabled": true,
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = isProDriftDetectionEnabled(instance)
	}
}
