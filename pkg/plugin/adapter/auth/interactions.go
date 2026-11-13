package auth

import (
	"context"
	"net/url"
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	deviceCodeInteractionKind = "device_code"
	maxVerificationURILength  = 2048
)

var publicDeviceCode = regexp.MustCompile(`^[A-Za-z0-9-]{4,32}$`)

// interactionHandler permits a typed device prompt only on the host's UI channel.
// Arbitrary titles, progress text, and complete/token-bearing URLs are never rendered.
func interactionHandler(ctx context.Context, interactive bool, present func(string, string)) func(*pluginv1.Interaction) error {
	return func(event *pluginv1.Interaction) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !interactive || present == nil {
			return errUtils.ErrPluginInteractionRequired
		}
		uri, code, err := parseDeviceCode(event)
		if err != nil {
			return err
		}
		present(uri, code)
		return nil
	}
}

// parseDeviceCode accepts only the device prompt fields that are safe to render
// and rejects unsupported interaction kinds or malformed protocol data.
func parseDeviceCode(event *pluginv1.Interaction) (string, string, error) {
	if event.GetKind() != deviceCodeInteractionKind {
		return "", "", errUtils.ErrPluginInteractionRequired
	}
	fields := event.GetFields()
	uri, code := fields["verification_uri"], fields["user_code"]
	if len(fields) != 2 || !publicDeviceCode.MatchString(code) || !publicVerificationURI(uri) {
		return "", "", errUtils.ErrPluginProtocol
	}
	return uri, code, nil
}

// publicVerificationURI permits printable HTTPS verification links without
// credentials, queries or fragments. Complete authorization URLs use the browser API.
func publicVerificationURI(raw string) bool {
	if len(raw) > maxVerificationURILength || strings.IndexFunc(raw, func(r rune) bool { return r < '!' || r > '~' }) >= 0 {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil &&
		parsed.RawQuery == "" && !parsed.ForceQuery && parsed.Fragment == ""
}

func presentDeviceCode(uri, code string) {
	defer perf.Track(nil, "auth.presentDeviceCode")()

	ui.Infof("Open %s and enter code %s.", uri, code)
}
