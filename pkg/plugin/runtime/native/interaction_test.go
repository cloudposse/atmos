package native

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/bridge"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/plugin/testkit/fakeplugin"
)

// budgetProbe starts a one-minute budget whose expiry is observable without a real clock.
func budgetProbe() (*budget, *atomic.Bool) {
	var expired atomic.Bool
	return newBudget(time.Minute, 24*time.Hour, func(bool) { expired.Store(true) }), &expired
}

func TestBudgetExpiresAfterTimeoutWithoutUserWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		work, expired := budgetProbe()
		time.Sleep(time.Minute - time.Second)
		synctest.Wait()
		require.False(t, expired.Load(), "the budget must not expire early")
		time.Sleep(time.Second)
		synctest.Wait()
		require.True(t, expired.Load(), "the budget must expire once the timeout of work has passed")
		require.NotPanics(t, work.stop)
	})
}

func TestBudgetDoesNotCountUserWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		work, expired := budgetProbe()
		defer work.stop()
		time.Sleep(40 * time.Second)
		resume := work.await()
		time.Sleep(time.Hour)
		synctest.Wait()
		require.False(t, expired.Load(), "time spent waiting for the user must not count")
		resume()
		time.Sleep(19 * time.Second)
		synctest.Wait()
		require.False(t, expired.Load(), "only the 20 seconds left before the wait remain")
		time.Sleep(time.Second)
		synctest.Wait()
		require.True(t, expired.Load(), "the remaining work time still runs out after the wait")
	})
}

func TestBudgetStaysPausedUntilEveryWaitEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		work, expired := budgetProbe()
		defer work.stop()
		first, second := work.await(), work.await()
		first()
		first() // Ending the same wait twice must not end the other one.
		time.Sleep(time.Hour)
		synctest.Wait()
		require.False(t, expired.Load(), "one open wait keeps the budget paused")
		second()
		time.Sleep(time.Minute)
		synctest.Wait()
		require.True(t, expired.Load())
	})
}

func TestBudgetUsedUpBeforeWaitExpiresInsteadOfPausing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		work, expired := budgetProbe()
		defer work.stop()
		// Model a timer callback that started but has not acquired the lock: the allowance is spent
		// while the budget still looks live.
		work.mu.Lock()
		work.since = time.Now().Add(-time.Minute)
		work.mu.Unlock()
		resume := work.await()
		synctest.Wait()
		require.True(t, expired.Load(), "a budget whose work allowance is spent must expire, not pause")
		require.False(t, work.userWaitExhausted())
		require.NotPanics(t, resume)
	})
}

func TestBudgetWaitIsBoundedByWaitLimitDespiteLargeTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var waitExpired atomic.Bool
		work := newBudget(30*time.Minute, time.Minute, func(wait bool) { waitExpired.Store(wait) })
		defer work.stop()
		_ = work.await()
		time.Sleep(time.Minute - time.Second)
		synctest.Wait()
		require.False(t, waitExpired.Load())
		time.Sleep(time.Second)
		synctest.Wait()
		require.True(t, waitExpired.Load(), "the user wait must end at the wait limit, not the work timeout")
		require.True(t, work.userWaitExhausted())
	})
}

func TestBudgetWaitLimitAccumulatesAcrossWaits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var waitExpired atomic.Bool
		work := newBudget(30*time.Minute, time.Minute, func(wait bool) { waitExpired.Store(wait) })
		defer work.stop()
		work.await()()
		first := work.await()
		time.Sleep(40 * time.Second)
		first()
		_ = work.await()
		time.Sleep(19 * time.Second)
		synctest.Wait()
		require.False(t, waitExpired.Load(), "only 59 seconds of waiting so far")
		time.Sleep(time.Second)
		synctest.Wait()
		require.True(t, waitExpired.Load(), "cumulative waits reaching the limit end the call")
	})
}

func TestStoppedBudgetNeverExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		work, expired := budgetProbe()
		resume := work.await()
		work.stop()
		resume()
		require.NotNil(t, work.await(), "a wait on a finished budget is a no-op")
		time.Sleep(time.Hour)
		synctest.Wait()
		require.False(t, expired.Load(), "a stopped budget must not cancel a finished call")
	})
}

func TestAwaitUserIgnoresUnknownInvocation(t *testing.T) {
	handle := &Handle{}
	resume := handle.AwaitUser("missing")
	require.NotNil(t, resume)
	require.NotPanics(t, resume)
}

