package okta

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/marshallbrekka/go-u2fhost"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"golang.org/x/net/publicsuffix"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/cfg"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/helper/credentials"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/page"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/prompter"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/provider"
)

const (
	IdentifierDuoMfa          = "DUO WEB"
	IdentifierSmsMfa          = "OKTA SMS"
	IdentifierEmailMfa        = "OKTA EMAIL"
	IdentifierPushMfa         = "OKTA PUSH"
	IdentifierTotpMfa         = "GOOGLE TOKEN:SOFTWARE:TOTP"
	IdentifierOktaTotpMfa     = "OKTA TOKEN:SOFTWARE:TOTP"
	IdentifierSymantecTotpMfa = "SYMANTEC TOKEN"
	IdentifierFIDOWebAuthn    = "FIDO WEBAUTHN"
	IdentifierYubiMfa         = "YUBICO TOKEN:HARDWARE"
)

var logger = logrus.WithField("provider", "okta")

// errMFAPollTimeout is returned when an MFA push is still pending after mfaPollTimeout.
var errMFAPollTimeout = errors.New("timed out waiting for the MFA push to be approved")

// Atmos fork: the polling/settle delays and the U2F device finder factory are package
// variables (instead of inline literals) so tests can shorten the delays and inject a
// mock device without real sleeps or attached hardware. Production values are unchanged.
var (
	// pushPollInterval is the delay between Okta Verify push status checks.
	pushPollInterval = 3 * time.Second
	// duoPollInterval is the delay between Duo push status checks.
	duoPollInterval = 3 * time.Second
	// mfaPollTimeout bounds how long the Okta Verify push and Duo push polling loops wait for a terminal answer.
	// Atmos fork: upstream polled without a limit when the IdP kept answering with a non-terminal result.
	mfaPollTimeout = 5 * time.Minute
	// endpointHealthTimeout bounds the Duo endpoint health requests.
	endpointHealthTimeout = 60 * time.Second
	// endpointHealthSettleDelay is the delay before re-submitting the Duo auth form after an endpoint health report.
	endpointHealthSettleDelay = 2 * time.Second
	// newU2FDeviceFinder returns the DeviceFinder used for Duo U2F and Okta FIDO challenges.
	newU2FDeviceFinder = func() DeviceFinder { return new(U2FDeviceFinder) }
)

var supportedMfaOptions = map[string]string{
	IdentifierDuoMfa:          "DUO MFA authentication",
	IdentifierSmsMfa:          "SMS MFA authentication",
	IdentifierEmailMfa:        "EMAIL MFA authentication",
	IdentifierPushMfa:         "PUSH MFA authentication",
	IdentifierTotpMfa:         "TOTP MFA authentication",
	IdentifierOktaTotpMfa:     "Okta MFA authentication",
	IdentifierSymantecTotpMfa: "Symantec VIP MFA authentication",
	IdentifierFIDOWebAuthn:    "FIDO WebAuthn MFA authentication",
	IdentifierYubiMfa:         "YUBICO TOKEN:HARDWARE",
}

// Client is a wrapper representing a Okta SAML client
type Client struct {
	provider.ValidateBase

	client          *provider.HTTPClient
	mfa             string
	targetURL       string
	disableSessions bool
	rememberDevice  bool
}

// AuthRequest represents an mfa okta request
type AuthRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	StateToken string `json:"stateToken,omitempty"`
}

// VerifyRequest represents an mfa verify request
type VerifyRequest struct {
	StateToken     string `json:"stateToken"`
	PassCode       string `json:"passCode,omitempty"`
	RememberDevice string `json:"rememberDevice,omitempty"` // This is needed to remember Okta MFA device
}

// Articles referencing the Okta MFA + remembering device
// https://developer.okta.com/docs/reference/api/authn/#verify-security-question-factor
// https://devforum.okta.com/t/how-per-device-remember-me-api-works/3955/3

// SessionRequst holds the SessionToken used to create an Okta Session
type SessionRequst struct {
	SessionToken string `json:"sessionToken"`
}

// mfaChallengeContext is used to hold MFA challenge context in a simple struct.
type mfaChallengeContext struct {
	factorID              string
	oktaVerify            string
	mfaIdentifer          string
	challengeResponseBody string
}

// New creates a new Okta client
func New(idpAccount *cfg.IDPAccount) (*Client, error) {
	tr := provider.NewDefaultTransport(idpAccount.SkipVerify)

	client, err := provider.NewHTTPClient(tr, provider.BuildHttpClientOpts(idpAccount))
	if err != nil {
		return nil, errors.Wrap(err, "error building http client")
	}

	// assign a response validator to ensure all responses are either success or a redirect
	// this is to avoid have explicit checks for every single response
	client.CheckResponseStatus = provider.SuccessOrRedirectResponseValidator

	// add cookie jar to keep track of cookies during okta login flow
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, errors.Wrap(err, "error building cookie jar")
	}
	client.Jar = jar

	disableSessions := idpAccount.DisableSessions
	rememberDevice := !idpAccount.DisableRememberDevice

	if idpAccount.DisableSessions { // if user disabled sessions, also dont remember device
		rememberDevice = false
	}

	// Debug the disableSessions and rememberDevice values
	logger.Debugf("okta | disableSessions: %v", disableSessions)
	logger.Debugf("okta | rememberDevice: %v", rememberDevice)

	return &Client{
		client:          client,
		mfa:             idpAccount.MFA,
		targetURL:       idpAccount.TargetURL,
		disableSessions: disableSessions,
		rememberDevice:  rememberDevice,
	}, nil
}

type ctxKey string

