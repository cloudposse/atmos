package googleapps

import (
	"bufio"
	"bytes"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/cfg"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/prompter"
	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/provider"
)

var logger = logrus.WithField("provider", "googleapps")

// errInputValueNotFound is returned when a form input exists but has no value attribute.
var errInputValueNotFound = errors.New("unable to locate field value")

// errCaptchaImageNotFound is returned when a captcha is requested but its image cannot be found.
var errCaptchaImageNotFound = errors.New("captcha image not found but requested")

// errCaptchaAttemptsExceeded is returned when Google keeps asking for a captcha.
var errCaptchaAttemptsExceeded = errors.New("captcha still requested after the maximum number of attempts")

// errMFATransitionsExceeded is returned when Google keeps sending the user back to a challenge selection page.
var errMFATransitionsExceeded = errors.New("2-step verification did not complete after the maximum number of challenge transitions")

// errPhoneApprovalFailed is returned when waiting for the phone approval does not succeed.
var errPhoneApprovalFailed = errors.New("phone approval failed")

// maxMFATransitions bounds how many challenge selection pages one authentication attempt follows.
const maxMFATransitions = 10

// maxCaptchaAttempts bounds how many captcha answers are submitted before giving up.
const maxCaptchaAttempts = 5

// captchaInputIDs are the element ids that mark a page asking for a captcha.
var captchaInputIDs = []string{
	"logincaptcha",
	"identifier-captcha-input",
	"captchaimg",
}

// findCaptcha returns the captcha element on the page and the name of the form field to answer it in.
// It returns an empty selection when the page does not ask for a captcha.
func findCaptcha(doc *goquery.Document) (*goquery.Selection, string) {
	for _, id := range captchaInputIDs {
		if found := doc.Find("#" + id); found.Length() > 0 {
			return found, id
		}
	}
	// Atmos fork: the new-style captcha input has `name="ca"` and no id.
	if found := doc.Find("input[name='ca']"); found.Length() > 0 {
		return found, "ca"
	}
	return doc.Find("#" + captchaInputIDs[0]), ""
}

// Atmos fork: the U2F device finder is created through a package variable so tests can
// inject a mock device instead of requiring attached hardware.
var newU2FDeviceFinder = func() DeviceFinder { return &U2FDeviceFinder{} }

// Client wrapper around Google Apps.
type Client struct {
	provider.ValidateBase

	client *provider.HTTPClient

	// mfaTransitions counts the challenge selection pages followed during the current authentication attempt.
	// Atmos fork: upstream followed them recursively without a limit.
	mfaTransitions int
}

// New create a new Google Apps Client
func New(idpAccount *cfg.IDPAccount) (*Client, error) {
	tr := provider.NewDefaultTransport(idpAccount.SkipVerify)

	client, err := provider.NewHTTPClient(tr, provider.BuildHttpClientOpts(idpAccount))
	if err != nil {
		return nil, errors.Wrap(err, "error building http client")
	}

	return &Client{
		client: client,
	}, nil
}

