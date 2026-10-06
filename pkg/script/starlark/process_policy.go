package starlark

import (
	"time"

	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/automation"
)

func (s *session) processPolicy(thread *starlark.Thread, timeout string, retryValue starlark.Value) (automation.ExecutionPolicy, error) {
	policy := automation.ExecutionPolicy{Clock: s.engine.clock}
	if timeout != "" {
		duration, err := time.ParseDuration(timeout)
		if err != nil || duration <= 0 {
			return policy, invalidArg("timeout must be a positive duration")
		}
		policy.Timeout = duration
	}
	var err error
	policy.Retry, err = parseRetryConfig(thread, retryValue)
	return policy, err
}