// createSession calls the Okta sessions API to create a new session using the sessionToken passed in
func (oc *Client) createSession(loginDetails *creds.LoginDetails, sessionToken string) (string, string, error) {
	logger.Debug("create session func called")
	if loginDetails == nil || sessionToken == "" {
		// Atmos fork: upstream logged loginDetails with %v, which includes the password, and the
		// session token. Log only whether each input is present.
		logger.Debugf("unable to create an Okta session, nil input | loginDetails set: %t | sessionToken set: %t", loginDetails != nil, sessionToken != "")
		return "", "", fmt.Errorf("unable to create an okta session, nil input")
	}

	oktaURL, err := url.Parse(loginDetails.URL)
	if err != nil {
		return "", "", errors.Wrap(err, "error building okta url")
	}

	oktaOrgHost := oktaURL.Host

	// authenticate via okta api
	sessionReq := SessionRequst{SessionToken: sessionToken}
	sessionReqBody := new(bytes.Buffer)
	err = json.NewEncoder(sessionReqBody).Encode(sessionReq) //nolint:gosec // Okta's sessions API requires the sessionToken field in the request body.
	if err != nil {
		return "", "", errors.Wrap(err, "error encoding session req")
	}

	sessionReqURL := fmt.Sprintf("https://%s/api/v1/sessions", oktaOrgHost)

	req, err := http.NewRequest("POST", sessionReqURL, sessionReqBody)
	if err != nil {
		return "", "", errors.Wrap(err, "error building new session request")
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	res, err := oc.client.Do(req)
	if err != nil {
		return "", "", errors.Wrap(err, "error retrieving session response")
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", "", errors.Wrap(err, "error retrieving body from response")
	}

	if res.StatusCode != 200 { // https://developer.okta.com/docs/reference/api/sessions/#response-parameters
		if res.StatusCode == 401 {
			return "", "", fmt.Errorf("unable to create an Okta session, invalid sessionToken")
		}
		return "", "", fmt.Errorf("unable to create an Okta session, HTTP Code: %d", res.StatusCode)
	}

	resp := string(body)

	oktaSessionExpiresAtStr := gjson.Get(resp, "expiresAt").String()
	logger.Debugf("okta session expires at: %s", oktaSessionExpiresAtStr)

	oktaSessionCookie := gjson.Get(resp, "id").String()

	err = credentials.SaveCredentials(loginDetails.URL+"/sessionCookie", loginDetails.Username, oktaSessionCookie)
	if err != nil {
		return "", "", fmt.Errorf("error storing okta session token | err: %w", err)
	}

	oktaSessionToken := gjson.Get(resp, "sessionToken").String()
	sessionResponseStatus := gjson.Get(resp, "status").String()
	switch sessionResponseStatus {
	case "ACTIVE":
		logger.Debug("okta session established")
	case "MFA_REQUIRED":
		oktaSessionToken, err = verifyMfa(oc, oktaOrgHost, loginDetails, resp)
		if err != nil {
			return "", "", errors.Wrap(err, "error verifying MFA")
		}
	case "MFA_ENROLL":
		// Not yet fully implemented, most likely no need, so just return the status as the error string...
		return "", "", fmt.Errorf("MFA_ENROLL")
	}

	return oktaSessionCookie, oktaSessionToken, nil
}

// validateSession calls the Okta session API to check if the session is valid
// returns an error if the session is NOT valid
func (oc *Client) validateSession(loginDetails *creds.LoginDetails) error {
	logger.Debug("validate session func called")

	if loginDetails == nil {
		logger.Debug("unable to validate the okta session, nil input")
		return fmt.Errorf("unable to validate the okta session, nil input")
	}

	sessionCookie := loginDetails.OktaSessionCookie

	oktaURL, err := url.Parse(loginDetails.URL)
	if err != nil {
		return errors.Wrap(err, "error building oktaURL")
	}

	oktaOrgHost := oktaURL.Host

	sessionReqURL := fmt.Sprintf("https://%s/api/v1/sessions/me", oktaOrgHost) // This api endpoint returns user details
	sessionReqBody := new(bytes.Buffer)

	req, err := http.NewRequest("GET", sessionReqURL, sessionReqBody)
	if err != nil {
		return errors.Wrap(err, "error building new session request")
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Cookie", fmt.Sprintf("sid=%s", sessionCookie))

	res, err := oc.client.Do(req)
	if err != nil {
		return errors.Wrap(err, "error retrieving session response")
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return errors.Wrap(err, "error retrieving body from response")
	}

	resp := string(body)

	if res.StatusCode != 200 {
		logger.Debug("invalid okta session")
		return fmt.Errorf("invalid okta session")
	} else {
		sessionResponseStatus := gjson.Get(resp, "status").String()
		switch sessionResponseStatus {
		case "ACTIVE":
			logger.Debug("okta session established")
		case "MFA_REQUIRED":
			_, err := verifyMfa(oc, oktaOrgHost, loginDetails, resp)
			if err != nil {
				return errors.Wrap(err, "error verifying MFA")
			}
		case "MFA_ENROLL":
			// Not yet fully implemented, so just return the status as the error string...
			return fmt.Errorf("MFA_ENROLL")
		}
	}

	logger.Debug("valid okta session")
	return nil
}

// authWithSession authenticates user via sessions API -> direct to target URL using follow func
func (oc *Client) authWithSession(loginDetails *creds.LoginDetails) (string, error) {
	logger.Debug("auth with session func called")
	sessionCookie := loginDetails.OktaSessionCookie
	err := oc.validateSession(loginDetails)
	if err != nil {
		modifiedLoginDetails := loginDetails
		modifiedLoginDetails.OktaSessionCookie = ""
		return oc.Authenticate(modifiedLoginDetails)
	}

	req, err := http.NewRequest("GET", loginDetails.URL, nil)
	if err != nil {
		return "", errors.Wrap(err, "error building authWithSession request")
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Cookie", fmt.Sprintf("sid=%s", sessionCookie))

	ctx := context.WithValue(context.Background(), ctxKey("authWithSession"), loginDetails)

	res, err := oc.client.Do(req)
	if err != nil {
		logger.Debugf("error authing with session: %v", err)
		// Atmos fork: a transport error yields a nil response; upstream dereferenced it and panicked.
		if res == nil {
			return "", errors.Wrap(err, "error authing with session")
		}
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		logger.Debugf("error reading body for auth with session: %v", err)
		// The response is unusable; request the app page again, as before.
		return oc.follow(ctx, req, loginDetails)
	}

	// This usually happens if using an active session (> 5 mins) but MFA was NOT remembered
	if strings.Contains(string(body), "/login/step-up/") { // https://developer.okta.com/docs/reference/api/authn/#step-up-authentication-with-okta-session
		logger.Debug("okta step-up prompted, need mfa...")
		stateToken, err := getStateTokenFromOktaPageBody(string(body))
		if err != nil {
			return "", errors.Wrap(err, "error retrieving saml response")
		}
		loginDetails.StateToken = stateToken
		return oc.Authenticate(loginDetails)
	}

	// Atmos fork: upstream requested the app page a second time in follow(). The response already read
	// above is handled directly, so a valid assertion in it is not lost when the second request fails.
	return oc.followResponse(ctx, req, res, body, loginDetails)
}

// getDeviceTokenFromOkta creates a dummy HTTP call to Okta and returns the device token
// cookie value
// This function is not currently used and but can be used in the future
func (oc *Client) getDeviceTokenFromOkta(loginDetails *creds.LoginDetails) (string, error) {
	// dummy request to set device token cookie ("dt")
	req, err := http.NewRequest("GET", loginDetails.URL, nil)
	if err != nil {
		return "", errors.Wrap(err, "error building device token request")
	}
	resp, err := oc.client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "error retrieving device token")
	}
	defer resp.Body.Close()

	for _, c := range resp.Cookies() {
		if c.Name == "DT" { // Device token
			return c.Value, nil
		}
	}

	return "", fmt.Errorf("unable to get a device token from okta")
}

// setDeviceTokenCookie sets the DT cookie in the HTTP Client cookie jar
// using the okta_<loginDetails.Username>_saml2aws, we reduce making an extra api call
// this func can be uplifted in the future to set custom device tokens or used with
// getDeviceTokenFromOkta function
func (oc *Client) setDeviceTokenCookie(loginDetails *creds.LoginDetails) error {
	// getDeviceTokenFromOkta is not used but doing this to keep the function code
	// uncommented (avoid linting issues)
	if false {
		dt, _ := oc.getDeviceTokenFromOkta(loginDetails)
		logger.Debugf("getDeviceTokenFromOkta is not yet implemented: device token set: %t", dt != "")
	}

	oktaURL, err := url.Parse(loginDetails.URL)
	if err != nil {
		return errors.Wrap(err, "error building oktaURL to set device token cookie")
	}
	oktaURLScheme := oktaURL.Scheme
	oktaURLHost := oktaURL.Host
	baseURL := &url.URL{Scheme: oktaURLScheme, Host: oktaURLHost, Path: "/"}

	var cookies []*http.Cookie
	cookie := http.Cookie{ //nolint:gosec // HttpOnly and SameSite apply to Set-Cookie responses; this cookie only seeds the client jar and is Secure.
		Name:    "DT",
		Secure:  true,
		Expires: time.Now().Add(time.Hour * 24 * 30),                    // 30 Days -> this time might not matter as this cookie is set on every saml2aws login request
		Value:   fmt.Sprintf("okta_%s_saml2aws", loginDetails.Username), // Okta recommends using an UUID but this should be unique enough. Also, this is key to remembering Okta MFA device
	}
	cookies = append(cookies, &cookie)
	oc.client.Jar.SetCookies(baseURL, cookies)

	return nil
}

// primaryAuth creates the Okta Primary Authentication request
// returns the authStatus, sessionToken, http response and a error
func (oc *Client) primaryAuth(loginDetails *creds.LoginDetails) (string, string, string, error) {
	oktaURL, err := url.Parse(loginDetails.URL)
	if err != nil {
		return "", "", "", errors.Wrap(err, "error building oktaURL")
	}

	oktaOrgHost := oktaURL.Host
	// authenticate via okta api
	authReq := AuthRequest{Username: loginDetails.Username, Password: loginDetails.Password}
	if loginDetails.StateToken != "" {
		authReq = AuthRequest{StateToken: loginDetails.StateToken}
	}
	authBody := new(bytes.Buffer)
	err = json.NewEncoder(authBody).Encode(authReq) //nolint:gosec // Okta's primary-auth API requires the password field in the request body.
	if err != nil {
		return "", "", "", errors.Wrap(err, "error encoding authreq")
	}

	authSubmitURL := fmt.Sprintf("https://%s/api/v1/authn", oktaOrgHost)

	req, err := http.NewRequest("POST", authSubmitURL, authBody)
	if err != nil {
		return "", "", "", errors.Wrap(err, "error building authentication request")
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	res, err := oc.client.Do(req)
	if err != nil {
		return "", "", "", errors.Wrap(err, "error retrieving auth response")
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", "", "", errors.Wrap(err, "error retrieving body from response")
	}

	resp := string(body)

	authStatus := gjson.Get(resp, "status").String()
	oktaSessionToken := gjson.Get(resp, "sessionToken").String()

	return authStatus, oktaSessionToken, resp, nil
}

// Authenticate logs into Okta and returns a SAML response
func (oc *Client) Authenticate(loginDetails *creds.LoginDetails) (string, error) {
	// Set Okta device token
	err := oc.setDeviceTokenCookie(loginDetails)
	if err != nil {
		return "", errors.Wrap(err, "error setting device token in cookie jar")
	}

	// Get Okta session cookie (sid) from login details (if found via login.go)
	oktaSessionCookie := loginDetails.OktaSessionCookie

	// If user disabled sessions, do not use sessions API
	if !oc.disableSessions {
		// If Okta session cookie is not empty
		// Note on checking StateToken: StateToken is set in the follow func
		// if the follow func calls this function (Authenticate), it means the session requires MFA to continue
		// so don't call authWithSession, instead flow through to create the primary authentication call
		if oktaSessionCookie != "" && loginDetails.StateToken == "" {
			return oc.authWithSession(loginDetails)
		}
	}

	oktaURL, err := url.Parse(loginDetails.URL)
	if err != nil {
		return "", errors.Wrap(err, "error building oktaURL")
	}

	oktaOrgHost := oktaURL.Host

	authStatus, oktaSessionToken, primaryAuthResp, err := oc.primaryAuth(loginDetails)
	if err != nil {
		return "", err
	}

	switch authStatus {
	case "MFA_REQUIRED":
		oktaSessionToken, err = verifyMfa(oc, oktaOrgHost, loginDetails, primaryAuthResp)
		if err != nil {
			return "", errors.Wrap(err, "error verifying MFA")
		}
	case "LOCKED_OUT":
		return "", errors.New("the account is locked")
	}

	// if user disabled sessions, default to using standard login WITHOUT sessions
	if oc.disableSessions {
		// now call saml endpoint
		oktaSessionRedirectURL := fmt.Sprintf("https://%s/login/sessionCookieRedirect", oktaOrgHost)

		req, err := http.NewRequest("GET", oktaSessionRedirectURL, nil)
		if err != nil {
			return "", errors.Wrap(err, "error building authentication request")
		}
		q := req.URL.Query()
		q.Add("checkAccountSetupComplete", "true")
		q.Add("token", oktaSessionToken)
		q.Add("redirectUrl", loginDetails.URL)
		req.URL.RawQuery = q.Encode()

		ctx := context.WithValue(context.Background(), ctxKey("login"), loginDetails)
		return oc.follow(ctx, req, loginDetails)
	}

	// Only reaches here if user DID NOT DISABLE okta sessions
	if oktaSessionCookie == "" {
		oktaSessionCookie, _, err = oc.createSession(loginDetails, oktaSessionToken)
		if err != nil {
			return "", err
		}
		loginDetails.OktaSessionCookie = oktaSessionCookie
	}

	return oc.authWithSession(loginDetails)
}

func (oc *Client) follow(ctx context.Context, req *http.Request, loginDetails *creds.LoginDetails) (string, error) {
	if ctx.Value(ctxKey("follow")) != nil {
		logger.Debug("follow func called from itself")
	}

	if ctx.Value(ctxKey("authWithSession")) != nil {
		logger.Debug("follow func called from auth with session func")
	}

	res, err := oc.client.Do(req)
	if err != nil {
		logger.Debug("ERROR FOLLOWING")
		return "", errors.Wrap(err, "error following")
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		logger.Debug("FAILED TO READ RESP BODY")
		return "", errors.Wrap(err, "failed to build document from response")
	}
	return oc.followResponse(ctx, req, res, body, loginDetails)
}

// followResponse handles an already fetched response (res, with its body read into body) in the SAML redirect flow.
func (oc *Client) followResponse(ctx context.Context, req *http.Request, res *http.Response, body []byte, loginDetails *creds.LoginDetails) (string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		logger.Debug("FAILED TO BUILD DOC FROM RESP")
		return "", errors.Wrap(err, "failed to build document from response")
	}
	// Atmos fork: reader-based documents have no URL; keep the response URL so relative form actions can be resolved.
	if doc.Url == nil && res.Request != nil {
		doc.Url = res.Request.URL
	}

	var handler func(context.Context, *goquery.Document) (context.Context, *http.Request, error)

	if targetForm := findTargetForm(doc, oc.targetURL); targetForm.Size() > 0 {
		logger.WithField("type", "saml-response-to-aws").Debug("doc detect")
		// Atmos fork: the assertion is read from the form that posts to the target, not from the first
		// SAMLResponse input on the page, which could belong to an intermediate form.
		if samlResponse, ok := extractSAMLResponse(targetForm); ok {
			decodedSamlResponse, err := base64.StdEncoding.DecodeString(samlResponse)
			if err != nil {
				return "", errors.Wrap(err, "failed to decode saml-response")
			}
			// Atmos fork: upstream logged the full decoded SAML response. The assertion is a bearer
			// credential (it can be exchanged for AWS credentials until it expires), so only its size is logged.
			logger.WithField("type", "saml-response").WithField("saml-response-bytes", len(decodedSamlResponse)).Debug("doc detect")
			return samlResponse, nil
		}
	} else if docIsFormSamlRequest(doc) {
		logger.WithField("type", "saml-request").Debug("doc detect")
		handler = oc.handleFormRedirect
	} else if docIsFormResume(doc) {
		logger.WithField("type", "resume").Debug("doc detect")
		handler = oc.handleFormRedirect
	} else if docIsFormSamlResponse(doc) {
		logger.WithField("type", "saml-response").Debug("doc detect")
		handler = oc.handleFormRedirect
	} else {
		stateToken, err := oc.getStateToken(req, loginDetails)
		if err != nil {
			return "", errors.Wrap(err, "failed to getStateToken")
		}
		loginDetails.StateToken = stateToken
		return oc.Authenticate(loginDetails)
	}

	if handler == nil {
		// Atmos fork: upstream logged the whole page, which can carry state tokens and other secrets.
		logger.WithField("forms", doc.Find("form").Size()).Debug("Unknown document type")
		return "", errors.New("unknown document type")
	}

	ctx, req, err = handler(ctx, doc)
	if err != nil {
		return "", err
	}
	return oc.follow(ctx, req, loginDetails)
}

func (oc *Client) getStateToken(req *http.Request, loginDetails *creds.LoginDetails) (string, error) {
	url, err := url.Parse(loginDetails.URL)
	if err != nil {
		return "", errors.Wrap(err, "error building app request")
	}

	req.URL = url
	req.Method = "GET"
	req.Body = nil

	res, err := oc.client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "error retrieving app response")
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", errors.Wrap(err, "error retrieving body from response")
	}
	stateToken, err := getStateTokenFromOktaPageBody(string(body))
	if err != nil {
		return "", errors.Wrap(err, "error retrieving saml response")
	}
	return stateToken, nil
}