// Authenticate logs into Google Apps and returns a SAML response
func (kc *Client) Authenticate(loginDetails *creds.LoginDetails) (string, error) {
	kc.mfaTransitions = 0

	// Get the first page
	authURL, authForm, err := kc.loadFirstPage(loginDetails)
	if err != nil {
		return "", errors.Wrap(err, "error loading first page")
	}

	// Google supports only JavaScript-enabled clients
	authForm.Set("bgresponse", "js_enabled")

	authForm.Set("Email", loginDetails.Username)

	// Post email address w/o password, then Get the password-input page
	passwordURL, passwordForm, err := kc.loadLoginPage(withLocale(authURL), withLocale(loginDetails.URL), authForm)
	if err != nil {
		// if failed, try with "identifier"
		authForm.Set("Email", "") // Clear previous key
		authForm.Set("identifier", loginDetails.Username)
		passwordURL, passwordForm, err = kc.loadLoginPage(withLocale(authURL), withLocale(loginDetails.URL), authForm)
		if err != nil {
			return "", errors.Wrap(err, "error loading login page")
		}
	}

	logger.Debugf("loginURL: %s", provider.RedactRawURL(passwordURL))

	passwordForm.Set("Passwd", loginDetails.Password)
	passwordForm.Set("TrustDevice", "on")

	referingURL := passwordURL

	responseDoc, err := kc.loadChallengePage(withLocale(passwordURL), referingURL, passwordForm, loginDetails)
	if err != nil {
		return "", errors.Wrap(err, "error loading challenge page")
	}

	// Atmos fork: the captcha handling is shared by every response in this flow, with one attempt budget.
	captchaAttempts := 0
	responseDoc, err = kc.solveCaptchas(responseDoc, loginDetails, &captchaAttempts)
	if err != nil {
		return "", err
	}

	// New Captcha proceeds back to password page
	passworddBeingRequested := responseDoc.Find("#password")
	if passworddBeingRequested != nil && passworddBeingRequested.Length() > 0 {
		loginForm, loginURL, err := extractInputsByFormID(responseDoc, "challenge")
		if err != nil {
			return "", errors.Wrap(err, "error parsing password page after captcha")
		}

		loginForm.Set("Passwd", loginDetails.Password)

		responseDoc, err = kc.loadChallengePage(withLocale(loginURL), loginURL, loginForm, loginDetails)
		if err != nil {
			return "", errors.Wrap(err, "error loading challenge page")
		}

		// Atmos fork: the resubmitted password can be answered with a captcha as well.
		responseDoc, err = kc.solveCaptchas(responseDoc, loginDetails, &captchaAttempts)
		if err != nil {
			return "", err
		}
	}

	samlAssertion, err := findInputByName(responseDoc, "SAMLResponse")
	if err != nil {
		return "", err
	}
	if samlAssertion == "" {
		if responseDoc.Selection.Find("#passwordError").Text() != "" {
			return "", errors.New("Password error")
		}

		if err := isMissing2StepSetup(responseDoc); err != nil {
			return "", err
		}

		return "", errors.New("page is missing saml assertion")
	}

	return samlAssertion, nil
}

// solveCaptchas answers captcha pages until the response no longer asks for one. The attempts counter is
// shared by all callers in one authentication, so the total number of answers stays bounded.
func (kc *Client) solveCaptchas(responseDoc *goquery.Document, loginDetails *creds.LoginDetails, attempts *int) (*goquery.Document, error) {
	captchaFound, captchaInputId := findCaptcha(responseDoc)

	for captchaFound != nil && captchaFound.Length() > 0 {
		if *attempts >= maxCaptchaAttempts {
			return nil, errCaptchaAttemptsExceeded
		}
		*attempts++
		// Atmos fork: upstream checked `captchaImgDiv != nil`, which is always true for a
		// *goquery.Selection, so the legacy .captcha-img markup was never used, and it indexed
		// Children().Nodes[0] without a length check, which panicked when the container was
		// missing or empty. The new-style container is now only a fallback, and a missing image
		// returns an error.
		captchaImgDiv := responseDoc.Find(".captcha-img")
		if captchaImgDiv.Length() == 0 {
			captchaImgDiv = responseDoc.Find("div[data-auto-init='CaptchaInput']")
			captchaInputId = "ca"
		}
		captchaImg := captchaImgDiv.Children().First()
		if captchaImg.Length() == 0 {
			return nil, errCaptchaImageNotFound
		}
		captchaPictureSrc, found := captchaImg.Attr("src")

		if !found {
			return nil, errCaptchaImageNotFound
		}

		// Atmos fork: the image resolves against the page that contains it, which can be on another host
		// after a redirect, not against the original password page.
		pageURL := ""
		if responseDoc.Url != nil {
			pageURL = responseDoc.Url.String()
		}
		captchaPictureURL, err := generateFullURLIfRelative(captchaPictureSrc, pageURL)
		if err != nil {
			return nil, errors.Wrap(err, "error generating captcha image URL")
		}

		captcha, err := kc.tryDisplayCaptcha(captchaPictureURL)
		if err != nil {
			return nil, err
		}

		captchaForm, captchaURL, err := extractInputsByFormID(responseDoc, "gaia_loginform", "challenge")
		if err != nil {
			return nil, errors.Wrap(err, "error extracting captcha")
		}

		logger.Debugf("captchaURL: %s", provider.RedactRawURL(captchaURL))

		_, captchaV1 := captchaForm["Passwd"]
		if captchaV1 {
			captchaForm.Set("Passwd", loginDetails.Password)
		}
		captchaForm.Set(captchaInputId, captcha)

		responseDoc, err = kc.loadChallengePage(withLocale(captchaURL), captchaURL, captchaForm, loginDetails)
		if err != nil {
			return nil, errors.Wrap(err, "error loading challenge page")
		}

		// Atmos fork: upstream re-checked only `#<input id>`, but the new-style captcha input
		// is `name="ca"` with no id, so a rejected answer was never retried and the flow failed
		// looking for a SAMLResponse on the captcha page. Re-run the same detection instead.
		captchaFound, captchaInputId = findCaptcha(responseDoc)
	}

	return responseDoc, nil
}

