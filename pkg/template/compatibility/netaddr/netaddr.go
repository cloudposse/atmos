// Package netaddr reproduces the inet.af/netaddr IP, IPPrefix and IPRange
// values that gomplate v3's net.ParseIP, net.ParseIPPrefix and
// net.ParseIPRange returned. Templates written for v3 call their methods
// directly and pass them to each other (for example
// `(net.ParseIPPrefix "10.0.0.0/8").Contains (net.ParseIP "10.1.2.3")`), so the
// compatibility shims return these types instead of net/netip values. The
// package name matches inet.af/netaddr so `printf "%T"` output is unchanged.
//
// The values are backed by net/netip and go4.org/netipx; inet.af/netaddr is no
// longer maintained.
package netaddr

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/netip"

	"go4.org/netipx"
)

var (
	errNonContiguousNetmask = errors.New("netmask is non-contiguous")
	errNetmaskLength        = errors.New("netmask length incorrect")
)

// bitCount converts a prefix length or bit length, always within 0-128, to the
// uint8 netaddr uses; out-of-range values become 0.
func bitCount(n int) uint8 {
	if n < 0 || n > math.MaxUint8 {
		return 0
	}
	return uint8(n)
}

// IP is an IPv4 or IPv6 address, possibly with an IPv6 zone. The zero value is
// not a valid address.
type IP struct {
	addr netip.Addr
}

// IPPrefix is an IP address and a prefix length (CIDR notation), with the
// address kept as written.
type IPPrefix struct {
	ip   IP
	bits uint8
}

// IPRange is an inclusive range of IP addresses.
type IPRange struct {
	from IP
	to   IP
}

// FromAddr wraps a net/netip address.
func FromAddr(addr netip.Addr) IP { return IP{addr: addr} }

// FromPrefix wraps a net/netip prefix, keeping its address as written.
func FromPrefix(prefix netip.Prefix) IPPrefix {
	if !prefix.IsValid() {
		return IPPrefix{}
	}
	return IPPrefix{ip: FromAddr(prefix.Addr()), bits: bitCount(prefix.Bits())}
}

// FromRange wraps a go4.org/netipx range.
func FromRange(r netipx.IPRange) IPRange {
	return IPRange{from: FromAddr(r.From()), to: FromAddr(r.To())}
}

// IsZero reports whether ip is the zero value.
func (ip IP) IsZero() bool { return !ip.addr.IsValid() }

// IsValid reports whether ip is a valid address (not the zero value).
func (ip IP) IsValid() bool { return ip.addr.IsValid() }

// BitLen returns 32 for IPv4, 128 for IPv6, and 0 for the zero value.
func (ip IP) BitLen() uint8 { return bitCount(ip.addr.BitLen()) }

// Zone returns ip's IPv6 scoped addressing zone, if any.
func (ip IP) Zone() string { return ip.addr.Zone() }

// Compare returns -1, 0 or 1 as ip sorts before, equal to or after ip2.
func (ip IP) Compare(ip2 IP) int { return ip.addr.Compare(ip2.addr) }

// Less reports whether ip sorts before ip2.
func (ip IP) Less(ip2 IP) bool { return ip.Compare(ip2) == -1 }

// IPAddr returns ip as a net.IPAddr.
func (ip IP) IPAddr() *net.IPAddr {
	return &net.IPAddr{IP: ip.std(), Zone: ip.addr.Zone()}
}

// Is4 reports whether ip is an IPv4 address.
func (ip IP) Is4() bool { return ip.addr.Is4() }

// Is4in6 reports whether ip is an IPv4-mapped IPv6 address.
func (ip IP) Is4in6() bool { return ip.addr.Is4In6() }

// Is6 reports whether ip is an IPv6 address, including IPv4-mapped ones.
func (ip IP) Is6() bool { return ip.addr.Is6() }

// Unmap returns ip with any IPv4-mapped IPv6 prefix removed.
func (ip IP) Unmap() IP { return FromAddr(ip.addr.Unmap()) }

// WithZone returns ip with the given IPv6 zone; IPv4 addresses are unchanged.
func (ip IP) WithZone(zone string) IP {
	if !ip.addr.Is6() {
		return ip
	}
	return FromAddr(ip.addr.WithZone(zone))
}

// IsLinkLocalUnicast reports whether ip is a link-local unicast address.
func (ip IP) IsLinkLocalUnicast() bool { return ip.addr.IsLinkLocalUnicast() }

// IsLoopback reports whether ip is a loopback address.
func (ip IP) IsLoopback() bool { return ip.addr.IsLoopback() }

// IsMulticast reports whether ip is a multicast address.
func (ip IP) IsMulticast() bool { return ip.addr.IsMulticast() }

