package starlark

import (
	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/automation"
	durationpkg "github.com/cloudposse/atmos/pkg/duration"
)

func (s *session) processPolicy(thread *starlark.Thread, timeout string, retryValue starlark.Value) (automation.ExecutionPolicy, error) {
	policy := automation.ExecutionPolicy{Clock: s.engine.clock}
	if timeout != "" {
		duration, err := durationpkg.ParseString(timeout)
		if err != nil || duration <= 0 {
			return policy, invalidArg("timeout must be a positive duration")
		}
		policy.Timeout = duration
	}
	var err error
	policy.Retry, err = parseRetryConfig(thread, retryValue)
	return policy, err
}