func isMissing2StepSetup(responseDoc *goquery.Document) error {
	if responseDoc.Selection.Find("section.aN1Vld ").Text() != "" {
		return errors.New("Because of your organization settings, you must set-up 2-Step Verification in your account")
	}
	return nil
}

func (kc *Client) tryDisplayCaptcha(captchaPictureURL string) (string, error) {
	// TODO: check for user flag for easy captcha presentation

	if os.Getenv("TERM_PROGRAM") == "iTerm.app" {
		// Use iTerm to show the image if available
		return kc.iTermCaptchaPrompt(captchaPictureURL)
	} else {
		return simpleCaptchaPrompt(captchaPictureURL)
	}
}

func (kc *Client) iTermCaptchaPrompt(captchaPictureURL string) (string, error) {
	// Atmos fork: the image URL carries a challenge token, so it is not logged; the image is shown inline.
	log.Println("Detected iTerm, displaying the captcha image inline.")
	imgResp, err := kc.client.Get(captchaPictureURL)
	if err != nil {
		return "", errors.Wrap(err, "unable to fetch captcha image")
	}
	defer imgResp.Body.Close()
	var buf bytes.Buffer
	b64Encoder := b64.NewEncoder(b64.StdEncoding, &buf)
	// Atmos fork: an interrupted download is an error; a partial image is not shown to the user.
	if _, err := io.Copy(b64Encoder, imgResp.Body); err != nil {
		return "", errors.Wrap(err, "unable to read captcha image")
	}
	if err := b64Encoder.Close(); err != nil {
		return "", errors.Wrap(err, "unable to encode captcha image")
	}

	// Atmos fork: upstream printed the inline image to stdout, which is the caller's data
	// channel (for example `eval $(atmos auth env)`). It is written to stderr, where the
	// prompts are shown and which is still the terminal.
	if os.Getenv("TERM") == "screen" {
		log.Println("Detected tmux, using specific workaround...")
		fmt.Fprintf(os.Stderr, "\033Ptmux;\033\033]1337;File=width=40;preserveAspectRatio=1;inline=1;:%s\a\033\\\n", buf.String())
	} else {
		fmt.Fprintf(os.Stderr, "\033]1337;File=width=40;preserveAspectRatio=1;inline=1;:%s\a\n", buf.String())
	}
	return prompter.String("Captcha", "")
}

func simpleCaptchaPrompt(captchaPictureURL string) (string, error) {
	log.Println("Open this link in a browser:\n", captchaPictureURL)
	return prompter.String("Captcha", "")
}

// withLocale returns rawURL with hl=en and loc=US set in its query.
//
// Atmos fork: upstream concatenated "?hl=en&loc=US" (or "&hl=en&loc=US") onto the URL, which appended the
// parameters to the value of the last existing query parameter when the URL already had a query string.
// The other query parameters are kept byte for byte, and any existing hl or loc parameter is replaced.
func withLocale(rawURL string) string {
	base, fragment, hasFragment := strings.Cut(rawURL, "#")
	base, query, _ := strings.Cut(base, "?")

	var kept []string
	for _, pair := range strings.Split(query, "&") {
		name, _, _ := strings.Cut(pair, "=")
		if pair == "" || name == "hl" || name == "loc" {
			continue
		}
		kept = append(kept, pair)
	}
	kept = append(kept, "hl=en", "loc=US")

	result := base + "?" + strings.Join(kept, "&")
	if hasFragment {
		result += "#" + fragment
	}
	return result
}

// documentURL returns the URL a response document must resolve relative and missing form actions against.
// Atmos fork: upstream used the URL that was requested, which is wrong after a redirect. The final request
// URL of the response is used, falling back to the requested URL when the response carries no request metadata.
func documentURL(res *http.Response, requested string) (*url.URL, error) {
	if res != nil && res.Request != nil && res.Request.URL != nil {
		return res.Request.URL, nil
	}
	return url.Parse(requested)
}

func (kc *Client) loadFirstPage(loginDetails *creds.LoginDetails) (string, url.Values, error) {
	firstPageURL := withLocale(loginDetails.URL)

	req, err := http.NewRequest("GET", firstPageURL, nil)
	if err != nil {
		return "", nil, errors.Wrap(err, "error retrieving login form from idp")
	}

	res, err := kc.client.Do(req)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to make request to login form")
	}
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return "", nil, errors.Wrap(err, "error parsing first page html document")
	}

	doc.Url, err = documentURL(res, firstPageURL)
	if err != nil {
		return "", url.Values{}, errors.Wrap(err, "failed to define URL for html doc")
	}

	authForm, submitURL, err := extractInputsByFormID(doc, "gaia_loginform", "challenge")
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to build login form data")
	}

	return submitURL, authForm, err
}

