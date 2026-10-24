package config

import (
	"context"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/script/starlark/internal/convert"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/threadcontext"
)

const (
	configSchemaTimeout      = 30 * time.Second
	configSchemaMaxBytes     = 4 << 20
	configSchemaMaxResources = 64
	configSchemaMaxRedirects = 10
)

func newSchemaHTTPClient() *http.Client {
	return &http.Client{Timeout: configSchemaTimeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= configSchemaMaxRedirects {
			return convert.InvalidArgument("schema redirect limit exceeded")
		}
		return configHTTPSURL(req.URL)
	}}
}

func configHTTPSURL(resource *url.URL) error {
	if resource.Scheme != "https" || resource.Host == "" || resource.User != nil {
		return convert.InvalidArgument("remote schema URLs must use HTTPS without embedded credentials")
	}
	return nil
}

func (s *binding) readRemoteSchema(thread *starlark.Thread, resource *url.URL) (*configDocument, error) {
	if err := configHTTPSURL(resource); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(threadcontext.Get(thread), configSchemaTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, resource.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/schema+json, application/json, application/yaml, text/yaml")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, convert.InvalidArgument("schema request returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, configSchemaMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > configSchemaMaxBytes {
		return nil, convert.InvalidArgument("schema exceeds %d byte limit", configSchemaMaxBytes)
	}
	doc, err := parseConfig(ctx, data, remoteSchemaFormat(resource, response.Header.Get("Content-Type")))
	if err != nil {
		return doc, err
	}
	if err := checkRemoteSchemaReferences(doc.value); err != nil {
		return doc, err
	}
	return doc, nil
}

func remoteSchemaFormat(resource *url.URL, contentType string) string {
	switch strings.ToLower(path.Ext(resource.Path)) {
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "application/yaml" || mediaType == "application/x-yaml" || mediaType == "text/yaml" {
		return "yaml"
	}
	return "json"
}

// Check before giving a remote document to the compiler: an absolute local $ref,
// or an id that rebases relative references onto file:, must never cause disk reads.
// Walk all objects because JSON Schema pointers may target arbitrary locations.
func checkRemoteSchemaReferences(value starlark.Value) error {
	switch value := value.(type) {
	case *starlark.Dict:
		for _, pair := range value.Items() {
			if err := checkRemoteSchemaReference(pair); err != nil {
				return err
			}
			if err := checkRemoteSchemaReferences(pair[1]); err != nil {
				return err
			}
		}
	case *starlark.List:
		for i := 0; i < value.Len(); i++ {
			if err := checkRemoteSchemaReferences(value.Index(i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkRemoteSchemaReference(pair starlark.Tuple) error {
	name := string(pair[0].(starlark.String))
	switch name {
	case "$ref", "$dynamicRef", "$recursiveRef", "$id", "id", "$schema":
		if reference, ok := starlark.AsString(pair[1]); ok {
			parsed, err := url.Parse(reference)
			if err != nil {
				return err
			}
			if parsed.Scheme == "file" {
				return convert.InvalidArgument("remote schemas cannot reference local files")
			}
		}
	}
	return nil
}
