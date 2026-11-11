package netaddr

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go4.org/netipx"
)

func ip(t *testing.T, s string) IP {
	t.Helper()
	return FromAddr(netip.MustParseAddr(s))
}

func prefix(t *testing.T, s string) IPPrefix {
	t.Helper()
	return FromPrefix(netip.MustParsePrefix(s))
}

func ipRange(t *testing.T, from, to string) IPRange {
	t.Helper()
	return FromRange(netipx.IPRangeFrom(netip.MustParseAddr(from), netip.MustParseAddr(to)))
}

func TestIPZeroValue(t *testing.T) {
	var zero IP
	assert.True(t, zero.IsZero())
	assert.False(t, zero.IsValid())
	assert.Equal(t, uint8(0), zero.BitLen())
	assert.Equal(t, "invalid IP", zero.String())
	assert.Equal(t, "invalid IP", zero.StringExpanded())
	text, err := zero.MarshalText()
	require.NoError(t, err)
	assert.Empty(t, text)
	assert.Nil(t, zero.IPAddr().IP)
	p, err := zero.Prefix(8)
	require.NoError(t, err)
	assert.True(t, p.IsZero())
}

func TestIPMethods(t *testing.T) {
	v4 := ip(t, "192.0.2.1")
	assert.True(t, v4.Is4())
	assert.False(t, v4.Is6())
	assert.Equal(t, uint8(32), v4.BitLen())
	assert.Equal(t, [4]byte{192, 0, 2, 1}, v4.As4())
	assert.Equal(t, "192.0.2.1", v4.IPAddr().String())
	assert.Equal(t, "192.0.2.2", v4.Next().String())
	assert.Equal(t, "192.0.2.0", v4.Prior().String())
	assert.Equal(t, 0, v4.Compare(ip(t, "192.0.2.1")))
	assert.Equal(t, 1, v4.Compare(ip(t, "192.0.2.0")))
	assert.False(t, v4.Less(ip(t, "192.0.2.0")))
	assert.Equal(t, "192.0.2.1", string(v4.AppendTo(nil)))

	mapped := ip(t, "::ffff:192.0.2.1")
	assert.True(t, mapped.Is4in6())
	assert.True(t, mapped.Is6())
	assert.Equal(t, "192.0.2.1", mapped.Unmap().String())

	zoned := ip(t, "fe80::1").WithZone("eth0")
	assert.Equal(t, "eth0", zoned.Zone())
	assert.Equal(t, "fe80::1%eth0", zoned.IPAddr().String())
	assert.Equal(t, v4, v4.WithZone("eth0"), "IPv4 addresses have no zone")
	assert.Equal(t, "fe80:0000:0000:0000:0000:0000:0000:0001", ip(t, "fe80::1").StringExpanded())

	assert.True(t, ip(t, "10.0.0.1").IsPrivate())
	assert.True(t, ip(t, "127.0.0.1").IsLoopback())
	assert.True(t, ip(t, "0.0.0.0").IsUnspecified())
}

func TestIPBoundaries(t *testing.T) {
	assert.True(t, ip(t, "255.255.255.255").Next().IsZero())
	assert.True(t, ip(t, "0.0.0.0").Prior().IsZero())
	assert.True(t, ip(t, "::").Prior().IsZero())
}

func TestIPPrefixAndNetmask(t *testing.T) {
	v4 := ip(t, "192.0.2.77")
	p, err := v4.Prefix(24)
	require.NoError(t, err)
	assert.Equal(t, "192.0.2.0/24", p.String())

	_, err = v4.Prefix(33)
	require.Error(t, err)

	p, err = v4.Netmask([]byte{255, 255, 255, 0})
	require.NoError(t, err)
	assert.Equal(t, "192.0.2.0/24", p.String())

	_, err = v4.Netmask([]byte{255, 255, 0})
	require.ErrorIs(t, err, errNetmaskLength)
	require.ErrorContains(t, err, "for IPv4")
	_, err = ip(t, "2001:db8::1").Netmask([]byte{255, 255, 255, 0})
	require.ErrorIs(t, err, errNetmaskLength)
	require.ErrorContains(t, err, "for IPv6")
	_, err = v4.Netmask([]byte{255, 0, 255, 0})
	require.ErrorIs(t, err, errNonContiguousNetmask)
}