func (kc *Client) loadLoginPage(submitURL string, referer string, authForm url.Values) (string, url.Values, error) {
	req, err := http.NewRequest("POST", submitURL, strings.NewReader(authForm.Encode()))
	if err != nil {
		return "", nil, errors.Wrap(err, "error retrieving login form")
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept-Language", "en-US")
	req.Header.Set("Content-Language", "en-US")
	req.Header.Set("Referer", referer)

	res, err := kc.client.Do(req)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to make request to login form")
	}
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return "", nil, errors.Wrap(err, "error parsing login page html document")
	}

	doc.Url, err = documentURL(res, submitURL)
	if err != nil {
		return "", url.Values{}, errors.Wrap(err, "failed to define URL for html doc")
	}

	loginForm, loginURL, err := extractInputsByFormID(doc, "gaia_loginform", "challenge")
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to build login form data")
	}

	return loginURL, loginForm, err
}

func (kc *Client) loadChallengePage(submitURL string, referer string, authForm url.Values, loginDetails *creds.LoginDetails) (*goquery.Document, error) {
	authForm.Set("bgresponse", "js_enabled")

	req, err := http.NewRequest("POST", submitURL, strings.NewReader(authForm.Encode()))
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving login form")
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept-Language", "en-US")
	req.Header.Set("Content-Language", "en-US")
	req.Header.Set("Referer", referer)

	res, err := kc.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make request to login form")
	}
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing login page html document")
	}

	doc.Url, err = documentURL(res, submitURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to define URL for html doc")
	}

	errMsg := mustFindErrorMsg(doc)

	if errMsg != "" {
		return nil, errors.New("Invalid username or password")
	}

	secondFactorHeader := "This extra step shows it’s really you trying to sign in"
	secondFactorHeader2 := "This extra step shows that it’s really you trying to sign in"
	secondFactorHeader3 := "2-Step Verification"
	secondFactorHeaderJp := "2 段階認証プロセス"

	// have we been asked for 2-Step Verification
	if extractNodeText(doc, "h2", secondFactorHeader) != "" ||
		extractNodeText(doc, "h2", secondFactorHeader2) != "" ||
		extractNodeText(doc, "h1", secondFactorHeader3) != "" ||
		extractNodeText(doc, "h1", secondFactorHeaderJp) != "" {

		responseForm, secondActionURL, err := extractInputsByFormID(doc, "challenge")
		if err != nil {
			return nil, errors.Wrap(err, "unable to extract challenge form")
		}

		logger.Debugf("secondActionURL: %s", provider.RedactRawURL(secondActionURL))

		switch {
		case strings.Contains(secondActionURL, "challenge/totp"): // handle TOTP challenge

			token := loginDetails.MFAToken
			if token == "" {
				var err error
				if token, err = prompter.RequestSecurityCode("000000"); err != nil {
					return nil, errors.Wrap(err, "failed to read the TOTP code")
				}
			}

			responseForm.Set("Pin", token)
			responseForm.Set("TrustDevice", "on") // Don't ask again on this computer

			return kc.loadResponsePage(secondActionURL, submitURL, responseForm)
		case strings.Contains(secondActionURL, "challenge/ipp"): // handle SMS challenge

			if extractNodeText(doc, "button", "Send text message") != "" {
				responseForm.Set("SendMethod", "SMS") // extractInputsByFormID does not extract the name and value from <button> tag that is the form submit
				doc, err = kc.loadResponsePage(secondActionURL, submitURL, responseForm)
				if err != nil {
					return nil, errors.Wrap(err, "failed to post sms request form")
				}
				submitURL = secondActionURL
				responseForm, secondActionURL, err = extractInputsByFormID(doc, "challenge")
				if err != nil {
					return nil, errors.Wrap(err, "unable to extract challenge form")
				}

				logger.Debugf("After sms request secondActionURL: %s", provider.RedactRawURL(secondActionURL))
			}

			token, err := prompter.StringRequired("Enter SMS token: G-")
			if err != nil {
				return nil, errors.Wrap(err, "failed to read the SMS token")
			}

			responseForm.Set("Pin", token)
			responseForm.Set("TrustDevice", "on") // Don't ask again on this computer

			return kc.loadResponsePage(secondActionURL, submitURL, responseForm)

		// Atmos fork: "challenge/skotp" must be matched before "challenge/sk"; upstream checked
		// "challenge/sk" first, which also matches skotp URLs and made this case unreachable.
		case strings.Contains(secondActionURL, "challenge/skotp"): // handle one-time HOTP challenge
			log.Println("Get a one-time code by visiting https://g.co/sc on another device where you can use your security key")
			token, err := prompter.RequestSecurityCode("000 000")
			if err != nil {
				return nil, errors.Wrap(err, "failed to read the one-time code")
			}

			responseForm.Set("Pin", token)
			responseForm.Set("TrustDevice", "on") // Don't ask again on this computer

			return kc.loadResponsePage(secondActionURL, submitURL, responseForm)
		case strings.Contains(secondActionURL, "challenge/sk"): // handle u2f challenge
			facetComponents, err := url.Parse(secondActionURL)
			if err != nil {
				return nil, errors.Wrap(err, "unable to parse action URL for U2F challenge")
			}
			facet := facetComponents.Scheme + "://" + facetComponents.Host
			challengeNonce := responseForm.Get("id-challenge")
			appID, data := extractKeyHandles(doc, challengeNonce)
			// Atmos fork: upstream indexed data[0] unconditionally and panicked when no key handle was found.
			if len(data) == 0 {
				return nil, errors.New("no U2F key handles found in challenge page")
			}
			u2fClient, err := NewU2FClient(challengeNonce, appID, facet, data[0], newU2FDeviceFinder())
			if err != nil {
				// Atmos fork: without an attached security key, another factor is offered when the page has a
				// skip form (as after a failure with an attached key). Otherwise the device error is returned.
				if (errors.Is(err, errNoDeviceFound) || errors.Is(err, errU2FOpenTimeout)) && doc.Find(`form[action$="skip"]`).Length() > 0 {
					logger.WithError(err).Warn("No usable security key; trying another second factor.")
					return kc.skipChallengePage(doc, submitURL, secondActionURL, loginDetails)
				}
				return nil, errors.Wrap(err, "Failed to prompt for second factor.")
			}

			response, err := u2fClient.ChallengeU2F()
			if err != nil {
				logger.WithError(err).Error("Second factor failed.")
				return kc.skipChallengePage(doc, submitURL, secondActionURL, loginDetails)
			}

			responseForm.Set("id-assertion", response)
			responseForm.Set("TrustDevice", "on")

			return kc.loadResponsePage(secondActionURL, submitURL, responseForm)
		case strings.Contains(secondActionURL, "challenge/az"): // handle phone challenge

			dataAttrs := extractDataAttributes(doc, "div[data-context]", []string{"data-context", "data-gapi-url", "data-tx-id", "data-api-key", "data-tx-lifetime"})

			// Atmos fork: log the attribute names only; the values include a transaction id and an API key.
			logger.Debugf("prompt with data attributes: %v", slices.Sorted(maps.Keys(dataAttrs)))

			waitValues := map[string]string{
				"txId": dataAttrs["data-tx-id"],
			}

			log.Println("Open the Google App, and tap 'Yes' on the prompt to sign in")

			waitRes, err := kc.postJSON(fmt.Sprintf("https://content.googleapis.com/cryptauth/v1/authzen/awaittx?alt=json&key=%s", dataAttrs["data-api-key"]), waitValues, submitURL)
			if err != nil {
				return nil, errors.Wrap(err, "unable to extract post wait tx form")
			}
			// Atmos fork: the response body was never closed, which prevents connection reuse.
			_ = waitRes.Body.Close()
			// Atmos fork: the HTTP client returns error statuses as responses, so a failed wait (for example a
			// 503) must stop the flow instead of submitting the verification form as if the phone approved.
			if waitRes.StatusCode < http.StatusOK || waitRes.StatusCode >= http.StatusMultipleChoices {
				return nil, fmt.Errorf("%w: status %s", errPhoneApprovalFailed, waitRes.Status)
			}

			// responseForm.Set("Pin", token)
			responseForm.Set("TrustDevice", "on") // Don't ask again on this computer

			return kc.loadResponsePage(secondActionURL, submitURL, responseForm)

		case strings.Contains(secondActionURL, "challenge/dp"): // handle device push challenge
			if extraNumber := extractDevicePushExtraNumber(doc); extraNumber != "" {
				log.Println("Check your phone and tap 'Yes' on the prompt, then tap the number:")
				log.Printf("\t%v\n", extraNumber)
				log.Println("Then press ENTER to continue.")
			} else {
				log.Print("Check your phone and tap 'Yes' on the prompt. Then press ENTER to continue.")
			}

			_, err := bufio.NewReader(os.Stdin).ReadBytes('\n')
			if err != nil {
				return nil, errors.Wrap(err, "error reading new line \\n")
			}
			responseForm.Set("TrustDevice", "on") // Don't ask again on this computer
			return kc.loadResponsePage(secondActionURL, submitURL, responseForm)

		}

		return kc.skipChallengePage(doc, submitURL, secondActionURL, loginDetails)

	} else if extractNodeText(doc, "h2", "To sign in to your Google Account, choose a task from the list below.") != "" {
		return kc.loadChallengeEntryPage(doc, submitURL, loginDetails)
	}

	return doc, nil
}

