package page

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/pkg/errors"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/provider"
)

type Form struct {
	URL    string
	Method string
	Values *url.Values
}

func (form *Form) BuildRequest() (*http.Request, error) {
	// Atmos fork: a GET form submits its fields in the URL query; upstream always sent them in the body.
	if strings.EqualFold(form.Method, http.MethodGet) {
		target, err := url.Parse(form.URL)
		if err != nil {
			return nil, errors.Wrap(err, "error building request")
		}
		query := target.Query()
		for name, vals := range *form.Values {
			for _, val := range vals {
				query.Add(name, val)
			}
		}
		target.RawQuery = query.Encode()
		req, err := http.NewRequest(http.MethodGet, target.String(), nil)
		if err != nil {
			return nil, errors.Wrap(err, "error building request")
		}
		return req, nil
	}

	values := strings.NewReader(form.Values.Encode())
	req, err := http.NewRequest(form.Method, form.URL, values)
	if err != nil {
		return nil, errors.Wrap(err, "error building request")
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	return req, nil
}

func (form *Form) Submit(client *provider.HTTPClient) (*http.Response, error) {
	if req, err := form.BuildRequest(); err != nil {
		return nil, errors.Wrap(err, "error building request")
	} else if res, err := client.Do(req); err != nil {
		return nil, errors.Wrap(err, "error submitting form")
	} else {
		return res, nil
	}
}

// If the document has multiple forms, the first form with an `action` attribute will be parsed.
// You can specify the exact form using a CSS filter.
func NewFormFromDocument(doc *goquery.Document, formFilter string) (*Form, error) {
	form := Form{Method: "POST"}

	if formFilter == "" {
		formFilter = "form[action]"
	}
	formSelection := doc.Find(formFilter).First()
	if formSelection.Size() != 1 {
		return nil, fmt.Errorf("could not find form")
	}

	attrValue, ok := formSelection.Attr("action")
	if ok {
		action, err := resolveFormAction(attrValue, doc.Url)
		if err != nil {
			return nil, err
		}
		form.URL = action
	} else if doc.Url != nil {
		form.URL = doc.Url.String()
	} else {
		// Atmos fork: goquery leaves doc.Url nil for reader-based documents; upstream dereferenced it.
		return nil, fmt.Errorf("form has no action and the document has no URL to fall back to")
	}

	attrValue, ok = formSelection.Attr("method")
	if ok {
		form.Method = strings.ToUpper(attrValue)
	}

	form.Values = &url.Values{}
	formSelection.Find("input").Each(func(_ int, s *goquery.Selection) {
		name, ok := s.Attr("name")
		if !ok {
			return
		}

		val, ok := s.Attr("value")
		if !ok {
			return
		}

		form.Values.Add(name, val)
	})

	return &form, nil
}

// resolveFormAction resolves a form action against the document URL.
// Atmos fork: upstream stored a relative action verbatim, which cannot be requested because it has no scheme or host.
// Absolute actions are returned unchanged; a relative action needs a document URL to resolve against.
func resolveFormAction(action string, base *url.URL) (string, error) {
	parsed, err := url.Parse(action)
	if err != nil {
		return "", errors.Wrap(err, "invalid form action")
	}
	if parsed.IsAbs() {
		return action, nil
	}
	if base == nil {
		return "", fmt.Errorf("form action %q is relative and the document has no URL to resolve it against", action)
	}
	return base.ResolveReference(parsed).String(), nil
}

func NewFormFromResponse(res *http.Response, formFilter string) (*Form, error) {
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build document from response")
	}
	// Atmos fork: reader-based documents have no URL, so supply the request URL as the fallback form action.
	if doc.Url == nil && res.Request != nil {
		doc.Url = res.Request.URL
	}
	return NewFormFromDocument(doc, formFilter)
}