// runInteractiveHang starts an interactive hanging command and optionally opens a user wait
// before it hangs, returning the invocation the plugin received and the call's error.
func runInteractiveHang(t *testing.T, ctx context.Context, handle *Handle, waitForUser bool) (*pluginv1.Invocation, error) {
	t.Helper()
	var seen *pluginv1.Invocation
	err := handle.Invoke(ctx, bridge.Invocation{Namespace: "test", Interactive: true}, func(ctx context.Context, invocation *pluginv1.Invocation) error {
		seen = invocation
		if waitForUser {
			defer handle.AwaitUser(invocation.GetId())()
		}
		return handle.Command(ctx, &pluginv1.CommandRequest{Invocation: invocation, Name: "hang"}, nopWriter{}, nopWriter{})
	})
	return seen, err
}

func TestInteractiveCallTimeoutStopsHungPluginBeforeHardDeadline(t *testing.T) {
	const timeout = 500 * time.Millisecond
	handle, directory := startFake(t, t.Context(), fakeplugin.ModeHang, WithCallTimeout(timeout), WithInteractionLimit(time.Hour))
	started := time.Now()

	invocation, err := runInteractiveHang(t, t.Context(), handle, false)

	require.ErrorIs(t, err, errUtils.ErrPluginTimeout)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), eventually, "the budget, not the hour-long hard deadline, stops the plugin")
	require.NotContains(t, strings.Join(errUtils.AllHints(err), "\n"), "sign-in", "a hung plugin is not a slow sign-in")
	require.NotNil(t, invocation.GetDeadline())
	require.WithinDuration(t, started.Add(timeout+time.Hour), invocation.GetDeadline().AsTime(), time.Minute,
		"the plugin is told the hard deadline: the timeout plus the interaction limit")
	pids, pidErr := fakeplugin.PIDs(directory)
	require.NoError(t, pidErr)
	requireGone(t, pids)
}

func TestInteractiveUserWaitIsBoundedByInteractionLimit(t *testing.T) {
	const timeout, limit = 300 * time.Millisecond, time.Second
	handle, directory := startFake(t, t.Context(), fakeplugin.ModeHang, WithCallTimeout(timeout), WithInteractionLimit(limit), WithPackageAlias("corporate"))
	started := time.Now()

	_, err := runInteractiveHang(t, t.Context(), handle, true)

	require.ErrorIs(t, err, errUtils.ErrPluginTimeout)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.GreaterOrEqual(t, time.Since(started), limit, "the user wait lasts the whole interaction limit")
	hints := strings.Join(errUtils.AllHints(err), "\n")
	require.Contains(t, hints, "sign-in", "the error explains the sign-in was not finished")
	require.Contains(t, hints, "`plugins.packages.corporate.interaction_timeout`", "the hint names the key that raises the limit")
	pids, pidErr := fakeplugin.PIDs(directory)
	require.NoError(t, pidErr)
	requireGone(t, pids)
}

func TestInteractiveUserWaitEndsAtInteractionLimitDespiteLargeTimeout(t *testing.T) {
	const limit = 500 * time.Millisecond
	handle, directory := startFake(t, t.Context(), fakeplugin.ModeHang, WithCallTimeout(30*time.Minute), WithInteractionLimit(limit), WithPackageAlias("corporate"))
	started := time.Now()

	_, err := runInteractiveHang(t, t.Context(), handle, true)

	require.ErrorIs(t, err, errUtils.ErrPluginTimeout)
	require.Less(t, time.Since(started), eventually, "the wait ends at the interaction limit, not the 30 minute timeout")
	require.Contains(t, strings.Join(errUtils.AllHints(err), "\n"), "`plugins.packages.corporate.interaction_timeout`")
	pids, pidErr := fakeplugin.PIDs(directory)
	require.NoError(t, pidErr)
	requireGone(t, pids)
}

func TestInteractiveCallerDeadlineKeepsGenericTimeoutHint(t *testing.T) {
	handle, _ := startFake(t, t.Context(), fakeplugin.ModeHang, WithCallTimeout(time.Minute))
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	_, err := runInteractiveHang(t, ctx, handle, true)

	require.ErrorIs(t, err, errUtils.ErrPluginTimeout)
	require.NotContains(t, strings.Join(errUtils.AllHints(err), "\n"), "sign-in",
		"a deadline the caller set is not the interaction limit")
}