func (kc *Client) skipChallengePage(doc *goquery.Document, submitURL string, secondActionURL string, loginDetails *creds.LoginDetails) (*goquery.Document, error) {
	skipResponseForm, skipActionURL, err := extractInputsByFormQuery(doc, `[action$="skip"]`)
	if err != nil {
		return nil, errors.Wrap(err, "unable to extract skip form")
	}

	if skipActionURL == "" {
		return nil, errors.Errorf("unsupported second factor: %s", secondActionURL)
	}

	return kc.loadAlternateChallengePage(skipActionURL, submitURL, skipResponseForm, loginDetails)
}

func (kc *Client) loadAlternateChallengePage(submitURL string, referer string, authForm url.Values, loginDetails *creds.LoginDetails) (*goquery.Document, error) {
	req, err := http.NewRequest("POST", submitURL, strings.NewReader(authForm.Encode()))
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving login form")
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept-Language", "en-US")
	req.Header.Set("Content-Language", "en-US")
	req.Header.Set("Referer", referer)

	res, err := kc.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make request to login form")
	}
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing login page html document")
	}

	doc.Url, err = documentURL(res, submitURL)
	if err != nil {
		return nil, errors.Wrap(err, "failed to define URL for html doc")
	}

	return kc.loadChallengeEntryPage(doc, submitURL, loginDetails)
}