func getStateTokenFromOktaPageBody(responseBody string) (string, error) {
	regexes := []*regexp.Regexp{
		regexp.MustCompile("var stateToken = [\"|'](.*)[\"|'];"),
		// Found on the "extra verification" page
		// hiding in a Javascript object
		regexp.MustCompile(`"stateToken":"([^"]*)"`),
	}

	for _, re := range regexes {
		match := re.FindStringSubmatch(responseBody)
		if len(match) >= 2 {
			return strings.ReplaceAll(match[1], `\x2D`, "-"), nil
		}
	}

	return "", errors.New("cannot find state token")
}

func parseMfaIdentifer(json string, arrayPosition int) (string, string, string) {
	mfaProvider := gjson.Get(json, fmt.Sprintf("_embedded.factors.%d.provider", arrayPosition)).String()
	factorType := strings.ToUpper(gjson.Get(json, fmt.Sprintf("_embedded.factors.%d.factorType", arrayPosition)).String())
	id := gjson.Get(json, fmt.Sprintf("_embedded.factors.%d.id", arrayPosition)).String()
	// Okta gives names to some authentication methods
	// displaying this name is useful when there's multiple auths of the same type. e.g. multiple FIDO options
	authName := gjson.Get(json, fmt.Sprintf("_embedded.factors.%d.profile.authenticatorName", arrayPosition)).String()
	return fmt.Sprintf("%s %s", mfaProvider, factorType), authName, id
}

func (oc *Client) handleFormRedirect(ctx context.Context, doc *goquery.Document) (context.Context, *http.Request, error) {
	form, err := page.NewFormFromDocument(doc, redirectFormSelector(doc))
	if err != nil {
		return ctx, nil, errors.Wrap(err, "error extracting redirect form")
	}
	req, err := form.BuildRequest()
	return ctx, req, err
}

// redirectFormSelector returns a selector for the form that carries the input which made follow() choose
// handleFormRedirect, in the same order as the dispatch. Atmos fork: upstream always took the first form
// with an action, which could be an unrelated form. The selected form may have no action; the form parser
// then submits it to the document URL. When the input that selected the branch is not inside a form, the
// empty selector keeps the upstream behavior.
func redirectFormSelector(doc *goquery.Document) string {
	for _, name := range []string{"SAMLRequest", "RelayState", "SAMLResponse"} {
		if doc.Find(fmt.Sprintf("input[name=\"%s\"]", name)).Size() != 1 {
			continue
		}
		// This is the input the dispatch selected, so no later input may choose a different form.
		selector := fmt.Sprintf("form:has(input[name=\"%s\"])", name)
		if doc.Find(selector).Size() > 0 {
			return selector
		}
		return ""
	}
	return ""
}

