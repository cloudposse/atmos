package policy

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	"go4.org/netipx"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/template/compatibility/netaddr"
)

// policyNetFuncs preserves pure IP helpers while guarding every DNS lookup method.
// Native network functions describe the Gomplate namespace capabilities guarded below.
type nativeNetFuncs interface {
	LookupIP(name any) (string, error)
	LookupIPs(name any) ([]string, error)
	LookupCNAME(name any) (string, error)
	LookupSRV(name any) (*net.SRV, error)
	LookupSRVs(name any) ([]*net.SRV, error)
	LookupTXT(name any) ([]string, error)
	ParseAddr(ip any) (netip.Addr, error)
	ParsePrefix(ipprefix any) (netip.Prefix, error)
	ParseRange(iprange any) (netipx.IPRange, error)
	// Gomplate v3 names, kept working by the templating compatibility shims.
	ParseIP(ip any) (netaddr.IP, error)
	ParseIPPrefix(ipprefix any) (netaddr.IPPrefix, error)
	ParseIPRange(iprange any) (netaddr.IPRange, error)
	CIDRHost(hostnum any, prefix any) (netip.Addr, error)
	CIDRNetmask(prefix any) (netip.Addr, error)
	CIDRSubnets(newbits any, prefix any) ([]netip.Prefix, error)
	CIDRSubnetSizes(args ...any) ([]netip.Prefix, error)
}

type policyNetFuncs struct {
	native nativeNetFuncs
	file   string
}

func (f *policyNetFuncs) check(method string, name any) error {
	if err := guard.Function(context.Background(), "net."+method, Engine, guard.FileOrigin(f.file)); err != nil {
		return err
	}
	return guard.Source(context.Background(), "template.dns", "dns://"+fmt.Sprint(name), f.file)
}

func (f *policyNetFuncs) LookupIP(name any) (string, error) {
	defer perf.Track(nil, "policy.LookupIP")()

	if err := f.check("LookupIP", name); err != nil {
		return "", err
	}
	return f.native.LookupIP(name)
}

func (f *policyNetFuncs) LookupIPs(name any) ([]string, error) {
	defer perf.Track(nil, "policy.LookupIPs")()

	if err := f.check("LookupIPs", name); err != nil {
		return nil, err
	}
	return f.native.LookupIPs(name)
}

func (f *policyNetFuncs) LookupCNAME(name any) (string, error) {
	defer perf.Track(nil, "policy.LookupCNAME")()

	if err := f.check("LookupCNAME", name); err != nil {
		return "", err
	}
	return f.native.LookupCNAME(name)
}

func (f *policyNetFuncs) LookupSRV(name any) (*net.SRV, error) {
	defer perf.Track(nil, "policy.LookupSRV")()

	if err := f.check("LookupSRV", name); err != nil {
		return nil, err
	}
	return f.native.LookupSRV(name)
}

func (f *policyNetFuncs) LookupSRVs(name any) ([]*net.SRV, error) {
	defer perf.Track(nil, "policy.LookupSRVs")()

	if err := f.check("LookupSRVs", name); err != nil {
		return nil, err
	}
	return f.native.LookupSRVs(name)
}

func (f *policyNetFuncs) LookupTXT(name any) ([]string, error) {
	defer perf.Track(nil, "policy.LookupTXT")()

	if err := f.check("LookupTXT", name); err != nil {
		return nil, err
	}
	return f.native.LookupTXT(name)
}

// nativeEnvFuncs describes the Gomplate namespace capabilities guarded below.
type nativeEnvFuncs interface {
	Getenv(key any, def ...string) string
	ExpandEnv(s any) string
}

type policyEnvFuncs struct {
	native nativeEnvFuncs
	file   string
}

func (f *policyEnvFuncs) Getenv(key any, defaults ...string) (string, error) {
	defer perf.Track(nil, "policy.Getenv")()

	if err := guard.Function(context.Background(), "env.Getenv", Engine, guard.FileOrigin(f.file)); err != nil {
		return "", err
	}
	return f.native.Getenv(key, defaults...), nil
}

