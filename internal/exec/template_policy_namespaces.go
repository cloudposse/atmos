package exec

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	"github.com/hairyhenderson/gomplate/v3/funcs" //nolint:staticcheck // Gomplate v3 exposes namespace types through this embedding API.
	"inet.af/netaddr"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

// policyNetFuncs preserves pure IP helpers while guarding every DNS lookup method.
type policyNetFuncs struct {
	native *funcs.NetFuncs
	file   string
}

func (f *policyNetFuncs) check(method string, name any) error {
	if err := guard.Function(context.Background(), "net."+method, policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return err
	}
	return guard.Source(context.Background(), "template.dns", "dns://"+fmt.Sprint(name), f.file)
}

func (f *policyNetFuncs) LookupIP(name any) (string, error) {
	defer perf.Track(nil, "exec.LookupIP")()

	if err := f.check("LookupIP", name); err != nil {
		return "", err
	}
	return f.native.LookupIP(name)
}

func (f *policyNetFuncs) LookupIPs(name any) ([]string, error) {
	defer perf.Track(nil, "exec.LookupIPs")()

	if err := f.check("LookupIPs", name); err != nil {
		return nil, err
	}
	return f.native.LookupIPs(name)
}

func (f *policyNetFuncs) LookupCNAME(name any) (string, error) {
	defer perf.Track(nil, "exec.LookupCNAME")()

	if err := f.check("LookupCNAME", name); err != nil {
		return "", err
	}
	return f.native.LookupCNAME(name)
}

func (f *policyNetFuncs) LookupSRV(name any) (*net.SRV, error) {
	defer perf.Track(nil, "exec.LookupSRV")()

	if err := f.check("LookupSRV", name); err != nil {
		return nil, err
	}
	return f.native.LookupSRV(name)
}

func (f *policyNetFuncs) LookupSRVs(name any) ([]*net.SRV, error) {
	defer perf.Track(nil, "exec.LookupSRVs")()

	if err := f.check("LookupSRVs", name); err != nil {
		return nil, err
	}
	return f.native.LookupSRVs(name)
}

func (f *policyNetFuncs) LookupTXT(name any) ([]string, error) {
	defer perf.Track(nil, "exec.LookupTXT")()

	if err := f.check("LookupTXT", name); err != nil {
		return nil, err
	}
	return f.native.LookupTXT(name)
}

type policyEnvFuncs struct {
	native *funcs.EnvFuncs
	file   string
}

func (f *policyEnvFuncs) Getenv(key any, defaults ...string) (string, error) {
	defer perf.Track(nil, "exec.Getenv")()

	if err := guard.Function(context.Background(), "env.Getenv", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return "", err
	}
	return f.native.Getenv(key, defaults...), nil
}

func (f *policyEnvFuncs) ExpandEnv(value any) (string, error) {
	defer perf.Track(nil, "exec.ExpandEnv")()

	if err := guard.Function(context.Background(), "env.ExpandEnv", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return "", err
	}
	return f.native.ExpandEnv(value), nil
}

func (f *policyNetFuncs) ParseIP(ip any) (netaddr.IP, error) {
	defer perf.Track(nil, "exec.ParseIP")()
	if err := guard.Function(context.Background(), "net.ParseIP", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return netaddr.IP{}, err
	}
	return f.native.ParseIP(ip)
}

func (f *policyNetFuncs) ParseIPPrefix(prefix any) (netaddr.IPPrefix, error) {
	defer perf.Track(nil, "exec.ParseIPPrefix")()
	if err := guard.Function(context.Background(), "net.ParseIPPrefix", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return netaddr.IPPrefix{}, err
	}
	return f.native.ParseIPPrefix(prefix)
}

func (f *policyNetFuncs) ParseIPRange(value any) (netaddr.IPRange, error) {
	defer perf.Track(nil, "exec.ParseIPRange")()
	if err := guard.Function(context.Background(), "net.ParseIPRange", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return netaddr.IPRange{}, err
	}
	return f.native.ParseIPRange(value)
}

func (f *policyNetFuncs) CIDRHost(host, prefix any) (netip.Addr, error) {
	defer perf.Track(nil, "exec.CIDRHost")()
	if err := guard.Function(context.Background(), "net.CIDRHost", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return netip.Addr{}, err
	}
	return f.native.CIDRHost(host, prefix)
}

func (f *policyNetFuncs) CIDRNetmask(prefix any) (netip.Addr, error) {
	defer perf.Track(nil, "exec.CIDRNetmask")()
	if err := guard.Function(context.Background(), "net.CIDRNetmask", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return netip.Addr{}, err
	}
	return f.native.CIDRNetmask(prefix)
}

func (f *policyNetFuncs) CIDRSubnets(bits, prefix any) ([]netip.Prefix, error) {
	defer perf.Track(nil, "exec.CIDRSubnets")()
	if err := guard.Function(context.Background(), "net.CIDRSubnets", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.CIDRSubnets(bits, prefix)
}

func (f *policyNetFuncs) CIDRSubnetSizes(args ...any) ([]netip.Prefix, error) {
	defer perf.Track(nil, "exec.CIDRSubnetSizes")()
	if err := guard.Function(context.Background(), "net.CIDRSubnetSizes", policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return nil, err
	}
	return f.native.CIDRSubnetSizes(args...)
}
