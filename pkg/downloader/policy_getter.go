package downloader

import (
	"context"
	"fmt"
	"net/url"

	"github.com/hashicorp/go-getter"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

// policyGetter also guards go-getter redirects and detected transports.
type policyGetter struct {
	getter.Getter
	ctx    context.Context
	scheme string
}

func (g *policyGetter) check(u *url.URL) error {
	if g.scheme == "hg" {
		return fmt.Errorf("%w: mercurial has no guarded transport", errUtils.ErrPolicyInputUnavailable)
	}
	return guard.Source(g.ctx, "download", u.String(), "")
}

func (g *policyGetter) Get(dest string, u *url.URL) error {
	defer perf.Track(nil, "downloader.Get")()

	if err := g.check(u); err != nil {
		return err
	}
	return g.Getter.Get(dest, u)
}

func (g *policyGetter) GetFile(dest string, u *url.URL) error {
	defer perf.Track(nil, "downloader.GetFile")()

	if err := g.check(u); err != nil {
		return err
	}
	return g.Getter.GetFile(dest, u)
}

func (g *policyGetter) ClientMode(u *url.URL) (getter.ClientMode, error) {
	defer perf.Track(nil, "downloader.ClientMode")()

	if err := g.check(u); err != nil {
		return getter.ClientModeAny, err
	}
	return g.Getter.ClientMode(u)
}
