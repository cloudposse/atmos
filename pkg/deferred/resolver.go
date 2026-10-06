package deferred

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_resolver_test.go -package=deferred

import (
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Resolver supplies a value only when requested. Implementations own their
// dependencies and failure semantics; callers own display and recovery policy.
type Resolver[T any] interface {
	Resolve() (T, error)
}

// Func adapts a closure to a deferred value without evaluating or caching it.
type Func[T any] func() (T, error)

func (f Func[T]) Resolve() (T, error) {
	defer perf.Track(nil, "deferred.Func.Resolve")()

	return f()
}

// Once memoizes a resolver's first result, including errors. Its owner chooses
// the lifetime and cache key; a failed dependency must not silently be retried
// through a different identity. Value lookups need not opt into memoization.
func Once[T any](resolver Resolver[T]) Resolver[T] {
	defer perf.Track(nil, "deferred.Once")()

	return &once[T]{resolve: sync.OnceValues(resolver.Resolve)}
}

type once[T any] struct{ resolve func() (T, error) }

func (r *once[T]) Resolve() (T, error) {
	defer perf.Track(nil, "deferred.once.Resolve")()

	return r.resolve()
}