func docIsFormSamlRequest(doc *goquery.Document) bool {
	return doc.Find("input[name=\"SAMLRequest\"]").Size() == 1
}

func docIsFormSamlResponse(doc *goquery.Document) bool {
	return doc.Find("input[name=\"SAMLResponse\"]").Size() == 1
}

func docIsFormResume(doc *goquery.Document) bool {
	return doc.Find("input[name=\"RelayState\"]").Size() == 1
}

func docIsFormRedirectToTarget(doc *goquery.Document, target string) bool {
	return findTargetForm(doc, target).Size() > 0
}

// findTargetForm returns a form that posts to the target (or to an AWS sign-in endpoint when no target
// is configured). Atmos fork: among the forms that post there, the first one carrying a SAMLResponse
// value is preferred, so an auxiliary form with the same action cannot hide the assertion. Without such
// a form, the first matching form is returned. The selection is empty when no form posts there.
func findTargetForm(doc *goquery.Document, target string) *goquery.Selection {
	var urls []string
	if target != "" {
		url := fmt.Sprintf("form[action=\"%s\"]", target)
		urls = []string{url}
	} else {
		urls = []string{
			"form[action=\"https://signin.aws.amazon.com/saml\"]",
			"form[action=\"https://signin.amazonaws-us-gov.com/saml\"]",
			"form[action=\"https://signin.amazonaws.cn/saml\"]",
		}
	}

	var first *goquery.Selection
	for _, value := range urls {
		forms := doc.Find(value)
		if forms.Size() == 0 {
			continue
		}
		if first == nil {
			first = forms.First()
		}
		withAssertion := forms.FilterFunction(func(_ int, form *goquery.Selection) bool {
			_, ok := extractSAMLResponse(form)
			return ok
		})
		if withAssertion.Size() > 0 {
			return withAssertion.First()
		}
	}
	if first != nil {
		return first
	}
	return &goquery.Selection{}
}

// extractSAMLResponse reads the SAMLResponse input inside the given form selection.
func extractSAMLResponse(form *goquery.Selection) (v string, ok bool) {
	return form.Find("input[name=\"SAMLResponse\"]").Attr("value")
}

func findMfaOption(mfa string, mfaOptions []string, startAtIdx int) int {
	for idx, val := range mfaOptions {
		if startAtIdx > idx {
			continue
		}
		if strings.HasPrefix(strings.ToUpper(val), mfa) {
			return idx
		}
	}
	return 0
}

// forceRememberDeviceOff sets every rememberDevice query parameter of the factor verify link to false.
// Atmos fork: upstream kept a rememberDevice value that was already in the link, so a link carrying
// rememberDevice=true still asked Okta to remember the device when DisableSessions or
// DisableRememberDevice turned remembering off. The other query parameters are kept. A link that
// cannot be parsed, or that mentions rememberDevice outside its query, is returned unchanged.
func forceRememberDeviceOff(link string) string {
	parsed, err := url.Parse(link)
	if err != nil {
		return link
	}
	query := parsed.Query()
	values, ok := query["rememberDevice"]
	if !ok {
		return link
	}
	if len(values) == 1 && values[0] == "false" {
		return link
	}
	query.Set("rememberDevice", "false")
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

// nextUntriedMfaOption returns the first option, in list order, that matches mfa the way findMfaOption
// does and is not in tried. It reports false when every matching option was tried.
func nextUntriedMfaOption(mfa string, mfaOptions []string, tried map[int]bool) (int, bool) {
	for idx, val := range mfaOptions {
		if !tried[idx] && strings.HasPrefix(strings.ToUpper(val), mfa) {
			return idx, true
		}
	}
	return 0, false
}

func getMfaChallengeContext(oc *Client, mfaOption int, resp string) (*mfaChallengeContext, error) {
	return getMfaChallengeContextWithContext(context.Background(), oc, mfaOption, resp)
}

// sleepUntilDone waits for d, or returns errMFAPollTimeout as soon as ctx ends.
// Atmos fork: used by the push polling loops so one deadline bounds both the waits and the requests.
func sleepUntilDone(ctx context.Context, d time.Duration) error {
	if ctx.Err() != nil {
		return errMFAPollTimeout
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errMFAPollTimeout
	case <-timer.C:
		return nil
	}
}

// pollError reports errMFAPollTimeout when the polling deadline has expired, and err otherwise.
func pollError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return errMFAPollTimeout
	}
	return err
}