func (kc *Client) loadChallengeEntryPage(doc *goquery.Document, submitURL string, loginDetails *creds.LoginDetails) (*goquery.Document, error) {
	// Atmos fork: every challenge cycle (including the skip and alternate-factor paths) passes through here,
	// so one budget bounds the recursion that upstream left unlimited.
	kc.mfaTransitions++
	if kc.mfaTransitions > maxMFATransitions {
		return nil, errMFATransitionsExceeded
	}

	var challengeEntry string

	doc.Find("form[data-challengeentry]").EachWithBreak(func(i int, s *goquery.Selection) bool {
		action, ok := s.Attr("action")
		if !ok {
			return true
		}

		if strings.Contains(action, "challenge/totp/") ||
			strings.Contains(action, "challenge/ipp/") ||
			strings.Contains(action, "challenge/az/") ||
			strings.Contains(action, "challenge/dp/") ||
			strings.Contains(action, "challenge/skotp/") {

			challengeEntry, _ = s.Attr("data-challengeentry")
			return false
		}

		return true
	})

	if challengeEntry == "" {
		return nil, errors.New("unable to find supported second factor")
	}

	query := fmt.Sprintf(`[data-challengeentry="%s"]`, challengeEntry)
	responseForm, newActionURL, err := extractInputsByFormQuery(doc, query)
	if err != nil {
		return nil, errors.Wrap(err, "unable to extract challenge form")
	}

	return kc.loadChallengePage(newActionURL, submitURL, responseForm, loginDetails)
}

func (kc *Client) postJSON(submitURL string, values map[string]string, referer string) (*http.Response, error) {
	data, _ := json.Marshal(values)

	req, err := http.NewRequest("POST", submitURL, bytes.NewReader(data))
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving login form")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "en-US")
	req.Header.Set("Content-Language", "en-US")
	req.Header.Set("Referer", referer)

	res, err := kc.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to post JSON")
	}

	return res, nil
}

func (kc *Client) loadResponsePage(submitURL string, referer string, responseForm url.Values) (*goquery.Document, error) {
	req, err := http.NewRequest("POST", submitURL, strings.NewReader(responseForm.Encode()))
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving response page")
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept-Language", "en")
	req.Header.Set("Content-Language", "en-US")
	req.Header.Set("Referer", submitURL)

	res, err := kc.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make request to login form")
	}
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing login page html document")
	}

	// Atmos fork: NewDocumentFromReader leaves doc.Url nil, and later steps (for example a captcha
	// in the MFA response) dereference it. Use the final request URL, falling back to the submit URL.
	if doc.Url, err = documentURL(res, submitURL); err != nil {
		return nil, errors.Wrap(err, "failed to define URL for html doc")
	}

	return doc, nil
}