// IsInterfaceLocalMulticast reports whether ip is an IPv6 interface-local multicast address.
func (ip IP) IsInterfaceLocalMulticast() bool { return ip.addr.IsInterfaceLocalMulticast() }

// IsLinkLocalMulticast reports whether ip is a link-local multicast address.
func (ip IP) IsLinkLocalMulticast() bool { return ip.addr.IsLinkLocalMulticast() }

// IsGlobalUnicast reports whether ip is a global unicast address.
func (ip IP) IsGlobalUnicast() bool { return ip.addr.IsGlobalUnicast() }

// IsPrivate reports whether ip is in a private address range (RFC 1918 or RFC 4193).
func (ip IP) IsPrivate() bool { return ip.addr.IsPrivate() }

// IsUnspecified reports whether ip is 0.0.0.0 or ::.
func (ip IP) IsUnspecified() bool { return ip.addr.IsUnspecified() }

// Prefix returns the prefix of ip with the given length, with host bits cleared.
func (ip IP) Prefix(bits uint8) (IPPrefix, error) {
	if ip.IsZero() {
		return IPPrefix{}, nil
	}
	prefix, err := ip.addr.Prefix(int(bits))
	if err != nil {
		return IPPrefix{}, err
	}
	return FromPrefix(prefix), nil
}

// Netmask returns the prefix of ip for a netmask in byte form.
func (ip IP) Netmask(mask []byte) (IPPrefix, error) {
	switch {
	case ip.IsZero():
		return IPPrefix{}, nil
	case ip.Is4() && len(mask) != net.IPv4len:
		return IPPrefix{}, fmt.Errorf("%w: %d for IPv4", errNetmaskLength, len(mask))
	case ip.Is6() && len(mask) != net.IPv6len:
		return IPPrefix{}, fmt.Errorf("%w: %d for IPv6", errNetmaskLength, len(mask))
	}
	ones, bits := net.IPMask(mask).Size()
	if ones == 0 && bits == 0 {
		return IPPrefix{}, errNonContiguousNetmask
	}
	return ip.Prefix(bitCount(ones))
}

// As16 returns ip in its 16-byte representation.
func (ip IP) As16() [net.IPv6len]byte { return ip.addr.As16() }

// As4 returns an IPv4 or IPv4-mapped IPv6 address in its 4-byte representation.
func (ip IP) As4() [net.IPv4len]byte { return ip.addr.As4() }

// Next returns the address following ip, or the zero value at the end of the address space.
func (ip IP) Next() IP {
	next := ip.addr.Next()
	if !next.IsValid() {
		return IP{}
	}
	return FromAddr(next)
}

// Prior returns the address before ip, or the zero value at the start of the address space.
func (ip IP) Prior() IP {
	prev := ip.addr.Prev()
	if !prev.IsValid() {
		return IP{}
	}
	return FromAddr(prev)
}

// String returns ip in its canonical form, or "invalid IP" for the zero value.
func (ip IP) String() string {
	if ip.IsZero() {
		return "invalid IP"
	}
	return ip.addr.String()
}

// AppendTo appends the text form of ip to b.
func (ip IP) AppendTo(b []byte) []byte { return append(b, ip.textString()...) }

// StringExpanded is like String but writes IPv6 addresses in full.
func (ip IP) StringExpanded() string {
	if ip.IsZero() {
		return "invalid IP"
	}
	return ip.addr.StringExpanded()
}

// MarshalText implements encoding.TextMarshaler; the zero value encodes as "".
func (ip IP) MarshalText() ([]byte, error) { return []byte(ip.textString()), nil }

