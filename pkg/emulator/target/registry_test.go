package target

import (
	"testing"

	"github.com/stretchr/testify/assert"

	emu "github.com/cloudposse/atmos/pkg/emulator"
)

func TestRegistryProfile_Branches(t *testing.T) {
	t.Run("bound port sets registry host", func(t *testing.T) {
		ep := &emu.Endpoint{Target: emu.TargetRegistry, Host: "localhost", Ports: map[int]int{5000: 35000}}
		p := RegistryProfile(ep)

		assert.Equal(t, "127.0.0.1:35000", p.Env["ATMOS_REGISTRY_HOST"])
	})

	t.Run("no bound port yields empty env", func(t *testing.T) {
		ep := &emu.Endpoint{Target: emu.TargetRegistry, Host: "localhost", Ports: map[int]int{}}
		p := RegistryProfile(ep)

		// Without a live authority, the registry host key is absent.
		assert.NotContains(t, p.Env, "ATMOS_REGISTRY_HOST")
		assert.Empty(t, p.Env)
	})
}

func TestKubernetesProfile_AlwaysEmpty(t *testing.T) {
	// The kubeconfig is harvested by the identity at runtime, not built from the
	// endpoint — so the profile is empty regardless of port bindings.
	cases := map[string]*emu.Endpoint{
		"with ports": {Target: emu.TargetKubernetes, Host: "localhost", Ports: map[int]int{6443: 16443}},
		"no ports":   {Target: emu.TargetKubernetes, Host: "localhost", Ports: map[int]int{}},
	}
	for name, ep := range cases {
		t.Run(name, func(t *testing.T) {
			p := KubernetesProfile(ep)
			assert.Empty(t, p.Env)
			assert.Nil(t, p.Kubeconfig)
			assert.Nil(t, p.Provider)
			assert.Empty(t, p.ResolverURL)
		})
	}
}