// getMfaChallengeContextWithContext is getMfaChallengeContext with the verify request bound to ctx.
func getMfaChallengeContextWithContext(ctx context.Context, oc *Client, mfaOption int, resp string) (*mfaChallengeContext, error) {
	stateToken := gjson.Get(resp, "stateToken").String()
	factorID := gjson.Get(resp, fmt.Sprintf("_embedded.factors.%d.id", mfaOption)).String()
	oktaVerify := gjson.Get(resp, fmt.Sprintf("_embedded.factors.%d._links.verify.href", mfaOption)).String()
	mfaIdentifer, _, _ := parseMfaIdentifer(resp, mfaOption)

	if !strings.Contains(oktaVerify, "rememberDevice") {
		separator := "?"
		if strings.Contains(oktaVerify, "?") {
			separator = "&"
		}
		oktaVerify = oktaVerify + separator + "rememberDevice=" + strconv.FormatBool(oc.rememberDevice)
	} else if !oc.rememberDevice {
		oktaVerify = forceRememberDeviceOff(oktaVerify)
	}

	logger.WithField("factorID", factorID).WithField("oktaVerify", oktaVerify).WithField("mfaIdentifer", mfaIdentifer).Debug("MFA")

	if _, ok := supportedMfaOptions[mfaIdentifer]; !ok {
		return nil, errors.New("unsupported mfa provider")
	}

	// get signature & callback
	verifyReq := VerifyRequest{StateToken: stateToken, RememberDevice: strconv.FormatBool(oc.rememberDevice)}
	verifyBody := new(bytes.Buffer)

	// Login flow is different for YubiKeys ( of course )
	// https://developer.okta.com/docs/reference/api/factors/#request-example-for-verify-yubikey-factor
	// verifyBody needs to be a json document with the OTP from the yubikey in it.
	// yay
	switch mfa := mfaIdentifer; mfa {
	case IdentifierYubiMfa:
		verifyCode, promptErr := prompter.Password("Press the button on your yubikey")
		if promptErr != nil {
			return nil, errors.Wrap(promptErr, "error reading the yubikey code")
		}
		verifyReq.PassCode = verifyCode
	}

	err := json.NewEncoder(verifyBody).Encode(verifyReq)
	if err != nil {
		return nil, errors.Wrap(err, "error encoding verifyReq")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", oktaVerify, verifyBody)
	if err != nil {
		return nil, errors.Wrap(err, "error building verify request")
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")

	res, err := oc.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving verify response")
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving body from response")
	}

	return &mfaChallengeContext{
		factorID:              factorID,
		oktaVerify:            oktaVerify,
		mfaIdentifer:          mfaIdentifer,
		challengeResponseBody: string(body),
	}, nil
}

func verifyMfa(oc *Client, oktaOrgHost string, loginDetails *creds.LoginDetails, resp string) (string, error) {
	stateToken := gjson.Get(resp, "stateToken").String()

	// choose an mfa option if there are multiple enabled
	mfaOption := 0
	var mfaOptions []string
	for i := range gjson.Get(resp, "_embedded.factors").Array() {
		identifier, authName, id := parseMfaIdentifer(resp, i)
		if val, ok := supportedMfaOptions[identifier]; ok {
			// If the authentication method as a name, we add it to the MFA option.
			// This makes it possible to identify which method to choose
			if len(authName) > 0 {
				mfaOptions = append(mfaOptions, fmt.Sprintf("%s - %s (%s)", val, authName, id))
			} else {
				mfaOptions = append(mfaOptions, fmt.Sprintf("%s - %s", val, id))
			}
		} else {
			mfaOptions = append(mfaOptions, "UNSUPPORTED: "+identifier)
		}
	}

	if strings.ToUpper(oc.mfa) != "AUTO" {
		var mfaOptionsMatches []string
		// Collect all options that match the chosen MFA
		// It will be more than 1 when there's multiple MFA of the same type configured - e.g.: multiple FIDO methods
		for _, option := range mfaOptions {
			if strings.HasPrefix(strings.ToUpper(option), oc.mfa) {
				mfaOptionsMatches = append(mfaOptionsMatches, option)
			}
		}
		// If multiple MFA of the same type are found, we prompt the user to pick which one to use
		if len(mfaOptionsMatches) > 1 {
			matchOptionIndex, promptErr := prompter.Choose(fmt.Sprintf("Multiple %s MFA options found. Select which MFA option to use", oc.mfa), mfaOptionsMatches)
			if promptErr != nil {
				return "", errors.Wrap(promptErr, "error selecting the MFA option")
			}
			for i := range mfaOptions {
				if mfaOptions[i] == mfaOptionsMatches[matchOptionIndex] {
					mfaOption = i
				}
			}
		} else {
			mfaOption = findMfaOption(oc.mfa, mfaOptions, 0)
		}
	} else if len(mfaOptions) > 1 {
		var promptErr error
		if mfaOption, promptErr = prompter.Choose("Select which MFA option to use", mfaOptions); promptErr != nil {
			return "", errors.Wrap(promptErr, "error selecting the MFA option")
		}
	}

	challengeContext, err := getMfaChallengeContext(oc, mfaOption, resp)
	if err != nil {
		return "", err
	}

	switch mfa := challengeContext.mfaIdentifer; mfa {
	case IdentifierYubiMfa:
		return gjson.Get(challengeContext.challengeResponseBody, "sessionToken").String(), nil
	case IdentifierSmsMfa, IdentifierEmailMfa, IdentifierTotpMfa, IdentifierOktaTotpMfa, IdentifierSymantecTotpMfa:
		verifyCode := loginDetails.MFAToken
		if verifyCode == "" {
			var promptErr error
			if verifyCode, promptErr = prompter.RequestSecurityCode("000000"); promptErr != nil {
				return "", errors.Wrap(promptErr, "error reading the MFA code")
			}
		}
		tokenReq := VerifyRequest{StateToken: stateToken, PassCode: verifyCode, RememberDevice: strconv.FormatBool(oc.rememberDevice)}
		tokenBody := new(bytes.Buffer)
		err = json.NewEncoder(tokenBody).Encode(tokenReq)
		if err != nil {
			return "", errors.Wrap(err, "error encoding token data")
		}

		req, err := http.NewRequest("POST", challengeContext.oktaVerify, tokenBody)
		if err != nil {
			return "", errors.Wrap(err, "error building token post request")
		}

		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("Accept", "application/json")

		res, err := oc.client.Do(req)
		if err != nil {
			return "", errors.Wrap(err, "error retrieving token post response")
		}
		defer res.Body.Close()

		return extractSessionToken(res.Body)

	case IdentifierPushMfa:

		log.Println("Waiting for approval, please check your Okta Verify app ...")

		// loop until success, error, or timeout
		body := challengeContext.challengeResponseBody
		// Atmos fork: one deadline bounds both the waits and the verify requests of the polling loop.
		pushCtx, cancelPush := context.WithTimeout(context.Background(), mfaPollTimeout)
		defer cancelPush()
		for {
			// on 'success' status
			if gjson.Get(body, "status").String() == "SUCCESS" {
				log.Println(" Approved")
				logger.Debugf("func verifyMfa | okta exiry: %s", gjson.Get(body, "expiresAt").String()) // DEBUG
				return gjson.Get(body, "sessionToken").String(), nil
			}

			// otherwise probably still waiting
			switch gjson.Get(body, "factorResult").String() {

			case "WAITING":
				if err := sleepUntilDone(pushCtx, pushPollInterval); err != nil {
					return "", err
				}
				logger.Debug("Waiting for user to authorize login")
				updatedContext, err := getMfaChallengeContextWithContext(pushCtx, oc, mfaOption, resp)
				if err != nil {
					return "", pollError(pushCtx, err)
				}
				body = updatedContext.challengeResponseBody
				if gjson.Get(body, "status").String() == "MFA_CHALLENGE" {
					correctAnswer := gjson.Get(body, "_embedded.factor._embedded.challenge.correctAnswer").String()
					if correctAnswer != "" {
						log.Printf("Correct Answer: %s", correctAnswer)
					}
				}

			case "TIMEOUT":
				log.Println(" Timeout")
				return "", errors.New("User did not accept MFA in time")

			case "REJECTED":
				log.Println(" Rejected")
				return "", errors.New("MFA rejected by user")

			default:
				log.Println(" Error")
				return "", errors.New("Unsupported response from Okta, please raise ticket with saml2aws")

			}

		}

	case IdentifierDuoMfa:
		duoHost := gjson.Get(challengeContext.challengeResponseBody, "_embedded.factor._embedded.verification.host").String()
		duoSignature := gjson.Get(challengeContext.challengeResponseBody, "_embedded.factor._embedded.verification.signature").String()
		duoSignatures := strings.Split(duoSignature, ":")
		// Atmos fork: upstream indexed duoSignatures[1] unconditionally and panicked on a malformed signature.
		if len(duoSignatures) < 2 {
			return "", errors.New("invalid duo verification signature")
		}
		// duoSignatures[0] = TX
		// duoSignatures[1] = APP
		duoCallback := gjson.Get(challengeContext.challengeResponseBody, "_embedded.factor._embedded.verification._links.complete.href").String()

		// initiate duo mfa to get sid
		duoSubmitURL := fmt.Sprintf("https://%s/frame/web/v1/auth", duoHost)

		duoForm := url.Values{}
		duoForm.Add("parent", fmt.Sprintf("https://%s/signin/verify/duo/web", oktaOrgHost))
		duoForm.Add("java_version", "")
		duoForm.Add("flash_version", "")
		duoForm.Add("screen_resolution_width", "3008")
		duoForm.Add("screen_resolution_height", "1692")
		duoForm.Add("color_depth", "24")
		duoForm.Add("is_cef_browser", "false")
		duoForm.Add("is_ipad_os", "false")
		duoForm.Add("is_ie_compatability_mode", "") //nolint:misspell // Duo's actual (misspelled) form field name.
		duoForm.Add("acting_ie_version", "")
		duoForm.Add("react_support", "true")
		duoForm.Add("react_support_error_message", "")
		duoForm.Add("tx", duoSignatures[0])

		req, err := http.NewRequest("POST", duoSubmitURL, strings.NewReader(duoForm.Encode()))
		if err != nil {
			return "", errors.Wrap(err, "error building authentication request")
		}

		q := req.URL.Query()
		q.Add("tx", duoSignatures[0])
		q.Add("parent", fmt.Sprintf("https://%s/signin/verify/duo/web", oktaOrgHost))
		q.Add("v", "2.8")
		req.URL.RawQuery = q.Encode()

		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

		res, err := oc.client.Do(req)
		if err != nil {
			return "", errors.Wrap(err, "error retrieving verify response")
		}
		defer res.Body.Close()

		// At this point, if device trust is enabled we need to go on that tangent
		doc, err := goquery.NewDocumentFromReader(res.Body)
		if err != nil {
			return "", errors.Wrap(err, "error parsing document")
		}

		if doc.Find("form[id=\"client_cert_form\"]").Length() > 0 {
			doc, err = verifyTrustedCert(oc, doc, duoHost, duoSubmitURL, q)
			if err != nil {
				return "", errors.Wrap(err, "couldn't validate client cert")
			}

		} else if doc.Find("form[id=\"endpoint-health-form\"]").Length() > 0 {
			origUrl := req.URL.String()
			duoEndpointHost := "127.0.0.1:53100"
			doc, err = verifyEndpointHealth(oc, doc, origUrl, duoEndpointHost, duoHost, duoSubmitURL, q)
			if err != nil {
				return "", errors.Wrap(err, "couldn't validate endpoint health")
			}

		}

		duoSID, ok := doc.Find("input[name=\"sid\"]").Attr("value")
		if !ok {
			// Atmos fork: upstream used errors.Wrap(err, ...) with a nil err, which returned a nil error.
			return "", errors.New("unable to locate duo sid")
		}
		duoSID = html.UnescapeString(duoSID)

		duoMfaOptions := []string{}
		var token string

		webauthnOption := doc.Find("option[name=\"webauthn\"]")

		if webauthnOption.Length() > 0 {
			token, _ = webauthnOption.Attr("value")
			duoMfaOptions = append(duoMfaOptions, "U2F Key")
		}

		if doc.Find("option[value=\"phone1\"]").Length() > 0 {
			duoMfaOptions = append(duoMfaOptions, "Duo Push")
		}

		if doc.Find("option[value=\"token\"]").Length() > 0 ||
			doc.Find("option[value=\"phone1\"]").Length() > 0 {
			duoMfaOptions = append(duoMfaOptions, "Passcode")
		}

		duoMfaOption := -1

		if loginDetails.DuoMFAOption != "" {
			for i, v := range duoMfaOptions {
				if loginDetails.DuoMFAOption == v {
					duoMfaOption = i
				}
			}
		} else {
			var promptErr error
			if duoMfaOption, promptErr = prompter.Choose("Select a DUO MFA Option", duoMfaOptions); promptErr != nil {
				return "", errors.Wrap(promptErr, "error selecting the DUO MFA option")
			}
		}

		if duoMfaOption == -1 {
			return "", errors.Errorf("error unsupported MFA option '%s'", loginDetails.DuoMFAOption)
		}

		if duoMfaOptions[duoMfaOption] == "Passcode" {
			// get users DUO MFA Token
			var promptErr error
			if token, promptErr = prompter.StringRequired("Enter passcode"); promptErr != nil {
				return "", errors.Wrap(promptErr, "error reading the DUO passcode")
			}
		}

		// send mfa auth request
		duoSubmitURL = fmt.Sprintf("https://%s/frame/prompt", duoHost)

		duoForm = url.Values{}
		duoForm.Add("sid", duoSID)
		duoForm.Add("out_of_date", "false")

		switch duoMfaOptions[duoMfaOption] {
		case "Passcode":
			duoForm.Add("passcode", token)
			duoForm.Add("device", "phone1")
			duoForm.Add("factor", "Passcode")
		case "Duo Push":
			duoForm.Add("device", "phone1")
			duoForm.Add("factor", "Duo Push")
		case "U2F Key":
			duoForm.Add("device", "u2f_token")
			duoForm.Add("factor", "U2F Token")
		}

		req, err = http.NewRequest("POST", duoSubmitURL, strings.NewReader(duoForm.Encode()))
		if err != nil {
			return "", errors.Wrap(err, "error building authentication request")
		}

		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

		res, err = oc.client.Do(req)
		if err != nil {
			return "", errors.Wrap(err, "error retrieving verify response")
		}
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		if err != nil {
			return "", errors.Wrap(err, "error retrieving body from response")
		}

		resp = string(body)

		duoTxStat := gjson.Get(resp, "stat").String()
		duoTxID := gjson.Get(resp, "response.txid").String()
		if duoTxStat != "OK" {
			return "", errors.New("error authenticating mfa device")
		}

		// get duo cookie
		duoSubmitURL = fmt.Sprintf("https://%s/frame/status", duoHost)

		duoForm = url.Values{}
		duoForm.Add("sid", duoSID)
		duoForm.Add("txid", duoTxID)

		req, err = http.NewRequest("POST", duoSubmitURL, strings.NewReader(duoForm.Encode()))
		if err != nil {
			return "", errors.Wrap(err, "error building authentication request")
		}

		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

		res, err = oc.client.Do(req)
		if err != nil {
			return "", errors.Wrap(err, "error retrieving verify response")
		}
		defer res.Body.Close()

		body, err = io.ReadAll(res.Body)
		if err != nil {
			return "", errors.Wrap(err, "error retrieving body from response")
		}

		resp = string(body)

		duoTxResult := gjson.Get(resp, "response.result").String()
		duoResultURL := gjson.Get(resp, "response.result_url").String()
		newSID := gjson.Get(resp, "response.sid").String()
		if newSID != "" {
			duoSID = newSID
		}

		// Do the webauthn
		duoRestStatusCode := gjson.Get(resp, "response.status_code").String()

		if duoRestStatusCode == "u2f_sent" {
			appId := gjson.Get(resp, "response.u2f_sign_request.0.appId").String()
			// appId := "api-23a9854b.duosecurity.com"
			version := gjson.Get(resp, "response.u2f_sign_request.0.version").String()
			challengeNonce := gjson.Get(resp, "response.u2f_sign_request.0.challenge").String()
			keyHandle := gjson.Get(resp, "response.u2f_sign_request.0.keyHandle").String()
			sessionId := gjson.Get(resp, "response.u2f_sign_request.0.sessionId").String()

			u2fClient, err := NewDUOU2FClient(challengeNonce, appId, version, keyHandle, sessionId, newU2FDeviceFinder())
			if err != nil {
				return "", err
			}

			rd, err := u2fClient.ChallengeU2F()
			if err != nil {
				return "", err
			}

			payload, err := json.Marshal(rd)
			if err != nil {
				return "", err
			}

			duoForm = url.Values{}
			duoForm.Add("sid", duoSID)
			duoForm.Add("device", "u2f_token")
			duoForm.Add("factor", "u2f_finish")
			duoForm.Add("days_to_block", "None")
			duoForm.Add("out_of_date", "False")
			duoForm.Add("days_out_of_date", "0")
			duoForm.Add("response_data", string(payload))

			duoSubmitURL = fmt.Sprintf("https://%s/frame/prompt", duoHost)

			req, err = http.NewRequest("POST", duoSubmitURL, strings.NewReader(duoForm.Encode()))
			if err != nil {
				return "", errors.Wrap(err, "error building authentication request")
			}

			req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

			res, err = oc.client.Do(req)
			if err != nil {
				return "", errors.Wrap(err, "error retrieving verify response")
			}
			defer res.Body.Close()

			body, err = io.ReadAll(res.Body)
			if err != nil {
				return "", errors.Wrap(err, "error retrieving body from response")
			}

			resp = string(body)

			duoTxStat := gjson.Get(resp, "stat").String()
			duoTxID := gjson.Get(resp, "response.txid").String()
			if duoTxStat != "OK" {
				return "", errors.New("error authenticating mfa device")
			}

			// get duo cookie
			duoSubmitURL = fmt.Sprintf("https://%s/frame/status", duoHost)

			duoForm = url.Values{}
			duoForm.Add("sid", duoSID)
			duoForm.Add("txid", duoTxID)

			req, err = http.NewRequest("POST", duoSubmitURL, strings.NewReader(duoForm.Encode()))
			if err != nil {
				return "", errors.Wrap(err, "error building authentication request")
			}

			req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

			res, err = oc.client.Do(req)
			if err != nil {
				return "", errors.Wrap(err, "error retrieving verify response")
			}

			defer res.Body.Close()

			body, err = io.ReadAll(res.Body)
			if err != nil {
				return "", errors.Wrap(err, "error retrieving body from response")
			}

			resp = string(body)

			duoTxResult = gjson.Get(resp, "response.result").String()
			duoResultURL = gjson.Get(resp, "response.result_url").String()
			newSID = gjson.Get(resp, "response.sid").String()
			if newSID != "" {
				duoSID = newSID
			}

		}

		// Atmos fork: a terminal failure in the first status response ends the flow; upstream slept and polled again.
		if duoTxResult == "FAILURE" {
			return "", errors.New("failed to authenticate device")
		}

		if duoTxResult != "SUCCESS" {
			// poll as this is likely a push request
			// Atmos fork: one deadline bounds both the waits and the status requests of the polling loop.
			duoCtx, cancelDuo := context.WithTimeout(context.Background(), mfaPollTimeout)
			defer cancelDuo()
			for {
				if err = sleepUntilDone(duoCtx, duoPollInterval); err != nil {
					return "", err
				}

				// Atmos fork: send the latest sid; upstream kept posting the sid of the first status request.
				duoForm.Set("sid", duoSID)

				req, err = http.NewRequestWithContext(duoCtx, "POST", duoSubmitURL, strings.NewReader(duoForm.Encode()))
				if err != nil {
					return "", errors.Wrap(err, "error building authentication request")
				}

				req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

				res, err = oc.client.Do(req)
				if err != nil {
					return "", pollError(duoCtx, errors.Wrap(err, "error retrieving verify response"))
				}

				body, err = io.ReadAll(res.Body)
				// Close inside the polling loop so bodies are not held until the function returns.
				_ = res.Body.Close()
				if err != nil {
					return "", pollError(duoCtx, errors.Wrap(err, "error retrieving body from response"))
				}

				resp := string(body)

				duoTxResult = gjson.Get(resp, "response.result").String()
				duoResultURL = gjson.Get(resp, "response.result_url").String()
				newSID = gjson.Get(resp, "response.sid").String()
				if newSID != "" {
					duoSID = newSID
				}

				log.Println(gjson.Get(resp, "response.status").String())

				if duoTxResult == "FAILURE" {
					// Atmos fork: upstream used errors.Wrap(err, ...) with a nil err, which returned a nil error.
					return "", errors.New("failed to authenticate device")
				}

				if duoTxResult == "SUCCESS" {
					break
				}
			}
		}

		duoRequestURL := fmt.Sprintf("https://%s%s", duoHost, duoResultURL)

		duoForm = url.Values{}
		duoForm.Add("sid", duoSID)

		req, err = http.NewRequest("POST", duoRequestURL, strings.NewReader(duoForm.Encode()))
		if err != nil {
			return "", errors.Wrap(err, "error constructing request object to result url")
		}

		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

		res, err = oc.client.Do(req)
		if err != nil {
			return "", errors.Wrap(err, "error retrieving duo result response")
		}
		defer res.Body.Close()

		body, err = io.ReadAll(res.Body)
		if err != nil {
			return "", errors.Wrap(err, "duoResultSubmit: error retrieving body from response")
		}

		resp = string(body)

		duoTxStat = gjson.Get(resp, "stat").String()
		if duoTxStat != "OK" {
			message := gjson.Get(resp, "message").String()
			return "", fmt.Errorf("duoResultSubmit: %s %s", duoTxStat, message)
		}

		duoTxCookie := gjson.Get(resp, "response.cookie").String()
		if duoTxCookie == "" {
			return "", errors.New("duoResultSubmit: Unable to get response.cookie")
		}

		// callback to okta with cookie
		oktaForm := url.Values{}
		oktaForm.Add("id", challengeContext.factorID)
		oktaForm.Add("stateToken", stateToken)
		oktaForm.Add("sig_response", fmt.Sprintf("%s:%s", duoTxCookie, duoSignatures[1]))

		req, err = http.NewRequest("POST", duoCallback, strings.NewReader(oktaForm.Encode()))
		if err != nil {
			return "", errors.Wrap(err, "error building authentication request")
		}

		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

		// Atmos fork: the callback response was never closed, which prevents connection reuse.
		callbackRes, err := oc.client.Do(req) // TODO: check result
		if err != nil {
			return "", errors.Wrap(err, "error retrieving verify response")
		}
		_, _ = io.Copy(io.Discard, callbackRes.Body)
		_ = callbackRes.Body.Close()

		// extract okta session token

		verifyReq := VerifyRequest{StateToken: stateToken, RememberDevice: strconv.FormatBool(oc.rememberDevice)}
		verifyBody := new(bytes.Buffer)
		err = json.NewEncoder(verifyBody).Encode(verifyReq)
		if err != nil {
			return "", errors.Wrap(err, "error encoding verify request")
		}

		req, err = http.NewRequest("POST", challengeContext.oktaVerify, verifyBody)
		if err != nil {
			return "", errors.Wrap(err, "error building verify request")
		}

		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("Accept", "application/json")
		req.Header.Add("X-Okta-XsrfToken", "")

		res, err = oc.client.Do(req)
		if err != nil {
			return "", errors.Wrap(err, "error retrieving verify response")
		}
		defer res.Body.Close()

		body, err = io.ReadAll(res.Body)
		if err != nil {
			return "", errors.Wrap(err, "error retrieving body from response")
		}

		return gjson.GetBytes(body, "sessionToken").String(), nil

	case IdentifierFIDOWebAuthn:
		return fidoWebAuthn(oc, oktaOrgHost, challengeContext, mfaOption, stateToken, mfaOptions, resp)
	}

	// catch all
	return "", errors.New("no mfa options provided")
}

func extractSessionToken(r io.Reader) (string, error) {
	bb, err := io.ReadAll(r)
	if err != nil {
		return "", errors.Wrap(err, "error retrieving body from response")
	}

	resp := string(bb)
	sessionToken := gjson.Get(resp, "sessionToken").String()
	if sessionToken == "" {
		status := gjson.Get(resp, "status").String()
		if status != "" {
			return "", errors.Errorf("response does not contain session token, received status is: %q", status)
		}
		return "", errors.Errorf("response does not contain session token")
	}

	return gjson.Get(resp, "sessionToken").String(), nil
}

func fidoWebAuthn(oc *Client, oktaOrgHost string, challengeContext *mfaChallengeContext, mfaOption int, stateToken string, mfaOptions []string, resp string) (string, error) {
	var signedAssertion *SignedAssertion
	challengeResponseBody := challengeContext.challengeResponseBody
	tried := map[int]bool{mfaOption: true}

	for {
		nonce := gjson.Get(challengeResponseBody, "_embedded.factor._embedded.challenge.challenge").String()
		credentialID := gjson.Get(challengeResponseBody, "_embedded.factor.profile.credentialId").String()
		version := gjson.Get(challengeResponseBody, "_embedded.factor.profile.version").String()

		fidoClient, err := NewFidoClient(
			nonce,
			oktaOrgHost,
			version,
			credentialID,
			stateToken,
			newU2FDeviceFinder(),
		)
		if err != nil {
			// Try to authenticate with the system level Webauthn libraries
			signedAssertion, err = ChallengeSystemWebAuthn(nonce, oktaOrgHost, stateToken)
			if err == nil {
				break
			}
			return "", err
		}

		signedAssertion, err = fidoClient.ChallengeU2F()
		if err != nil {
			// if this error is not a bad key error we are done
			var badKeyErr *u2fhost.BadKeyHandleError
			if !errors.As(err, &badKeyErr) {
				return "", errors.Wrap(err, "failed to perform U2F challenge")
			}

			// check if there is another fido device and try that
			// Atmos fork: upstream passed the failed option to findMfaOption, which is inclusive of its
			// start index, so it always returned the failed option again and the "try the next FIDO
			// device" fallback never ran. Every other matching option is now tried once, including
			// options listed before the selected one, and the failed options are never tried again.
			// Atmos fork: with the "Auto" setting no option label starts with "AUTO", so the search
			// never found another FIDO option. Search for the FIDO label itself in that case.
			fidoPrefix := oc.mfa
			if strings.EqualFold(oc.mfa, "AUTO") {
				fidoPrefix = strings.ToUpper(supportedMfaOptions[IdentifierFIDOWebAuthn])
			}
			nextMfaOption, found := nextUntriedMfaOption(fidoPrefix, mfaOptions, tried)
			if !found {
				return "", errors.Wrap(err, "tried all MFA options")
			}
			tried[nextMfaOption] = true

			nextChallengeContext, err := getMfaChallengeContext(oc, nextMfaOption, resp)
			if err != nil {
				return "", errors.Wrap(err, "get mfa challenge failed for U2F device")
			}
			challengeResponseBody = nextChallengeContext.challengeResponseBody
			continue
		}

		break
	}

	payload, err := json.Marshal(signedAssertion)
	if err != nil {
		return "", err
	}

	webauthnCallback := gjson.Get(challengeResponseBody, "_links.next.href").String()
	req, err := http.NewRequest("POST", webauthnCallback, strings.NewReader(string(payload)))
	if err != nil {
		return "", errors.Wrap(err, "error building authentication request")
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	res, err := oc.client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "error retrieving verify response")
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", errors.Wrap(err, "error retrieving body from response")
	}

	return gjson.GetBytes(body, "sessionToken").String(), nil
}

