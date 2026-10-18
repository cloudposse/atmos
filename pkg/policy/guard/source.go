package guard

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const schemeSeparator = "://"

// Source checks a destination before credentials, cache reuse, or I/O.
func Source(ctx context.Context, operation, raw, sourceFile string) error {
	defer perf.Track(nil, "guard.Source")()

	enabled, err := Enabled("source")
	if err != nil || !enabled {
		return err
	}
	input, err := DescribeSource(operation, raw, sourceFile)
	if err != nil {
		return err
	}
	return evaluate(ctx, "source", input)
}

// DescribeSource normalizes transport syntax without fetching or exposing credentials.
func DescribeSource(operation, raw, sourceFile string) (SourceInput, error) {
	defer perf.Track(nil, "guard.DescribeSource")()

	input := SourceInput{Operation: operation, Origin: FileOrigin(sourceFile)}
	transport := normalizeTransport(raw)
	parsed, err := url.Parse(transport)
	if err != nil {
		return input, fmt.Errorf("%w: invalid source URI (details withheld)", errUtils.ErrPolicyInputInvalid)
	}
	if parsed.Scheme != "" && (len(parsed.Scheme) != 1 || !filepath.IsAbs(transport)) {
		input.Scheme = strings.ToLower(parsed.Scheme)
		input.Host = strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
		input.Remote = input.Scheme != "file"
		input.Path = parsed.Path
		if input.Remote {
			return input, nil
		}
		transport, err = fileURLTransport(parsed)
		if err != nil {
			return input, err
		}
	}
	input.Scheme = "file"
	absolute, err := filepath.Abs(transport)
	if err != nil {
		return input, fmt.Errorf("%w: source path", errUtils.ErrPolicyInputInvalid)
	}
	input.Path = filepath.Clean(absolute)
	resolved, err := filepath.EvalSymlinks(input.Path)
	if err == nil {
		input.Path = resolved
		input.PathResolved = true
	}
	return input, nil
}

// fileURLTransport returns the local path a file URL names. A file URL with an opaque part or no
// path, such as file:../x or file://localhost, names no absolute path, and resolving it would
// describe the working directory instead of the destination the reader would use.
func fileURLTransport(parsed *url.URL) (string, error) {
	if parsed.Host != "" && parsed.Host != "localhost" {
		return "", fmt.Errorf("%w: nonlocal file authority", errUtils.ErrPolicyInputInvalid)
	}
	if parsed.Opaque != "" || parsed.Path == "" {
		return "", fmt.Errorf("%w: file URL has no absolute path", errUtils.ErrPolicyInputInvalid)
	}
	if runtime.GOOS == windowsOS {
		return trimWindowsDriveSlash(parsed.Path), nil
	}
	return parsed.Path, nil
}

// windowsOS is the runtime.GOOS value for Windows.
const windowsOS = "windows"

// trimWindowsDriveSlash turns the path of a Windows file URL, such as /C:/dir, into C:/dir. The
// URL form carries a leading slash before the drive letter that filepath cannot resolve, which
// would place an allowed local path outside every allowed root.
func trimWindowsDriveSlash(path string) string {
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' && isASCIILetter(path[1]) && (len(path) == 3 || path[3] == '/') {
		return path[1:]
	}
	return path
}

func isASCIILetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isSCP(raw string) bool {
	at := strings.IndexByte(raw, '@')
	colon := strings.IndexByte(raw, ':')
	return at > 0 && colon > at && !strings.Contains(raw, schemeSeparator)
}

func isRemote(raw string) bool {
	if isSCP(raw) {
		return true
	}
	if _, rest, ok := strings.Cut(raw, "::"); ok {
		raw = rest
	}
	if strings.Contains(raw, schemeSeparator) {
		return !strings.HasPrefix(strings.ToLower(raw), "file://")
	}
	return strings.HasPrefix(raw, "github.com/") || strings.HasPrefix(raw, "gitlab.com/") || strings.HasPrefix(raw, "bitbucket.org/")
}

func safeSource(raw string) string {
	input, err := DescribeSource("", raw, "")
	if err != nil {
		return "[unavailable]"
	}
	if !input.Remote {
		return input.Path
	}
	return input.Scheme + schemeSeparator + input.Host + input.Path
}

func normalizeTransport(raw string) string {
	transport := raw
	if _, rest, found := strings.Cut(raw, "::"); found {
		transport = rest
	}
	if isSCP(transport) {
		_, hostpath, _ := strings.Cut(transport, "@")
		host, path, _ := strings.Cut(hostpath, ":")
		transport = "ssh://" + host + "/" + path
	}
	if !strings.Contains(transport, schemeSeparator) && !filepath.IsAbs(transport) {
		host, _, _ := strings.Cut(transport, "/")
		if host == "github.com" || host == "gitlab.com" || host == "bitbucket.org" {
			transport = "https://" + transport
		}
	}
	return transport
}

// IsPolicyError identifies failures which optional imports must never suppress.
func IsPolicyError(err error) bool {
	defer perf.Track(nil, "guard.IsPolicyError")()
	for _, sentinel := range []error{errUtils.ErrPolicyDenied, errUtils.ErrPolicyInvalid, errUtils.ErrPolicyEvaluation, errUtils.ErrPolicyInputUnavailable, errUtils.ErrPolicyInputInvalid, errUtils.ErrPolicyRequirements, errUtils.ErrPolicyEngineUnknown} {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}
