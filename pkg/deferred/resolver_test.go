package deferred

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestOnceDefersAndSharesSuccessOrFailure(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failure], func(t *testing.T) {
			dependency := NewMockResolver[string](gomock.NewController(t))
			var wantErr error
			if failure {
				wantErr = errors.New("dependency unavailable")
			}
			calls := 0
			dependency.EXPECT().Resolve().DoAndReturn(func() (string, error) {
				calls++
				return "resolved", wantErr
			}).Times(1)
			value := Once[string](dependency)
			require.Zero(t, calls, "constructing a deferred value must not evaluate it")
			const readers = 16
			results := make([]string, readers)
			errs := make([]error, readers)
			var wg sync.WaitGroup
			for i := range readers {
				wg.Go(func() { results[i], errs[i] = value.Resolve() })
			}
			wg.Wait()
			for i := range readers {
				require.Equal(t, "resolved", results[i])
				require.ErrorIs(t, errs[i], wantErr)
			}
			require.Equal(t, 1, calls)
		})
	}
}

func TestFuncDoesNotImplicitlyCache(t *testing.T) {
	calls := 0
	var value Resolver[int] = Func[int](func() (int, error) {
		calls++
		return calls, nil
	})
	require.Zero(t, calls)
	for _, want := range []int{1, 2} {
		got, err := value.Resolve()
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}