// findInputByName returns the value of the last input named name, or "" when there is none.
//
// Atmos fork: upstream (mustFindInputByName) called logger.Fatal, which exits the whole process,
// when the input had no value attribute. It now returns errInputValueNotFound.
func findInputByName(doc *goquery.Document, name string) (string, error) {
	var fieldValue string
	var err error

	q := fmt.Sprintf(`input[name="%s"]`, name)

	doc.Find(q).EachWithBreak(func(i int, s *goquery.Selection) bool {
		val, ok := s.Attr("value")
		if !ok {
			err = fmt.Errorf("%w: input %q", errInputValueNotFound, name)
			return false
		}
		fieldValue = val
		return true
	})
	if err != nil {
		return "", err
	}

	return fieldValue, nil
}

func mustFindErrorMsg(doc *goquery.Document) string {
	var fieldValue string
	doc.Find(".error-msg").Each(func(i int, s *goquery.Selection) {
		fieldValue = s.Text()
	})
	return fieldValue
}

func extractInputsByFormID(doc *goquery.Document, formID ...string) (url.Values, string, error) {
	// First try to find form by specific id.
	// Atmos fork: a form selected by id is used on its own; when it has no action,
	// extractInputsByFormQuery defaults to the document URL. Upstream fell through to
	// every form and merged their fields, posting them to another form's action.
	for _, id := range formID {
		query := fmt.Sprintf("#%s", id)
		if doc.Find(fmt.Sprintf("form%s", query)).Length() == 0 {
			continue
		}
		formData, actionURL, err := extractInputsByFormQuery(doc, query)
		if err != nil {
			return nil, "", err
		}
		return formData, actionURL, nil
	}

	// No form with the given ids exists, so use one form.
	// Atmos fork: upstream merged the fields of every form and used the action of the last one, so a second
	// unnamed form (for example account recovery) supplied the action for the first form's fields. The first
	// form that has an action is used, or the first form when none has, with its own action and fields.
	forms := doc.Find("form")
	if forms.Length() == 0 {
		return nil, "", errors.New("could not find any forms")
	}
	chosen := forms.First()
	forms.EachWithBreak(func(_ int, f *goquery.Selection) bool {
		if _, ok := f.Attr("action"); ok {
			chosen = f
			return false
		}
		return true
	})

	formData, actionURL, err := extractInputsFromForms(doc, chosen)
	if err != nil || len(formData) == 0 {
		return nil, "", errors.New("could not find any forms")
	}
	return formData, actionURL, nil
}

func extractInputsByFormQuery(doc *goquery.Document, formQuery string) (url.Values, string, error) {
	query := fmt.Sprintf("form%s", formQuery)

	// Atmos fork: goquery leaves doc.Url nil for reader-based documents; upstream dereferenced it.
	if doc.Url == nil {
		return url.Values{}, "", errors.New("document has no URL")
	}

	foundForms := doc.Find(query)
	if len(foundForms.Nodes) == 0 {
		return url.Values{}, "", fmt.Errorf("could not find form with query %q", query)
	}

	return extractInputsFromForms(doc, foundForms)
}

// extractInputsFromForms returns the fields of the given forms and the action URL of the last form that
// has an action, resolved against the document URL (the document URL itself when no form has an action).
func extractInputsFromForms(doc *goquery.Document, foundForms *goquery.Selection) (url.Values, string, error) {
	formData := url.Values{}
	var actionAttr string

	if doc.Url == nil {
		return formData, "", errors.New("document has no URL")
	}
	currentURL := doc.Url.String()

	foundForms.Each(func(i int, s *goquery.Selection) {
		action, ok := s.Attr("action")
		if !ok {
			return
		}
		actionAttr = action
	})
	// Atmos fork: a form without an action submits to the document URL.
	if actionAttr == "" {
		actionAttr = currentURL
	}

	actionURL, err := generateFullURLIfRelative(actionAttr, currentURL)
	if err != nil {
		return formData, "", errors.Wrap(err, "error getting action URL")
	}

	// extract form data to passthrough
	foundForms.Find("input").Each(func(i int, s *goquery.Selection) {
		name, ok := s.Attr("name")
		if !ok {
			return
		}
		val, ok := s.Attr("value")
		if !ok {
			return
		}
		// Atmos fork: log the field name only; values carry authentication state and can hold secrets.
		logger.Debugf("form input: %s", name)
		formData.Add(name, val)
	})

	return formData, actionURL, nil
}