func (f *policyEnvFuncs) ExpandEnv(value any) (string, error) {
	defer perf.Track(nil, "policy.ExpandEnv")()

	if err := guard.Function(context.Background(), "env.ExpandEnv", Engine, guard.FileOrigin(f.file)); err != nil {
		return "", err
	}
	return f.native.ExpandEnv(value), nil
}

func (f *policyNetFuncs) ParseAddr(ip any) (netip.Addr, error) {
	defer perf.Track(nil, "policy.ParseAddr")()
	if err := guard.Function(context.Background(), "net.ParseAddr", Engine, guard.FileOrigin(f.file)); err != nil {
		return netip.Addr{}, err
	}
	return f.native.ParseAddr(ip)
}

func (f *policyNetFuncs) ParsePrefix(prefix any) (netip.Prefix, error) {
	defer perf.Track(nil, "policy.ParsePrefix")()
	if err := guard.Function(context.Background(), "net.ParsePrefix", Engine, guard.FileOrigin(f.file)); err != nil {
		return netip.Prefix{}, err
	}
	return f.native.ParsePrefix(prefix)
}

func (f *policyNetFuncs) ParseRange(value any) (netipx.IPRange, error) {
	defer perf.Track(nil, "policy.ParseRange")()
	if err := guard.Function(context.Background(), "net.ParseRange", Engine, guard.FileOrigin(f.file)); err != nil {
		return netipx.IPRange{}, err
	}
	return f.native.ParseRange(value)
}

func (f *policyNetFuncs) ParseIP(ip any) (netaddr.IP, error) {
	defer perf.Track(nil, "policy.ParseIP")()
	if err := guard.Function(context.Background(), "net.ParseIP", Engine, guard.FileOrigin(f.file)); err != nil {
		return netaddr.IP{}, err
	}
	return f.native.ParseIP(ip)
}

func (f *policyNetFuncs) ParseIPPrefix(prefix any) (netaddr.IPPrefix, error) {
	defer perf.Track(nil, "policy.ParseIPPrefix")()
	if err := guard.Function(context.Background(), "net.ParseIPPrefix", Engine, guard.FileOrigin(f.file)); err != nil {
		return netaddr.IPPrefix{}, err
	}
	return f.native.ParseIPPrefix(prefix)
}

func (f *policyNetFuncs) ParseIPRange(value any) (netaddr.IPRange, error) {
	defer perf.Track(nil, "policy.ParseIPRange")()
	if err := guard.Function(context.Background(), "net.ParseIPRange", Engine, guard.FileOrigin(f.file)); err != nil {
		return netaddr.IPRange{}, err
	}
	return f.native.ParseIPRange(value)
}

func (f *policyNetFuncs) CIDRHost(host, prefix any) (netip.Addr, error) {
	defer perf.Track(nil, "policy.CIDRHost")()
	if err := guard.Function(context.Background(), "net.CIDRHost", Engine, guard.FileOrigin(f.file)); err != nil {
		return netip.Addr{}, err
	}
	return f.native.CIDRHost(host, prefix)
}

func (f *policyNetFuncs) CIDRNetmask(prefix any) (netip.Addr, error) {
	defer perf.Track(nil, "policy.CIDRNetmask")()
	if err := guard.Function(context.Background(), "net.CIDRNetmask", Engine, guard.FileOrigin(f.file)); err != nil {
		return netip.Addr{}, err
	}
	return f.native.CIDRNetmask(prefix)
}

func (f *policyNetFuncs) CIDRSubnets(bits, prefix any) ([]netip.Prefix, error) {
	defer perf.Track(nil, "policy.CIDRSubnets")()
	if err := guard.Function(context.Background(), "net.CIDRSubnets", Engine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.CIDRSubnets(bits, prefix)
}

func (f *policyNetFuncs) CIDRSubnetSizes(args ...any) ([]netip.Prefix, error) {
	defer perf.Track(nil, "policy.CIDRSubnetSizes")()
	if err := guard.Function(context.Background(), "net.CIDRSubnetSizes", Engine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.CIDRSubnetSizes(args...)
}
