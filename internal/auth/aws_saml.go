package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/charmbracelet/log"
	"github.com/chromedp/chromedp"
	"github.com/cloudposse/atmos/pkg/schema"
	_ "github.com/versent/saml2aws/v2"
	"github.com/versent/saml2aws/v2/pkg/cfg"

	"net/http"
	"time"
)

type awsSaml struct {
	schema.IdentityDefaultConfig `yaml:",inline"`
	IdpAccount                   cfg.IDPAccount `yaml:",inline"`
}

func (i *awsSaml) Login() error {

	samlResponse, err := captureSAMLResponse(i.IdpAccount.URL)
	if err != nil {
		return err
	}
	log.Info("SAML Response", "response", samlResponse)
	return nil
}

func captureSAMLResponse(ssoURL string) (string, error) {
	log.Info("Capturing SAML Response...", "sso_url", ssoURL)
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false), // ⬅️ disables headless mode
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("start-maximized", true),
	)
	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// Set a timeout to avoid hanging
	ctx, timeoutCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer timeoutCancel()

	var samlValue string

	tasks := chromedp.Tasks{
		chromedp.Navigate(ssoURL),

		// Wait for form input to appear — this may take time due to SSO/MFA
		chromedp.WaitVisible(`input[name="SAMLResponse"]`, chromedp.ByQuery),

		// Read its value
		chromedp.Value(`input[name="SAMLResponse"]`, &samlValue, chromedp.ByQuery),
	}

	err := chromedp.Run(ctx, tasks)
	if err != nil {
		return "", fmt.Errorf("chromedp failed: %w", err)
	}

	// Optional: Decode to verify it's a valid SAML XML blob
	decoded, err := base64.StdEncoding.DecodeString(samlValue)
	if err != nil {
		return "", fmt.Errorf("base64 decode failed: %w", err)
	}

	log.Info("SAML XML snippet:", string(decoded[:120])+"...")

	return samlValue, nil
}

func startSAMLServer(samlChan chan string) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "ParseForm() error", http.StatusBadRequest)
			return
		}

		saml := r.FormValue("SAMLResponse")
		if saml == "" {
			http.Error(w, "Missing SAMLResponse", http.StatusBadRequest)
			return
		}

		log.Info("Received SAML assertion")
		fmt.Fprint(w, "SAML assertion received. You can close this tab.")
		samlChan <- saml
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}

func (i *awsSaml) Logout() error {
	return nil
}

func (i *awsSaml) Validate() error {
	return nil
}
