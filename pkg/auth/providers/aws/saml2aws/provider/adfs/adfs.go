package adfs

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/pkg/errors"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/cfg"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/prompter"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/provider"
)

// Client wrapper around ADFS enabling authentication and retrieval of assertions
type Client struct {
	provider.ValidateBase

	client     *provider.HTTPClient
	idpAccount *cfg.IDPAccount
}

type AuthResponseType int

const (
	UNKNOWN AuthResponseType = iota
	SAML_RESPONSE
	MFA_PROMPT
	AZURE_MFA_WAIT
	AZURE_MFA_SERVER_WAIT
)

// Atmos fork: the Azure MFA poll interval is a package variable (upstream hard-coded
// time.Sleep(1 * time.Second)) so tests can shorten it.
var azureMFAPollInterval = 1 * time.Second

// Atmos fork: azureMFAWaitTimeout is the overall elapsed-time limit for Azure MFA approval. The poll
// count alone does not bound the wait, because each poll request can itself take a long time.
var azureMFAWaitTimeout = 5 * time.Minute

// Atmos fork: upstream looped without a limit, so a server that kept returning an MFA page
// prompted for codes, or polled, forever. The limits below end the loop with ErrMFAAttemptsExceeded.
const (
	// maxMFAPromptAttempts is the number of one-time codes submitted before giving up.
	maxMFAPromptAttempts = 5
	// maxAzureMFAPolls is the number of Azure MFA approval polls before giving up
	// (about five minutes at the default one-second poll interval).
	maxAzureMFAPolls = 300
)

// ErrMFAAttemptsExceeded is returned when the server keeps asking for MFA after the attempt limit.
var ErrMFAAttemptsExceeded = errors.New("too many MFA attempts")

// New create a new ADFS client
func New(idpAccount *cfg.IDPAccount) (*Client, error) {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		// Atmos fork: explicit TLS 1.2 floor (upstream left MinVersion unset). Not TLS 1.3:
		// renegotiation, which ADFS relies on, only exists up to TLS 1.2.
		TLSClientConfig: &tls.Config{MinVersion: provider.MinTLSVersion, InsecureSkipVerify: idpAccount.SkipVerify, Renegotiation: tls.RenegotiateFreelyAsClient}, //nolint:gosec // Skipping verification is an explicit, user-configured opt-in (skip_verify).
	}

	client, err := provider.NewHTTPClient(tr, provider.BuildHttpClientOpts(idpAccount))
	if err != nil {
		return nil, errors.Wrap(err, "error building http client")
	}

	return &Client{
		client:     client,
		idpAccount: idpAccount,
	}, nil
}

