package source

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/vendor"
)

// WithDepth applies a default Git history depth without overriding source parameters.
// Zero requests full history; non-Git sources are unchanged.
func WithDepth(src string, depth int) (string, error) {
	defer perf.Track(nil, "source.WithDepth")()

	if depth < 0 {
		return "", fmt.Errorf("%w: Git history depth must be zero or greater", errUtils.ErrInvalidFlagValue)
	}
	if !vendor.IsGitURI(src) {
		return src, nil
	}
	base, rawQuery, _ := strings.Cut(src, "?")
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", err
	}
	if query.Has("depth") {
		return src, nil
	}
	query.Set("depth", strconv.Itoa(depth))
	return base + "?" + query.Encode(), nil
}
