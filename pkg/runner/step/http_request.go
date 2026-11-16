package step

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// buildHTTPRequest resolves templates and assembles the request spec.
func buildHTTPRequest(step *schema.WorkflowStep, vars *Variables) (*httpRequest, error) {
	rawURL, err := resolveField(vars, step.Name, "url", step.URL)
	if err != nil {
		return nil, err
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrHTTPStepRequestFailed).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("url", rawURL).
			WithHint("Provide a valid absolute URL (e.g. https://example.com/hook)").
			Err()
	}

	// url.Parse accepts relative URLs; reject them here so they fail fast with clear
	// config feedback instead of surfacing later as retryable transport errors.
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, errUtils.Build(errUtils.ErrHTTPStepRequestFailed).
			WithContext("step", step.Name).
			WithContext("url", rawURL).
			WithHint("Provide a valid absolute URL (e.g. https://example.com/hook)").
			Err()
	}

	if err := applyQueryParams(parsedURL, step, vars); err != nil {
		return nil, err
	}

	headers, err := vars.ResolveEnvMap(step.Headers)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("field", "headers").
			Err()
	}
	if headers == nil {
		headers = make(map[string]string)
	}

	body, err := buildHTTPBody(step, vars, headers)
	if err != nil {
		return nil, err
	}

	method := http.MethodGet
	if step.Method != "" {
		method = strings.ToUpper(strings.TrimSpace(step.Method))
	}

	return &httpRequest{
		method:  method,
		url:     parsedURL.String(),
		headers: headers,
		body:    body,
	}, nil
}

// applyQueryParams resolves and appends query-string parameters to the URL.
func applyQueryParams(parsedURL *url.URL, step *schema.WorkflowStep, vars *Variables) error {
	if len(step.Query) == 0 {
		return nil
	}
	query := parsedURL.Query()
	for key, value := range step.Query {
		resolved, err := vars.Resolve(value)
		if err != nil {
			return errUtils.Build(errUtils.ErrTemplateEvaluation).
				WithCause(err).
				WithContext("step", step.Name).
				WithContext("field", "query").
				WithContext("param", key).
				Err()
		}
		query.Set(key, resolved)
	}
	parsedURL.RawQuery = query.Encode()
	return nil
}

// buildHTTPBody resolves the raw body or form params, setting a default Content-Type
// header for form payloads when none was provided.
func buildHTTPBody(step *schema.WorkflowStep, vars *Variables, headers map[string]string) ([]byte, error) {
	if step.Body != "" {
		resolved, err := vars.Resolve(step.Body)
		if err != nil {
			return nil, errUtils.Build(errUtils.ErrTemplateEvaluation).
				WithCause(err).
				WithContext("step", step.Name).
				WithContext("field", "body").
				Err()
		}
		return []byte(resolved), nil
	}

	if len(step.Form) == 0 {
		return nil, nil
	}

	resolvedForm, err := vars.ResolveEnvMap(step.Form)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("field", "form").
			Err()
	}

	existingCT := findHeader(headers, contentTypeHeader)
	if strings.Contains(strings.ToLower(existingCT), contentTypeJSON) {
		encoded, err := json.Marshal(resolvedForm)
		if err != nil {
			return nil, errUtils.Build(errUtils.ErrHTTPStepRequestFailed).
				WithCause(err).
				WithContext("step", step.Name).
				WithContext("field", "form").
				Err()
		}
		return encoded, nil
	}

	values := url.Values{}
	for key, value := range resolvedForm {
		values.Set(key, value)
	}
	if existingCT == "" {
		headers[contentTypeHeader] = contentTypeForm
	}
	return []byte(values.Encode()), nil
}