// Authenticate to ADFS and return the data from the body of the SAML assertion.
func (ac *Client) Authenticate(loginDetails *creds.LoginDetails) (string, error) {
	var authSubmitURL string
	var samlAssertion string
	var instructions string

	awsURN := url.QueryEscape(ac.idpAccount.AmazonWebservicesURN)

	adfsURL := buildSignOnURL(loginDetails.URL, awsURN)

	mfaToken := loginDetails.MFAToken

	doc, err := ac.get(adfsURL)
	if err != nil {
		return "", errors.Wrap(err, "failed to get adfs page")
	}

	authForm := url.Values{}

	doc.Find("input").Each(func(i int, s *goquery.Selection) {
		updateFormData(authForm, s, loginDetails)
	})

	doc.Find("form").Each(func(i int, s *goquery.Selection) {
		action, ok := s.Attr("action")
		if !ok {
			return
		}
		authSubmitURL = action
	})

	// Trim whitespace and discard empty values from Kmsi field
	if val, ok := authForm["Kmsi"]; ok {
		var trimmedKmsi []string
		var t string
		for _, s := range val {
			t = strings.TrimSpace(s)
			if len(t) > 0 {
				trimmedKmsi = append(trimmedKmsi, t)
			}
		}
		authForm["Kmsi"] = trimmedKmsi
	}

	if authSubmitURL == "" {
		return samlAssertion, fmt.Errorf("unable to locate IDP authentication form submit URL")
	} else {
		//
		// Atmos fork: resolve every action against the login page URL. Upstream resolved only actions that start
		// with "/", so "?step=password" or "step=password" reached the HTTP client without a scheme.
		// ResolveReference leaves absolute URLs unchanged.
		//
		// The base is the final login page URL (after redirects), not the initial request URL.
		parsedUrl := doc.Url
		if parsedUrl == nil {
			var err error
			if parsedUrl, err = url.Parse(adfsURL); err != nil {
				return "", errors.Wrap(err, "failed to parse ADFS URL")
			}
		}
		parsedPath, err := url.Parse(authSubmitURL)
		if err != nil {
			return "", errors.Wrap(err, "failed to parse authSubmitURL fragment")
		}
		authSubmitURL = parsedUrl.ResolveReference(parsedPath).String()
	}

	doc, err = ac.submit(authSubmitURL, authForm)
	if err != nil {
		return samlAssertion, errors.Wrap(err, "failed to submit adfs auth form")
	}

	mfaPromptAttempts, azureMFAPolls := 0, 0
	var azureCtx context.Context
	for {
		responseType, samlAssertion, err := checkResponse(doc)
		// Atmos fork: a parse error (for example a SAMLResponse input without a value) ends the
		// authentication before the response is dispatched, so a later MFA input cannot submit a code.
		if err != nil {
			return "", err
		}

		switch responseType {
		case SAML_RESPONSE:
			return samlAssertion, err
		case MFA_PROMPT:
			if mfaPromptAttempts >= maxMFAPromptAttempts {
				return "", fmt.Errorf("%w: the server asked for a one-time code %d times", ErrMFAAttemptsExceeded, mfaPromptAttempts)
			}
			mfaPromptAttempts++
			otpForm := url.Values{}
			if mfaToken == "" {
				mfaToken, err = prompter.RequestSecurityCode("000000")
				if err != nil {
					return samlAssertion, errors.Wrap(err, "error reading the MFA code")
				}
			}

			challengeScope(doc).Find("input").Each(func(i int, s *goquery.Selection) {
				updateOTPFormData(otpForm, s, mfaToken)
			})
			doc, err = ac.submit(challengeSubmitURL(doc, authSubmitURL), otpForm)
			if err != nil {
				return samlAssertion, errors.Wrap(err, "error retrieving mfa form results")
			}
			mfaToken = ""
		case AZURE_MFA_SERVER_WAIT:
			fallthrough
		case AZURE_MFA_WAIT:
			if azureMFAPolls >= maxAzureMFAPolls {
				return "", fmt.Errorf("%w: no Azure MFA approval after %d polls", ErrMFAAttemptsExceeded, azureMFAPolls)
			}
			azureMFAPolls++
			if azureCtx == nil {
				var cancel context.CancelFunc
				azureCtx, cancel = context.WithTimeout(context.Background(), azureMFAWaitTimeout)
				defer cancel()
			}
			doc, err = ac.pollAzureMFA(azureCtx, doc, authSubmitURL, responseType, &instructions)
			if err != nil {
				return samlAssertion, err
			}
		case UNKNOWN:
			return samlAssertion, errors.New("unable to classify response from auth server")
		}
	}
}

// pollAzureMFA waits one poll interval, then submits the Azure MFA wait form and returns the response.
// Atmos fork: the wait and the request are bound by ctx, so the overall approval time is limited
// whatever the server's response time. The count cap in Authenticate remains a separate guard.
func (ac *Client) pollAzureMFA(ctx context.Context, doc *goquery.Document, fallbackURL string, responseType AuthResponseType, instructions *string) (*goquery.Document, error) {
	azureForm := url.Values{}
	challengeScope(doc).Find("input").Each(func(i int, s *goquery.Selection) {
		updatePassthroughFormData(azureForm, s)
	})
	sel := doc.Find("p#validEntropyNumber")
	if sel.Index() != -1 {
		if *instructions != sel.Text() {
			*instructions = sel.Text()
			log.Println("Open your Microsoft Authenticator app and tap the number you see below to sign in.")
			log.Println(*instructions)
		}
	}
	sel = doc.Find("p#instructions")
	if sel.Index() != -1 {
		if *instructions != sel.Text() {
			*instructions = sel.Text()
			log.Println(*instructions)
		}
	}

	timer := time.NewTimer(azureMFAPollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: no Azure MFA approval within %s", ErrMFAAttemptsExceeded, azureMFAWaitTimeout)
	case <-timer.C:
	}

	doc, err := ac.submitContext(ctx, challengeSubmitURL(doc, fallbackURL), azureForm)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: no Azure MFA approval within %s", ErrMFAAttemptsExceeded, azureMFAWaitTimeout)
		}
		return nil, errors.Wrap(err, "error retrieving mfa form results")
	}
	if responseType == AZURE_MFA_SERVER_WAIT {
		sel := doc.Find("label#errorText")
		if sel.Index() != -1 {
			return nil, errors.New(sel.Text())
		}
	}
	return doc, nil
}

