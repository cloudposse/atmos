package broker

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// contextProvider fails while its context is cancelled, like a broker whose token
// exchange is interrupted, and otherwise exports env. The onProvision hook runs first.
type contextProvider struct {
	env         map[string]string
	onProvision func()
	provisioned int
}

func (c *contextProvider) Name() string                            { return "context" }
func (c *contextProvider) Enabled(*schema.AtmosConfiguration) bool { return true }
func (c *contextProvider) Provision(ctx context.Context, _ *schema.AtmosConfiguration) (map[string]string, error) {
	c.provisioned++
	if c.onProvision != nil {
		c.onProvision()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.env, nil
}

// fakeClock replaces the package clock for one test.
func fakeClock(t *testing.T) *time.Time {
	t.Helper()
	current := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	previous := now
	now = func() time.Time { return current }
	t.Cleanup(func() { now = previous })
	return &current
}

// TestEnsureCredentials_RetriesAfterCancelledFirstCall reproduces the suspected #3720
// broker finding: a first call whose context was cancelled consumed the process-wide
// sync.Once, so brokering stayed disabled for the rest of the process.
func TestEnsureCredentials_RetriesAfterCancelledFirstCall(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	const key = "ATMOS_BROKER_TEST_RETRY_AFTER_CANCEL"
	t.Setenv(key, "")
	ctx, cancel := context.WithCancel(context.Background())
	p := &contextProvider{env: map[string]string{key: "minted"}, onProvision: cancel}
	Register(p)

	EnsureCredentials(ctx, &schema.AtmosConfiguration{})
	require.Equal(t, 1, p.provisioned)
	require.False(t, HasBrokeredCredentials())

	p.onProvision = nil
	EnsureCredentials(context.Background(), &schema.AtmosConfiguration{})
	assert.Equal(t, 2, p.provisioned, "a cancelled first call must not disable brokering for the process")
	assert.Equal(t, "minted", os.Getenv(key))
	assert.True(t, HasBrokeredCredentials())
}

// TestEnsureCredentials_SkipsAlreadyCancelledCaller verifies a caller that is already
// cancelled does not provision and does not consume the run.
func TestEnsureCredentials_SkipsAlreadyCancelledCaller(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	p := &fakeProvider{name: "cancelled", enabled: true}
	Register(p)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	EnsureCredentials(ctx, &schema.AtmosConfiguration{})
	assert.Zero(t, p.provisioned, "an already-cancelled caller must not provision")

	EnsureCredentials(context.Background(), &schema.AtmosConfiguration{})
	assert.Equal(t, 1, p.provisioned)
}

// TestEnsureCredentials_RefreshesAfterInterval verifies a long-lived process re-runs the
// brokers once a successful run is older than refreshInterval, so short-lived tokens
// (GitHub installation tokens last one hour) are refreshed.
func TestEnsureCredentials_RefreshesAfterInterval(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)
	clock := fakeClock(t)

	p := &fakeProvider{name: "refresh", enabled: true}
	Register(p)
	cfg := &schema.AtmosConfiguration{}

	EnsureCredentials(context.Background(), cfg)
	*clock = clock.Add(refreshInterval - time.Second)
	EnsureCredentials(context.Background(), cfg)
	assert.Equal(t, 1, p.provisioned, "a successful run is reused within the refresh interval")

	*clock = clock.Add(time.Second)
	EnsureCredentials(context.Background(), cfg)
	assert.Equal(t, 2, p.provisioned, "a successful run older than the refresh interval must re-provision")
}

// TestEnsureCredentials_RetriesFailedRunAfterBackoff verifies a failed run is retried,
// but not on every call.
func TestEnsureCredentials_RetriesFailedRunAfterBackoff(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)
	clock := fakeClock(t)

	p := &fakeProvider{name: "flaky", enabled: true, err: errors.New("mint failed")}
	Register(p)
	cfg := &schema.AtmosConfiguration{}

	EnsureCredentials(context.Background(), cfg)
	EnsureCredentials(context.Background(), cfg)
	assert.Equal(t, 1, p.provisioned, "a failed run must not be retried on every call")

	*clock = clock.Add(retryBackoff)
	p.err = nil
	EnsureCredentials(context.Background(), cfg)
	assert.Equal(t, 2, p.provisioned, "a failed run must be retried after the backoff")

	*clock = clock.Add(retryBackoff)
	EnsureCredentials(context.Background(), cfg)
	assert.Equal(t, 2, p.provisioned, "a successful retry uses the refresh interval, not the backoff")
}

// TestSwapRegistryForTest_ResetsRunState verifies the cross-package test helper lets a
// freshly registered broker run, and restores the previous registry.
func TestSwapRegistryForTest_ResetsRunState(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	first := &fakeProvider{name: "first", enabled: true}
	Register(first)
	EnsureCredentials(context.Background(), &schema.AtmosConfiguration{})
	require.Equal(t, 1, first.provisioned)

	restore := SwapRegistryForTest()
	second := &fakeProvider{name: "second", enabled: true}
	Register(second)
	EnsureCredentials(context.Background(), &schema.AtmosConfiguration{})
	assert.Equal(t, 1, second.provisioned, "a swapped registry must run its broker")
	assert.Equal(t, 1, first.provisioned)

	restore()
	assert.Equal(t, []Provider{first}, snapshot())
	EnsureCredentials(context.Background(), &schema.AtmosConfiguration{})
	assert.Equal(t, 2, first.provisioned, "restoring resets the run state too")
}