func TestIPPrefixMethods(t *testing.T) {
	p := prefix(t, "192.0.2.1/24")
	assert.Equal(t, "192.0.2.1", p.IP().String(), "the address is kept as written")
	assert.Equal(t, uint8(24), p.Bits())
	assert.True(t, p.IsValid())
	assert.True(t, p.Valid())
	assert.False(t, p.IsZero())
	assert.False(t, p.IsSingleIP())
	assert.Equal(t, "192.0.2.0/24", p.Masked().String())
	assert.Equal(t, "192.0.2.0-192.0.2.255", p.Range().String())
	assert.Equal(t, "192.0.2.1/24", p.IPNet().String())
	assert.True(t, p.Contains(ip(t, "192.0.2.200")))
	assert.False(t, p.Contains(ip(t, "198.51.100.1")))
	assert.True(t, p.Overlaps(prefix(t, "192.0.2.128/25")))
	assert.False(t, p.Overlaps(prefix(t, "198.51.100.0/24")))
	assert.True(t, prefix(t, "192.0.2.1/32").IsSingleIP())
	text, err := p.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "192.0.2.1/24", string(text))
	assert.Equal(t, "192.0.2.1/24", string(p.AppendTo(nil)))
}

func TestIPPrefixZeroValue(t *testing.T) {
	var zero IPPrefix
	assert.True(t, zero.IsZero())
	assert.False(t, zero.IsValid())
	assert.Equal(t, "invalid IPPrefix", zero.String())
	assert.True(t, zero.Range().IsZero())
	assert.Nil(t, zero.IPNet().IP)
	assert.False(t, zero.Contains(ip(t, "192.0.2.1")))
	assert.False(t, zero.Overlaps(prefix(t, "192.0.2.0/24")))
	assert.True(t, zero.Masked().IsZero())
	text, err := zero.MarshalText()
	require.NoError(t, err)
	assert.Empty(t, text)
	assert.True(t, FromPrefix(netip.Prefix{}).IsZero())
}

func TestIPRangeMethods(t *testing.T) {
	r := ipRange(t, "192.0.2.1", "192.0.2.3")
	assert.Equal(t, "192.0.2.1", r.From().String())
	assert.Equal(t, "192.0.2.3", r.To().String())
	assert.Equal(t, "192.0.2.1-192.0.2.3", r.String())
	assert.True(t, r.IsValid())
	assert.True(t, r.Valid())
	assert.False(t, r.IsZero())
	assert.True(t, r.Contains(ip(t, "192.0.2.2")))
	assert.False(t, r.Contains(ip(t, "192.0.2.4")))
	assert.True(t, r.Overlaps(ipRange(t, "192.0.2.3", "192.0.2.9")))
	assert.False(t, r.Overlaps(ipRange(t, "192.0.2.4", "192.0.2.9")))

	prefixes := r.Prefixes()
	require.Len(t, prefixes, 2)
	assert.Equal(t, "192.0.2.1/32", prefixes[0].String())
	assert.Equal(t, "192.0.2.2/31", prefixes[1].String())
	_, ok := r.Prefix()
	assert.False(t, ok, "1-3 is not a single prefix")

	single, ok := ipRange(t, "192.0.2.0", "192.0.2.255").Prefix()
	require.True(t, ok)
	assert.Equal(t, "192.0.2.0/24", single.String())

	text, err := r.MarshalText()
	require.NoError(t, err)
	assert.Equal(t, "192.0.2.1-192.0.2.3", string(text))
	assert.Equal(t, "192.0.2.1-192.0.2.3", string(r.AppendTo(nil)))
}

func TestIPRangeInvalid(t *testing.T) {
	var zero IPRange
	assert.True(t, zero.IsZero())
	assert.False(t, zero.IsValid())
	assert.Equal(t, "invalid IPRange", zero.String())
	assert.Empty(t, zero.Prefixes())
	_, ok := zero.Prefix()
	assert.False(t, ok)
	text, err := zero.MarshalText()
	require.NoError(t, err)
	assert.Empty(t, text)

	backwards := ipRange(t, "192.0.2.9", "192.0.2.1")
	assert.False(t, backwards.IsValid())
	assert.False(t, backwards.Contains(ip(t, "192.0.2.5")))
	assert.False(t, backwards.Overlaps(ipRange(t, "192.0.2.1", "192.0.2.9")))
}

func TestBitCountRejectsOutOfRange(t *testing.T) {
	assert.Equal(t, uint8(24), bitCount(24))
	assert.Equal(t, uint8(0), bitCount(-1))
	assert.Equal(t, uint8(0), bitCount(256))
}