// buildSignOnURL returns the IdP-initiated sign-on URL for the relying party.
// Atmos fork: the configured URL may already be the full sign-on URL (the docs ask for the IdP's SSO URL).
// An existing sign-on endpoint is reused as is, and loginToRp is added only when it is missing, instead of
// appending a second endpoint inside the query. A base URL is handled as before.
func buildSignOnURL(baseURL, awsURN string) string {
	if parsed, err := url.Parse(baseURL); err == nil && strings.Contains(strings.ToLower(parsed.Path), "/adfs/ls/idpinitiatedsignon.aspx") {
		query := parsed.Query()
		if !query.Has("loginToRp") {
			if parsed.RawQuery != "" {
				parsed.RawQuery += "&"
			}
			parsed.RawQuery += "loginToRp=" + awsURN
		}
		return parsed.String()
	}
	return fmt.Sprintf("%s/adfs/ls/IdpInitiatedSignOn.aspx?loginToRp=%s", baseURL, awsURN)
}

func (ac *Client) get(url string) (*goquery.Document, error) {
	res, err := ac.client.Get(url)
	if err != nil {
		return nil, errors.Wrap(err, "error retieving form")
	}
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build document from response")
	}
	setDocumentURL(doc, res)
	return doc, nil
}

func (ac *Client) submit(url string, form url.Values) (*goquery.Document, error) {
	return ac.submitContext(context.Background(), url, form)
}

func (ac *Client) submitContext(ctx context.Context, url string, form url.Values) (*goquery.Document, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, errors.Wrap(err, "error building request")
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err := ac.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "error submitting form")
	}
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build document from response")
	}
	setDocumentURL(doc, res)
	return doc, nil
}

// setDocumentURL records the final request URL (after redirects) on a reader-based document.
// Atmos fork: goquery leaves Document.Url nil for documents built from a reader, and form actions
// must be resolved against the URL of the response that contained the form.
func setDocumentURL(doc *goquery.Document, res *http.Response) {
	if doc.Url == nil && res.Request != nil {
		doc.Url = res.Request.URL
	}
}

// challengeSubmitURL returns the URL an MFA challenge form must be submitted to: the form's own
// action resolved against the URL of the response that contained it.
// Atmos fork: upstream posted every MFA form to the initial login form's action, which fails when the
// IdP routes the challenge to a different endpoint (such as /adfs/ls/mfa). When the form has no action,
// or it cannot be resolved, the fallback (the login form's action) is used, as before.
func challengeSubmitURL(doc *goquery.Document, fallback string) string {
	var action string
	// Atmos fork: the action comes from the form that owns the challenge controls, not from whichever form
	// is last on the page. Pages without such a form keep the previous behavior.
	forms := doc.Find("form")
	if owner := challengeForm(doc); owner != nil {
		forms = owner
	}
	forms.Each(func(_ int, s *goquery.Selection) {
		if val, ok := s.Attr("action"); ok {
			action = val
		}
	})
	if action == "" || doc.Url == nil {
		return fallback
	}
	parsed, err := url.Parse(action)
	if err != nil {
		return fallback
	}
	return doc.Url.ResolveReference(parsed).String()
}

// challengeForm returns the form that owns the MFA challenge, or nil when no form contains challenge controls.
//
// Atmos fork: the form holding the control that classified the page (a one-time code input for an MFA prompt,
// the matching Azure AuthMethod value for an Azure wait) is preferred, so an earlier unrelated form, such as an
// options form with its own AuthMethod input, is not submitted instead. Otherwise the first form that contains
// any challenge control (a one-time code, security code, challenge answer or AuthMethod input) is used.
func challengeForm(doc *goquery.Document) *goquery.Selection {
	responseType, _, _ := checkResponse(doc)
	if owner := firstFormWithInput(doc, func(name, value string) bool {
		return classifiesChallenge(responseType, name, value)
	}); owner != nil {
		return owner
	}
	return firstFormWithInput(doc, func(name, _ string) bool {
		return strings.EqualFold(name, "AuthMethod") || isCodeControl(name)
	})
}

// firstFormWithInput returns the first form that has an input matching the predicate, or nil.
func firstFormWithInput(doc *goquery.Document, matches func(name, value string) bool) *goquery.Selection {
	var owner *goquery.Selection
	doc.Find("form").EachWithBreak(func(_ int, f *goquery.Selection) bool {
		found := false
		f.Find("input").EachWithBreak(func(_ int, in *goquery.Selection) bool {
			name, _ := in.Attr("name")
			value, _ := in.Attr("value")
			found = matches(name, value)
			return !found
		})
		if found {
			owner = f
		}
		return !found
	})
	return owner
}