// certifierTLSConfig returns the TLS configuration for the Duo trusted-endpoint certifier request.
// Atmos fork: upstream skipped certificate verification for whatever certifier_url the Duo page
// named and left MinVersion unset. Only the locally running certifier has a self-signed
// certificate, so verification is skipped only for a loopback host.
func certifierTLSConfig(certifierURL string) *tls.Config {
	return &tls.Config{
		MinVersion:         provider.MinTLSVersion,
		InsecureSkipVerify: isLoopbackURL(certifierURL), // #nosec G402 -- Loopback-only: the local Duo certifier uses a self-signed certificate.
	}
}

// isLoopbackURL reports whether rawURL points at localhost or a loopback IP address.
func isLoopbackURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// errCertifierNonLoopbackDial is returned when the unverified certifier connection resolves to a non-loopback address.
var errCertifierNonLoopbackDial = errors.New("certifier connection resolved to a non-loopback address")

// requireLoopbackDial is a net.Dialer Control function that rejects any non-loopback destination address.
func requireLoopbackDial(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errors.Wrap(err, "invalid certifier dial address")
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return errCertifierNonLoopbackDial
	}
	return nil
}

// errCertifierRedirect is returned when the certifier redirects somewhere other than a loopback address.
var errCertifierRedirect = errors.New("certifier redirected to a non-loopback URL")

