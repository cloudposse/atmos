package aws

// OAuth2 protocol helpers: PKCE generation, authorization URL construction,
// local callback HTTP server.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// generatePKCEPair generates a PKCE code verifier and code challenge.
// The verifier is 32 random bytes, base64url-encoded without padding.
// The challenge is the SHA-256 hash of the verifier, base64url-encoded without padding.
func generatePKCEPair() (verifier string, challenge string, err error) {
	defer perf.Track(nil, "aws.generatePKCEPair")()

	buf := make([]byte, webflowCodeVerifierBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	verifier = base64.RawURLEncoding.EncodeToString(buf)

	hash := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(hash[:])

	return verifier, challenge, nil
}

// generateStateString generates a base64url-encoded random string of
// webflowStateBytes length, suitable for use as an OAuth2 state nonce.
func generateStateString() (string, error) {
	buf := make([]byte, webflowStateBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// getSigninEndpoint returns the AWS signin endpoint for the given region.
// The us-east-1 region uses the global endpoint (no region prefix).
func getSigninEndpoint(region string) string {
	if region == "us-east-1" {
		return "https://signin.aws.amazon.com"
	}
	return fmt.Sprintf("https://%s.signin.aws.amazon.com", region)
}

// buildAuthorizationURL constructs the OAuth2 authorization URL.
func buildAuthorizationURL(region string, port int, codeChallenge, state string) string {
	baseURL := getSigninEndpoint(region)
	params := url.Values{}
	params.Set("client_id", webflowOAuthClientID)
	params.Set("redirect_uri", fmt.Sprintf("http://127.0.0.1:%d%s", port, webflowCallbackPath))
	params.Set("response_type", webflowResponseType)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", webflowCodeChallengeMethod)
	params.Set("scope", webflowScope)
	params.Set("state", state)
	return fmt.Sprintf("%s/v1/authorize?%s", baseURL, params.Encode())
}

// startCallbackServer starts an HTTP server on an ephemeral port to receive the OAuth2 callback.
// It returns the listener, a channel for the result, and any error.
func startCallbackServer(ctx context.Context, expectedState string) (net.Listener, <-chan webflowResult, error) {
	defer perf.Track(nil, "aws.startCallbackServer")()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to bind to loopback address: %w", err)
	}

	resultCh := make(chan webflowResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(webflowCallbackPath, makeCallbackHandler(expectedState, resultCh))

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: webflowCallbackReadHeaderTimeout,
	}

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Debug("Callback server error", "error", err)
		}
	}()

	// Shut down server when context is cancelled.
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), webflowCallbackShutdownTimeout)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	return listener, resultCh, nil
}

// makeCallbackHandler returns an http.HandlerFunc that validates the OAuth2
// callback parameters and delivers the result to resultCh.
func makeCallbackHandler(expectedState string, resultCh chan<- webflowResult) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		errParam := r.URL.Query().Get("error")

		if errParam != "" {
			errDesc := r.URL.Query().Get("error_description")
			resultCh <- webflowResult{err: fmt.Errorf("%w: %s: %s", errUtils.ErrWebflowAuthorizationError, errParam, errDesc)}
			http.Error(w, "Authorization failed. You can close this tab.", http.StatusBadRequest)
			return
		}

		if code == "" {
			resultCh <- webflowResult{err: errUtils.ErrWebflowMissingCallbackCode}
			http.Error(w, "Missing authorization code. You can close this tab.", http.StatusBadRequest)
			return
		}

		if state != expectedState {
			resultCh <- webflowResult{err: errUtils.ErrWebflowStateMismatch}
			http.Error(w, "State mismatch. You can close this tab.", http.StatusBadRequest)
			return
		}

		resultCh <- webflowResult{code: code, state: state}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body><h2>Authentication successful!</h2><p>You can close this tab and return to your terminal.</p><script>window.close()</script></body></html>`)
	}
}

// exchangeCodeParams bundles the arguments needed to exchange an authorization
// code for AWS credentials. Grouped to keep the exchange function signature
// under the revive argument-limit.
type exchangeCodeParams struct {
	region       string
	code         string
	codeVerifier string
	redirectURI  string
}
