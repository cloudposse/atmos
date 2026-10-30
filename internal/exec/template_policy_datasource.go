package exec

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	data "github.com/cloudposse/atmos/pkg/template/gomplate"
)

func policyDatasource(d *data.Data, file, alias string, raw bool, args ...string) (any, error) {
	enabled, err := guard.Enabled("source")
	if err != nil {
		return nil, err
	}
	if !enabled {
		if raw {
			return d.Include(alias, args...)
		}
		return d.Datasource(alias, args...)
	}
	target, headers, err := policyDatasourceTarget(d, alias, args)
	if err != nil {
		return nil, err
	}
	if err := guard.Source(context.Background(), guard.OperationTemplateDatasource, target.String(), file); err != nil {
		return nil, err
	}
	switch target.Scheme {
	case "file", "env":
		content, err := d.Include(alias, args...)
		if err != nil {
			return nil, err
		}
		return parsePolicyDatasource([]byte(content), target.Path, "", raw)
	case "http", "https":
		return readPolicyHTTPDatasource(d, file, target, headers, raw)
	default:
		return nil, fmt.Errorf("%w: datasource scheme has no guarded collector", errUtils.ErrPolicyInputUnavailable)
	}
}

func readPolicyHTTPDatasource(d *data.Data, file string, target *url.URL, headers http.Header, raw bool) (any, error) {
	ctx := d.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	client := &http.Client{Timeout: policyDatasourceTimeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= policyDatasourceRedirects {
			return fmt.Errorf("%w: too many datasource redirects", errUtils.ErrPolicyInputUnavailable)
		}
		return guard.Source(req.Context(), guard.OperationTemplateDatasource, req.URL.String(), file)
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, errUtils.ErrPolicyInputInvalid
	}
	req.Header = headers.Clone()
	response, err := client.Do(req)
	if err != nil {
		// A redirect target can be denied or need approval; that decision must not be reported as an unreachable source.
		if isPolicyOutcome(err) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: guarded datasource fetch failed", errUtils.ErrPolicyInputUnavailable)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: datasource HTTP status %d", errUtils.ErrPolicyInputUnavailable, response.StatusCode)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, policyDatasourceMaxBytes+1))
	if err != nil || len(content) > policyDatasourceMaxBytes {
		return nil, fmt.Errorf("%w: datasource incomplete or too large", errUtils.ErrPolicyInputUnavailable)
	}
	return parsePolicyDatasource(content, target.Path, response.Header.Get("Content-Type"), raw)
}

const (
	policyDatasourceTimeout   = 5 * time.Second
	policyDatasourceRedirects = 10
	policyDatasourceMaxBytes  = 16 * 1024 * 1024
)

func parsePolicyDatasource(content []byte, path, mediaType string, raw bool) (any, error) {
	if raw {
		return string(content), nil
	}
	format, _, _ := mime.ParseMediaType(mediaType)
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case format == "application/json", ext == ".json", ext == ".ejson":
		return parsePolicyJSONDatasource(content)
	case strings.Contains(format, "yaml"), ext == ".yaml", ext == ".yml":
		var result any
		if err := yaml.Unmarshal(content, &result); err != nil {
			return nil, err
		}
		return result, nil
	case format == "text/csv", ext == ".csv":
		return data.CSV(string(content))
	case format == "application/toml", ext == ".toml":
		return data.TOML(string(content))
	default:
		return string(content), nil
	}
}

func policyDatasourceTarget(d *data.Data, alias string, args []string) (*url.URL, http.Header, error) {
	target, err := url.Parse(alias)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: invalid datasource", errUtils.ErrPolicyInputInvalid)
	}
	var headers http.Header
	if source, exists := d.Sources[alias]; exists {
		if source.URL == nil {
			return nil, nil, errUtils.ErrPolicyInputUnavailable
		}
		copy := *source.URL
		target = &copy
		headers = source.Header
	}
	if len(args) > 1 {
		return nil, nil, fmt.Errorf("%w: guarded datasources accept at most one subpath", errUtils.ErrPolicyInputUnavailable)
	}
	if len(args) == 1 {
		relative, err := url.Parse(args[0])
		if err != nil {
			return nil, nil, errUtils.ErrPolicyInputInvalid
		}
		if target.Scheme == "file" {
			// The file URL path is a URL path, so join it with forward slashes on every OS.
			target = target.JoinPath(relative.Path)
		} else {
			target = target.ResolveReference(relative)
		}
	}
	return target, headers, nil
}

func parsePolicyJSONDatasource(content []byte) (any, error) {
	// Decode with YAML (a JSON superset) so whole numbers stay ints, matching native gomplate data.JSON.
	var result any
	if err := yaml.Unmarshal(content, &result); err != nil {
		return nil, err
	}
	if object, ok := result.(map[string]any); ok {
		if _, encrypted := object["_public_key"]; encrypted {
			return nil, fmt.Errorf("%w: EJSON key reads have no guarded collector", errUtils.ErrPolicyInputUnavailable)
		}
	}
	return result, nil
}