// maxCertifierRedirects bounds the redirects followed by the certifier request.
const maxCertifierRedirects = 10

// newCertifierClient returns a dedicated client for the Duo certifier request.
// Atmos fork: upstream (and the first Atmos version) swapped the shared client's transport for one
// that skips certificate verification, which also applied to every redirect hop. The dedicated client
// leaves the shared client untouched, shares its cookie jar and status check, and refuses to follow a
// redirect to a non-loopback URL, so an unverified connection can never reach a remote host.
func newCertifierClient(base *provider.HTTPClient, certifierURL string) *provider.HTTPClient {
	certTransport := http.DefaultTransport.(*http.Transport).Clone()
	certTransport.TLSClientConfig = certifierTLSConfig(certifierURL)
	// Atmos fork: the unverified connection must never go through a proxy and must only reach a loopback
	// address. A proxy from the environment is bypassed by Go only for lowercase "localhost" or a loopback IP,
	// so a mixed-case loopback name could otherwise be sent to the proxy with verification disabled.
	// A non-loopback certifier keeps full certificate verification and the default proxy handling.
	if isLoopbackURL(certifierURL) {
		certTransport.Proxy = nil
		certTransport.DialContext = (&net.Dialer{Control: requireLoopbackDial}).DialContext
	}

	client := *base
	client.Transport = certTransport
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxCertifierRedirects {
			return errors.New("stopped after too many certifier redirects")
		}
		if !isLoopbackURL(req.URL.String()) {
			return errCertifierRedirect
		}
		return nil
	}
	return &client
}