// textString returns the text form of ip, or "" for the zero value.
func (ip IP) textString() string {
	if ip.IsZero() {
		return ""
	}
	return ip.addr.String()
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (ip IP) MarshalBinary() ([]byte, error) { return ip.addr.MarshalBinary() }

func (ip IP) std() net.IP {
	if ip.IsZero() {
		return nil
	}
	if ip.addr.Is4() {
		b := ip.addr.As4()
		return net.IP(b[:])
	}
	b := ip.addr.As16()
	return net.IP(b[:])
}

// IP returns the prefix's address as written, host bits included.
func (p IPPrefix) IP() IP { return p.ip }

// Bits returns the prefix length.
func (p IPPrefix) Bits() uint8 { return p.bits }

// IsValid reports whether p has a valid address and a length that fits it.
func (p IPPrefix) IsValid() bool { return !p.ip.IsZero() && p.bits <= p.ip.BitLen() }

// Valid is IsValid.
func (p IPPrefix) Valid() bool { return p.IsValid() }

// IsZero reports whether p is the zero value.
func (p IPPrefix) IsZero() bool { return p == IPPrefix{} }

// IsSingleIP reports whether p contains exactly one address.
func (p IPPrefix) IsSingleIP() bool { return p.bits != 0 && p.bits == p.ip.BitLen() }

// Masked returns p with host bits cleared, or the zero value if p is invalid.
func (p IPPrefix) Masked() IPPrefix {
	if masked, err := p.ip.Prefix(p.bits); err == nil {
		return masked
	}
	return IPPrefix{}
}

// Range returns the inclusive range of addresses in p.
func (p IPPrefix) Range() IPRange {
	if !p.IsValid() {
		return IPRange{}
	}
	return FromRange(netipx.RangeOfPrefix(p.netip()))
}

// IPNet returns p as a net.IPNet, keeping the address as written.
func (p IPPrefix) IPNet() *net.IPNet {
	if !p.IsValid() {
		return &net.IPNet{}
	}
	return &net.IPNet{IP: p.ip.std(), Mask: net.CIDRMask(int(p.bits), int(p.ip.BitLen()))}
}

// Contains reports whether p contains ip.
func (p IPPrefix) Contains(ip IP) bool { return p.IsValid() && p.netip().Contains(ip.addr) }

// Overlaps reports whether p and o have any address in common.
func (p IPPrefix) Overlaps(o IPPrefix) bool {
	return p.IsValid() && o.IsValid() && p.netip().Overlaps(o.netip())
}

// AppendTo appends the text form of p to b.
func (p IPPrefix) AppendTo(b []byte) []byte {
	text, _ := p.MarshalText()
	return append(b, text...)
}

// MarshalText implements encoding.TextMarshaler; the zero value encodes as "".
func (p IPPrefix) MarshalText() ([]byte, error) {
	if p.IsZero() {
		return []byte(""), nil
	}
	return []byte(p.String()), nil
}

// String returns p in CIDR notation, or "invalid IPPrefix" if p is invalid.
func (p IPPrefix) String() string {
	if !p.IsValid() {
		return "invalid IPPrefix"
	}
	return fmt.Sprintf("%s/%d", p.ip, p.bits)
}

func (p IPPrefix) netip() netip.Prefix { return netip.PrefixFrom(p.ip.addr, int(p.bits)) }

// From returns the first address in r.
func (r IPRange) From() IP { return r.from }

// To returns the last address in r.
func (r IPRange) To() IP { return r.to }

// String returns r as "from-to", or "invalid IPRange" if r is invalid.
func (r IPRange) String() string {
	if !r.IsValid() {
		return "invalid IPRange"
	}
	return r.from.String() + "-" + r.to.String()
}

// AppendTo appends the text form of r to b.
func (r IPRange) AppendTo(b []byte) []byte {
	text, _ := r.MarshalText()
	return append(b, text...)
}

// MarshalText implements encoding.TextMarshaler; the zero value encodes as "".
func (r IPRange) MarshalText() ([]byte, error) {
	if r.IsZero() {
		return []byte(""), nil
	}
	return []byte(r.String()), nil
}

// IsZero reports whether r is the zero value.
func (r IPRange) IsZero() bool { return r == IPRange{} }

// IsValid reports whether r has addresses of the same family with From not after To.
func (r IPRange) IsValid() bool { return r.netipx().IsValid() }

// Valid is IsValid.
func (r IPRange) Valid() bool { return r.IsValid() }

// Contains reports whether r contains addr.
func (r IPRange) Contains(addr IP) bool { return r.IsValid() && r.netipx().Contains(addr.addr) }

// Overlaps reports whether r and o have any address in common.
func (r IPRange) Overlaps(o IPRange) bool {
	return r.IsValid() && o.IsValid() && r.netipx().Overlaps(o.netipx())
}

// Prefixes returns the smallest set of prefixes that covers r.
func (r IPRange) Prefixes() []IPPrefix { return r.AppendPrefixes(nil) }

// AppendPrefixes appends the prefixes that cover r to dst.
func (r IPRange) AppendPrefixes(dst []IPPrefix) []IPPrefix {
	if !r.IsValid() {
		return dst
	}
	for _, prefix := range r.netipx().Prefixes() {
		dst = append(dst, FromPrefix(prefix))
	}
	return dst
}

// Prefix returns r as a single prefix when it is exactly one.
func (r IPRange) Prefix() (IPPrefix, bool) {
	if !r.IsValid() {
		return IPPrefix{}, false
	}
	prefix, ok := r.netipx().Prefix()
	if !ok {
		return IPPrefix{}, false
	}
	return FromPrefix(prefix), true
}

func (r IPRange) netipx() netipx.IPRange { return netipx.IPRangeFrom(r.from.addr, r.to.addr) }