// normalizeSpace trims s and collapses every run of whitespace into a single space.
func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func extractNodeText(doc *goquery.Document, tag, txt string) string {
	var res string

	doc.Find(tag).Each(func(i int, s *goquery.Selection) {
		// Atmos fork: compare with whitespace normalized, because headings are often wrapped in newlines and indentation.
		if normalizeSpace(s.Text()) == normalizeSpace(txt) {
			res = normalizeSpace(s.Text())
		}
	})

	return res
}

func extractDataAttributes(doc *goquery.Document, query string, attrsToSelect []string) map[string]string {
	dataAttrs := make(map[string]string)

	doc.Find(query).Each(func(_ int, sel *goquery.Selection) {
		for _, f := range attrsToSelect {
			if val, ok := sel.Attr(f); ok {
				dataAttrs[f] = val
			}
		}
	})

	return dataAttrs
}

func extractKeyHandles(doc *goquery.Document, challengeTxt string) (string, []string) {
	appId := ""
	keyHandles := []string{}
	result := map[string]interface{}{}
	doc.Find("div[jsname=C0oDBd]").Each(func(_ int, sel *goquery.Selection) {
		val, ok := sel.Attr("data-challenge-ui")
		if ok {
			firstIdx := strings.Index(val, "{")
			lastIdx := strings.LastIndex(val, "}")
			// Atmos fork: upstream sliced with unchecked indexes and panicked on values without a JSON object, such as "[]".
			if firstIdx < 0 || lastIdx < firstIdx {
				return
			}
			obj := []byte(val[firstIdx : lastIdx+1])
			_ = json.Unmarshal(obj, &result) // ignore the error and continue
			// Key handles
			for _, val := range result {
				list, ok := val.([]interface{})
				if !ok {
					continue
				}
				tmpId, stringList := filterKeyHandleList(list, challengeTxt)
				if tmpId != "" {
					appId = tmpId
				}
				if len(stringList) != 0 {
					keyHandles = append(keyHandles, stringList...)
				}
			}
		}
	})
	return appId, keyHandles
}

func filterKeyHandleList(list []interface{}, challengeTxt string) (string, []string) {
	appId := ""
	newList := []string{}
	for _, entry := range list {
		if entry == nil {
			continue
		}
		moreList, ok := entry.([]interface{})
		if ok {
			id, l := filterKeyHandleList(moreList, challengeTxt)
			if id != "" {
				appId = id
			}
			newList = append(newList, l...)
			continue
		}
		str, ok := entry.(string)
		if !ok {
			continue
		}
		if appId == "" {
			appId = isAppId(str)
		}
		if isKeyHandle(str, challengeTxt) {
			newList = append(newList, str)
		}
	}
	return appId, newList
}

func generateFullURLIfRelative(destination, currentPageURL string) (string, error) {
	// Atmos fork: upstream indexed destination[0] and panicked on forms without an action attribute.
	if destination == "" {
		return destination, nil
	}

	// Atmos fork: upstream resolved only root-relative destinations, so a directory-relative action such as
	// "next" stayed without a scheme and host. Every relative reference is now resolved against the page URL.
	reference, err := url.Parse(destination)
	if err != nil || reference.IsAbs() {
		return destination, nil
	}

	currentURLParsed, err := url.Parse(currentPageURL)
	if err != nil {
		if strings.HasPrefix(destination, "/") {
			return "", errors.Wrap(err, "error generating full URL")
		}
		return destination, nil
	}

	return currentURLParsed.ResolveReference(reference).String(), nil
}

func isKeyHandle(key, challengeTxt string) bool {
	_, err := b64.StdEncoding.DecodeString(key)
	if err != nil {
		return false
	}
	return key != challengeTxt
}

func isAppId(val string) string {
	obj := map[string]interface{}{}
	err := json.Unmarshal([]byte(val), &obj)
	if err != nil {
		return ""
	}
	appId, ok := obj["appid"].(string)
	if !ok {
		return ""
	}
	return appId
}

func extractDevicePushExtraNumber(doc *goquery.Document) string {
	extraNumber := ""
	doc.Find("div[jsname=feLNVc]").Each(func(_ int, s *goquery.Selection) {
		extraNumber = s.Text()
	})
	return extraNumber
}