func verifyTrustedCert(oc *Client, doc *goquery.Document, duoHost string, duoSubmitURL string, q url.Values) (*goquery.Document, error) {
	// If you enable DUO trusted cert validation, it requires an extra step before continuing.
	// The way the validation process works is it attempts to send a request to a localhost:15310
	// where the DUO cert proxy may be running.  This returns a JSON blob if it was successful.
	// If that isn't running, there is also a public DUO endpoint you can use for the validation.
	// This code attempts to hit the local validator, then the remote one if it is not available,
	// which is the same flow the webpage does if you validate through a browser.

	// We then follow up again with a POST request to /frame/web/v1/auth, this time with the
	// cert validation parameters in the POST body.  If that succeeds, then we can continue
	// along the existing request path.

	sid, _ := doc.Find("input[name=\"sid\"]").Attr("value")
	certUrl, _ := doc.Find("input[name=\"certs_url\"]").Attr("value")
	txid, _ := doc.Find("input[name=\"certs_txid\"]").Attr("value")
	certifierUrl, _ := doc.Find("input[name=\"certifier_url\"]").Attr("value")

	duoUrl := fmt.Sprintf("%s?type=AJAX&sid=%s&certs_txid=%s", certUrl, url.QueryEscape(sid), txid)
	duoCertifierURL := fmt.Sprintf("%s?certUrl=%s", certifierUrl, url.QueryEscape(duoUrl))

	req, err := http.NewRequest("GET", duoCertifierURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error building cert validation request")
	}

	req.Header.Add("Referer", "https://"+duoHost+"/")
	res, err := newCertifierClient(oc.client, certifierUrl).Do(req)
	if err != nil {
		// Local certifier not running, try online one
		duoCertURL := fmt.Sprintf("%s?sid=%s&certs_txid=%s&type=AJAX", certUrl, url.QueryEscape(sid), txid)

		req, err = http.NewRequest("GET", duoCertURL, nil)
		if err != nil {
			return nil, errors.Wrap(err, "error building cert validation request ")
		}

		req.Header.Add("Referer", "https://"+duoHost)
		res, err = oc.client.Do(req)
		if err != nil {
			return nil, errors.Wrap(err, "error retrieving cert validation response")
		}
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving body from response")
	}

	resp := string(body)

	duoStat := gjson.Get(resp, "stat").String()
	if duoStat != "OK" {
		return nil, errors.New("error validation certificate")
	}

	certForm := url.Values{}
	certForm.Add("sid", sid)
	certForm.Add("certs_url", certUrl)
	certForm.Add("certs_txid", txid)
	certForm.Add("certifier_url", certifierUrl)

	// Try POST again
	req, err = http.NewRequest("POST", duoSubmitURL, strings.NewReader(certForm.Encode()))
	if err != nil {
		return nil, errors.Wrap(err, "error building authentication request")
	}

	req.URL.RawQuery = q.Encode()

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err = oc.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving verify response")
	}
	defer res.Body.Close()

	doc, err = goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing document")
	}

	return doc, nil
}

func verifyEndpointHealth(oc *Client, doc *goquery.Document, origURL string, duoEndpointHost string, duoHost string, duoSubmitURL string, postQuery url.Values) (*goquery.Document, error) {
	txid, _ := doc.Find("input[name=\"txid\"]").Attr("value")
	sid, _ := doc.Find("input[name=\"sid\"]").Attr("value")
	ehServiceUrl, _ := doc.Find("input[name=\"eh_service_url\"]").Attr("value")
	akey, _ := doc.Find("input[name=\"akey\"]").Attr("value")
	responseTimeout, _ := doc.Find("input[name=\"response_timeout\"]").Attr("value")
	parent, _ := doc.Find("input[name=\"parent\"]").Attr("value")
	duoAppUrl, _ := doc.Find("input[name=\"duo_app_url\"]").Attr("value")
	ehDownloadLink, _ := doc.Find("input[name=\"eh_download_link\"]").Attr("value")
	// isSilentCollection, _ := doc.Find("input[name=\"is_silent_collection\"]").Attr("value")

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	duoAliveUrl := fmt.Sprintf("https://%s/alive", duoEndpointHost)
	req, _ := http.NewRequest("GET", duoAliveUrl, nil)

	q := req.URL.Query()
	q.Add("_", timestamp+"100")

	req.URL.RawQuery = q.Encode()
	req.Header.Add("Referer", "https://"+duoHost+"/")
	req.Header.Add("Origin", "https://"+duoHost)

	aliveRes, err := oc.client.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to query alive URL %s", duoAliveUrl)
	}
	_ = aliveRes.Body.Close()

	duoCheckEndpointAppURL := fmt.Sprintf("https://%s/frame/check_endpoint_app_status", duoHost)
	req, _ = http.NewRequest("GET", duoCheckEndpointAppURL, nil)

	q = req.URL.Query()

	q.Add("txid", txid)
	q.Add("sid", sid)

	req.URL.RawQuery = q.Encode()

	req.Header.Add("Referer", origURL)
	req.Header.Add("X-Requested-With", "XMLHttpRequest")

	// Atmos fork: the status request blocks until the report is queried, so it gets a cancellable, bounded
	// context. If the report fails, the status request is cancelled instead of blocking forever.
	ctx, cancel := context.WithTimeout(context.Background(), endpointHealthTimeout)
	defer cancel()
	req = req.WithContext(ctx)

	var wg sync.WaitGroup
	wg.Add(1)

	// statusErr records why the status request failed. It is written by the goroutine below and read
	// only after wg.Wait.
	var statusErr error

	// check_endpoint_app_status blocks until the healthurl report is queried, so we need to use goroutines.
	go func(r *http.Request) {
		defer wg.Done()
		// Atmos fork: upstream deferred res.Body.Close() on a possibly nil response and panicked on transport errors.
		res, err := oc.client.Do(r)
		if err != nil {
			statusErr = err
			return
		}
		_ = res.Body.Close()
		if res.StatusCode >= http.StatusBadRequest {
			statusErr = errors.Errorf("unexpected status %s", res.Status)
		}
	}(req)

	// Separator

	duoHealthURL := fmt.Sprintf("https://%s/report", duoEndpointHost)

	req2, _ := http.NewRequest("GET", duoHealthURL, nil)

	q = req2.URL.Query()

	q.Add("txid", txid)
	q.Add("eh_service_url", ehServiceUrl+"?_="+timestamp+"101")

	req2.URL.RawQuery = q.Encode()
	req2.Header.Add("Referer", "https://"+duoHost+"/")
	req2.Header.Add("Origin", "https://"+duoHost)

	req2 = req2.WithContext(ctx)

	res, err := oc.client.Do(req2)
	if err != nil {
		cancel()
		wg.Wait()
		return nil, errors.Wrapf(err, "failed to query report URL %s", duoHealthURL)
	}
	defer res.Body.Close()

	// Wait for check_endpoint_app_status to block
	wg.Wait()
	// Atmos fork: upstream discarded the status result. Duo does not document whether the resubmit needs
	// a successful status check, so the flow still resubmits as before, and the status failure is kept
	// to explain a failed resubmit.
	if statusErr != nil {
		logger.WithError(statusErr).Debug("Duo endpoint status check failed")
	}

	// Try the call to /v1/frame/auth again

	certForm := url.Values{}
	certForm.Add("sid", sid)
	certForm.Add("txid", txid)
	certForm.Add("eh_service_url", ehServiceUrl)
	certForm.Add("akey", akey)
	certForm.Add("response_timeout", responseTimeout)
	certForm.Add("parent", parent)
	certForm.Add("duo_app_url", duoAppUrl)
	certForm.Add("eh_download_link", ehDownloadLink)
	// certForm.Add("is_silent_collection", isSilentCollection)

	time.Sleep(endpointHealthSettleDelay)

	// Try POST again
	req, err = http.NewRequest("POST", duoSubmitURL, strings.NewReader(certForm.Encode()))
	if err != nil {
		return nil, errors.Wrap(err, "error building authentication request")
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	req.URL.RawQuery = postQuery.Encode()

	res, err = oc.client.Do(req)
	if err != nil {
		if statusErr != nil {
			return nil, errors.Wrapf(err, "error retrieving verify response (endpoint status check failed: %v)", statusErr)
		}
		return nil, errors.Wrap(err, "error retrieving verify response")
	}
	defer res.Body.Close()

	doc, err = goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing document")
	}
	return doc, nil
}