func TestNoninteractiveCallKeepsCallTimeoutDeadline(t *testing.T) {
	// A configured interaction limit must not stretch a call that can never wait for a person.
	handle, _ := startFake(t, t.Context(), fakeplugin.ModeEcho, WithCallTimeout(30*time.Minute), WithInteractionLimit(time.Hour))
	var seen *pluginv1.Invocation
	started := time.Now()
	require.NoError(t, handle.Invoke(t.Context(), bridge.Invocation{Namespace: "test"}, func(_ context.Context, invocation *pluginv1.Invocation) error {
		seen = invocation
		resume := handle.AwaitUser(invocation.GetId())
		resume()
		return nil
	}))
	require.WithinDuration(t, started.Add(30*time.Minute), seen.GetDeadline().AsTime(), time.Minute,
		"a noninteractive call cannot wait for a person, so its deadline stays the timeout")
	_, tracked := handle.budgets.Load(seen.GetId())
	require.False(t, tracked)
}

// TestConfiguredInteractionLimitAllowsSignInLongerThanDefault proves a larger limit lets a user
// take longer than DefaultInteractionLimit, while the default still cuts the same wait off.
func TestConfiguredInteractionLimitAllowsSignInLongerThanDefault(t *testing.T) {
	const timeout = time.Minute
	userWait := DefaultInteractionLimit + 10*time.Minute
	cases := []struct {
		name    string
		limit   time.Duration
		expired bool
	}{
		{"default limit ends the wait", DefaultInteractionLimit, true},
		{"larger limit allows it", 45 * time.Minute, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				settings := options{interactionLimit: DefaultInteractionLimit}
				WithInteractionLimit(testCase.limit)(&settings)
				handle := &Handle{callTimeout: timeout, interactionLimit: settings.interactionLimit}
				started := time.Now()
				callCtx, work, cancel := handle.callContext(t.Context(), true)
				defer cancel()
				require.NotNil(t, work)
				deadline, ok := callCtx.Deadline()
				require.True(t, ok)
				require.Equal(t, started.Add(timeout+testCase.limit), deadline, "the hard deadline is the timeout plus the limit")

				resume := work.await()
				time.Sleep(userWait)
				synctest.Wait()
				if testCase.expired {
					require.ErrorIs(t, context.Cause(callCtx), errUtils.ErrPluginTimeout, "the default limit ends a 25-minute sign-in")
					resume()
					return
				}
				require.NoError(t, callCtx.Err(), "a 25-minute sign-in fits a 45-minute limit")
				resume()
				time.Sleep(timeout - time.Second)
				synctest.Wait()
				require.NoError(t, callCtx.Err(), "the plugin keeps its full work budget after the user finishes")
				time.Sleep(time.Second)
				synctest.Wait()
				require.ErrorIs(t, context.Cause(callCtx), errUtils.ErrPluginTimeout, "the work budget still bounds the plugin")
			})
		})
	}
}

func TestInteractionTimeoutErrorNamesTheConfigurationKey(t *testing.T) {
	deadline := time.Date(2026, time.November, 22, 12, 0, 0, 0, time.UTC)
	err := interactionTimeoutError(deadline, 15*time.Minute, "corporate")
	require.ErrorIs(t, err, errUtils.ErrPluginTimeout)
	hints := strings.Join(errUtils.AllHints(err), "\n")
	require.Contains(t, hints, "15m0s")
	require.Contains(t, hints, "`plugins.packages.corporate.interaction_timeout`")

	// Without a known alias the hint still names the key, with a placeholder for the package.
	hints = strings.Join(errUtils.AllHints(interactionTimeoutError(deadline, time.Minute, "")), "\n")
	require.Contains(t, hints, "`plugins.packages.<alias>.interaction_timeout`")
}

func TestWithPackageAliasRecordsTheAlias(t *testing.T) {
	var settings options
	WithPackageAlias("corporate")(&settings)
	require.Equal(t, "corporate", settings.packageAlias)
}

func TestWithInteractionLimitIgnoresNonPositive(t *testing.T) {
	settings := options{interactionLimit: DefaultInteractionLimit}
	WithInteractionLimit(0)(&settings)
	WithInteractionLimit(-time.Second)(&settings)
	require.Equal(t, DefaultInteractionLimit, settings.interactionLimit)
	WithInteractionLimit(time.Minute)(&settings)
	require.Equal(t, time.Minute, settings.interactionLimit)
}
