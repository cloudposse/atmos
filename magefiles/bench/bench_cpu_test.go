//go:build mage

package bench

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const armCPUInfo = `processor : 0
CPU implementer : 0x41
CPU architecture: 8
CPU variant : 0x0
CPU part : 0xd0c
CPU revision : 1
`

func TestBenchCPUIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
	}{
		{"x86", "processor: 0\nmodel name\t: AMD EPYC 7763 64-Core Processor\n", "AMD EPYC 7763 64-Core Processor"},
		{"ARM", armCPUInfo, "ARM implementer=65,architecture=8,variant=0,part=3340,revision=1"},
		{"repeated cores", armCPUInfo + "\n" + strings.Replace(armCPUInfo, "processor : 0", "processor : 1", 1), "ARM implementer=65,architecture=8,variant=0,part=3340,revision=1"},
		{"no blank separator", armCPUInfo + armCPUInfo, "ARM implementer=65,architecture=8,variant=0,part=3340,revision=1"},
		{"normalized", strings.NewReplacer("0x41", "  0X0041 ", "0xd0c", "03340", "0x0", "00").Replace(armCPUInfo), "ARM implementer=65,architecture=8,variant=0,part=3340,revision=1"},
		{"empty model", "model name : \n" + armCPUInfo, "ARM implementer=65,architecture=8,variant=0,part=3340,revision=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := benchCPUIdentity(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestBenchCPUIdentityRejectsMissingFields(t *testing.T) {
	for _, input := range []string{"", "processor: 0\nmodel name: \n", "Hardware: generic ARM\n", strings.Replace(armCPUInfo, "CPU part : 0xd0c\n", "", 1)} {
		_, err := benchCPUIdentity(input)
		require.ErrorContains(t, err, "cannot identify benchmark CPU")
	}
}

func TestBenchCPUIdentityPreservesHeterogeneousCores(t *testing.T) {
	other := strings.Replace(armCPUInfo, "0xd0c", "0xd40", 1)
	a, err := benchCPUIdentity(armCPUInfo + "\n" + other)
	require.NoError(t, err)
	b, err := benchCPUIdentity(other + "\n" + armCPUInfo)
	require.NoError(t, err)
	require.Equal(t, a, b)
	require.Contains(t, a, "part=3340")
	require.Contains(t, a, "part=3392")
}