// isCodeControl reports whether an input name is a one-time code, security code or challenge answer control.
func isCodeControl(name string) bool {
	lname := strings.ToLower(name)
	return strings.Contains(lname, "security_code") || strings.Contains(lname, "verificationcode") ||
		strings.Contains(lname, "challengequestionanswer") || strings.Contains(lname, "oathcode")
}

// classifiesChallenge reports whether an input is the control that gives a page its MFA response type.
func classifiesChallenge(responseType AuthResponseType, name, value string) bool {
	switch responseType {
	case MFA_PROMPT:
		if name == "AuthMethod" {
			return value == "VIPAuthenticationProviderWindowsAccountName" || value == "VIPAuthenticationProviderUPN" ||
				value == "Defender AD FS Adapter"
		}
		return isCodeControl(name)
	case AZURE_MFA_WAIT:
		return name == "AuthMethod" && value == "AzureMfaAuthentication"
	case AZURE_MFA_SERVER_WAIT:
		return name == "AuthMethod" && value == "AzureMfaServerAuthentication"
	default:
		return false
	}
}

// challengeScope returns the selection whose inputs make up an MFA submission: the form that owns the
// challenge controls, or the whole page when there is no such form.
func challengeScope(doc *goquery.Document) *goquery.Selection {
	if owner := challengeForm(doc); owner != nil {
		return owner
	}
	return doc.Selection
}

func checkResponse(doc *goquery.Document) (AuthResponseType, string, error) {
	samlAssertion := ""
	responseType := UNKNOWN
	var checkErr error

	doc.Find("input").Each(func(i int, s *goquery.Selection) {
		name, ok := s.Attr("name")
		if !ok {
			// Atmos fork: upstream called log.Fatalf here, terminating the whole process
			// whenever the response page contained an unnamed <input> (for example a
			// submit button). Unnamed inputs carry no form data, so skip them.
			return
		}
		if name == "SAMLResponse" {
			val, ok := s.Attr("value")
			if !ok {
				// Atmos fork: upstream called log.Fatalf here; return an error instead.
				checkErr = errors.New("unable to locate saml assertion value")
			}
			samlAssertion = val
			responseType = SAML_RESPONSE
		}
		if name == "OathCode" {
			responseType = MFA_PROMPT
		}
		if name == "AuthMethod" {
			val, _ := s.Attr("value")
			switch val {
			case "VIPAuthenticationProviderWindowsAccountName", "VIPAuthenticationProviderUPN", "Defender AD FS Adapter":
				responseType = MFA_PROMPT
			case "AzureMfaAuthentication":
				responseType = AZURE_MFA_WAIT
			case "AzureMfaServerAuthentication":
				responseType = AZURE_MFA_SERVER_WAIT
			}
		}
		if name == "VerificationCode" {
			responseType = MFA_PROMPT
		}
	})
	return responseType, samlAssertion, checkErr
}

func updateFormData(authForm url.Values, s *goquery.Selection, user *creds.LoginDetails) {
	name, ok := s.Attr("name")
	if !ok {
		return
	}

	typeValue, typeFound := s.Attr("type")
	hiddenAttr := typeFound && typeValue == "hidden"

	lname := strings.ToLower(name)
	if strings.Contains(lname, "user") {
		if !hiddenAttr {
			authForm.Add(name, user.Username)
		}
	} else if strings.Contains(lname, "email") {
		if !hiddenAttr {
			authForm.Add(name, user.Username)
		}
	} else if strings.Contains(lname, "pass") {
		if !hiddenAttr {
			authForm.Add(name, user.Password)
		}
	} else {
		updatePassthroughFormData(authForm, s)
	}
}

func updateOTPFormData(otpForm url.Values, s *goquery.Selection, token string) {
	name, ok := s.Attr("name")
	if !ok {
		return
	}
	lname := strings.ToLower(name)
	if strings.Contains(lname, "security_code") {
		otpForm.Add(name, token)
	} else if strings.Contains(lname, "verificationcode") {
		otpForm.Add(name, token)
	} else if strings.Contains(lname, "challengequestionanswer") {
		otpForm.Add(name, token)
	} else if strings.Contains(lname, "oathcode") {
		otpForm.Add(name, token)
	} else {
		updatePassthroughFormData(otpForm, s)
	}
}

func updatePassthroughFormData(otpForm url.Values, s *goquery.Selection) {
	name, ok := s.Attr("name")
	if !ok {
		return
	}
	val, ok := s.Attr("value")
	if !ok {
		return
	}
	otpForm.Add(name, val)
}
