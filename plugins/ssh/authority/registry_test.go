package authority_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/authority/authoritytest"
)

func fakeFactory(name string) authority.Factory {
	return authority.Factory{Name: name, New: func(authority.Options) (authority.Authority, error) {
		return authoritytest.NewFake(name), nil
	}}
}

func TestRegistry(t *testing.T) {
	r := authority.NewRegistry()
	require.NoError(t, r.Register(fakeFactory("beta")))
	require.NoError(t, r.Register(fakeFactory("alpha")))

	assert.Equal(t, []string{"alpha", "beta"}, r.Names())
	a, err := r.New("beta", authority.Options{})
	require.NoError(t, err)
	assert.Equal(t, "beta", a.Name())
	_, ok := r.Lookup("alpha")
	assert.True(t, ok)
	_, ok = r.Lookup("gamma")
	assert.False(t, ok)
}

func TestRegistryErrors(t *testing.T) {
	r := authority.NewRegistry()
	require.NoError(t, r.Register(fakeFactory("alpha")))

	tests := []struct {
		name string
		do   func() error
		want error
	}{
		{"duplicate", func() error { return r.Register(fakeFactory("alpha")) }, authority.ErrDuplicateAuthority},
		{"no name", func() error { return r.Register(authority.Factory{New: fakeFactory("x").New}) }, authority.ErrInvalidRequest},
		{"no constructor", func() error { return r.Register(authority.Factory{Name: "x"}) }, authority.ErrInvalidRequest},
		{"unknown", func() error { _, err := r.New("nope", authority.Options{}); return err }, authority.ErrUnknownAuthority},
		{"constructor fails", func() error {
			require.NoError(t, r.Register(authority.Factory{Name: "bad", New: func(authority.Options) (authority.Authority, error) {
				return nil, authority.ErrInvalidRequest
			}}))
			_, err := r.New("bad", authority.Options{})
			return err
		}, authority.ErrInvalidRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { assert.ErrorIs(t, tt.do(), tt.want) })
	}
	assert.Equal(t, []string{"alpha", "bad"}, r.Names(), "failed registrations changed nothing")
}

func TestRegistryIsSafeForConcurrentUse(t *testing.T) {
	r := authority.NewRegistry()
	var wg sync.WaitGroup
	for _, name := range []string{"a", "b", "c", "d"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Register(fakeFactory(name))
			_ = r.Names()
		}()
	}
	wg.Wait()
	assert.Len(t, r.Names(), 4)
}

func TestDefaultRegistry(t *testing.T) {
	// The Default registry is process-wide, so the name must be new on every run (-count=N).
	name := fmt.Sprintf("default-registry-test-%d", time.Now().UnixNano())
	require.NoError(t, authority.Register(fakeFactory(name)))
	assert.Contains(t, authority.Names(), name)
	a, err := authority.New(name, authority.Options{})
	require.NoError(t, err)
	assert.Equal(t, name, a.Name())
}
