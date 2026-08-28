package utils_test

import (
	"sync"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// TestYqConcurrentEvaluate is the regression test for #2821. Before the
// shared evaluator existed, these concurrent calls raced in both go-logging
// and yq's expression parser.
func TestYqConcurrentEvaluate(t *testing.T) {
	config := &schema.AtmosConfiguration{Logs: schema.Logs{Level: "Info"}}
	data := map[string]any{"a": map[string]any{"b": "value"}}

	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)

		go func() {
			defer wg.Done()
			result, err := utils.EvaluateYqExpression(config, data, ".a.b")
			if err != nil {
				t.Error(err)
				return
			}
			if result != "value" {
				t.Errorf("EvaluateYqExpression() = %v, want value", result)
			}
		}()
	}
	wg.Wait()
}

// TestYqConcurrentEvaluationAcrossCallers verifies that the stack/include
// evaluation path and the YAML editor share the synchronization required by
// yq and go-logging.
func TestYqConcurrentEvaluationAcrossCallers(t *testing.T) {
	config := &schema.AtmosConfiguration{Logs: schema.Logs{Level: "Info"}}
	data := map[string]any{"a": map[string]any{"b": "value"}}
	content := []byte("a:\n  b: value\n")

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(2)

		go func() {
			defer wg.Done()
			result, err := utils.EvaluateYqExpression(config, data, ".a.b")
			if err != nil {
				t.Error(err)
				return
			}
			if result != "value" {
				t.Errorf("EvaluateYqExpression() = %v, want value", result)
			}
		}()

		go func() {
			defer wg.Done()
			result, err := atmosyaml.Query(content, ".a.b")
			if err != nil {
				t.Error(err)
				return
			}
			if result != "value" {
				t.Errorf("Query() = %q, want value", result)
			}
		}()
	}
	wg.Wait()
}
