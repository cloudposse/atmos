package aws

// Browser-based webflow orchestration: session preparation, interactive and
// non-interactive paths, spinner-backed callback wait, and token-response
// post-processing.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui"
)

// browserWebflow performs the full OAuth2 PKCE browser authentication flow.
func (i *userIdentity) browserWebflow(ctx context.Context, region string) (*types.AWSCredentials, error) {
	allowPrompts := types.AllowPrompts(ctx)

	// Use webflowIsInteractive so --force-tty / ATMOS_FORCE_TTY=true drives
	// the interactive path even in environments where TTY auto-detection
	// fails (e.g. screenshot generation, nested PTY scenarios).
	if allowPrompts && webflowIsInteractive() {
		return i.browserWebflowInteractive(ctx, region)
	}

	// Non-interactive mode: display URL for manual auth.
	if allowPrompts {
		return i.browserWebflowNonInteractive(ctx, region)
	}

	return nil, errUtils.Build(errUtils.ErrWebflowAuthFailed).
		WithExplanation("Browser authentication requires an interactive terminal or prompts").
		WithHint("Use 'atmos auth login' in an interactive terminal").
		WithHint("Or configure static credentials in atmos.yaml or keychain").
		WithContext("identity", i.name).
		WithExitCode(2).
		Err()
}

// browserWebflowInteractive performs the browser flow with automatic browser opening and callback server.
func (i *userIdentity) browserWebflowInteractive(ctx context.Context, region string) (*types.AWSCredentials, error) {
	setup, err := i.prepareWebflowSession(ctx, region)
	if err != nil {
		return nil, err
	}
	defer setup.listener.Close()

	// Display UI and open browser.
	displayWebflowDialogFunc(setup.authURL)

	if !telemetry.IsCI() {
		if err := openURLFunc(setup.authURL); err != nil {
			log.Debug("Failed to open browser automatically", "error", err)
		}
	}

	// Wait for callback with spinner.
	tokenResp, err := i.waitForCallbackWithSpinner(ctx, setup.resultCh, setup.region, setup.verifier, setup.redirectURI)
	if err != nil {
		return nil, err
	}

	return i.processTokenResponse(tokenResp, region), nil
}

// browserWebflowNonInteractive performs the browser flow with manual code entry.
func (i *userIdentity) browserWebflowNonInteractive(ctx context.Context, region string) (*types.AWSCredentials, error) {
	setup, err := i.prepareWebflowSession(ctx, region)
	if err != nil {
		return nil, err
	}
	defer setup.listener.Close()

	// Display URL for manual use.
	displayWebflowPlainTextFunc(setup.authURL)

	// Only start the stdin reader when stdin can actually produce user input.
	// In piped/closed stdin environments (CI, `cmd < file`, `go test`), reading
	// os.Stdin would return EOF immediately and incorrectly abort the valid
	// callback flow before the OAuth2 callback has a chance to complete.
	//
	// NOTE (known limitation): if the callback wins the race in an interactive
	// environment, the stdin reader goroutine remains parked on
	// bufio.Scanner.Scan() because there is no cancellation primitive for a
	// blocking os.Stdin read. This is acceptable for short-lived
	// `atmos auth login` invocations (bounded by process exit); repeated auth
	// calls within a single long-running process could theoretically have the
	// leaked goroutine steal a later stdin byte.
	var codeCh <-chan string
	var errCh <-chan error
	if webflowStdinIsReadableFunc() {
		codeCh, errCh = readStdinAuthCode()
	}
	return i.awaitNonInteractiveAuthCode(ctx, setup, codeCh, errCh)
}

// webflowSessionSetup groups the per-session PKCE state and callback server
// artifacts returned by prepareWebflowSession.
type webflowSessionSetup struct {
	listener    net.Listener
	resultCh    <-chan webflowResult
	authURL     string
	redirectURI string
	verifier    string
	region      string
}

// prepareWebflowSession generates PKCE + state, starts the local callback
// server, and builds the authorization URL. Shared between interactive and
// non-interactive flows.
func (i *userIdentity) prepareWebflowSession(ctx context.Context, region string) (*webflowSessionSetup, error) {
	verifier, challenge, err := generatePKCEPair()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to generate PKCE pair: %w", errUtils.ErrWebflowAuthFailed, err)
	}

	state, err := generateStateString()
	if err != nil {
		return nil, fmt.Errorf("%w: failed to generate state: %w", errUtils.ErrWebflowAuthFailed, err)
	}

	listener, resultCh, err := startCallbackServer(ctx, state)
	if err != nil {
		return nil, wrapWebflowErr(errUtils.ErrWebflowCallbackServer, err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	return &webflowSessionSetup{
		listener:    listener,
		resultCh:    resultCh,
		authURL:     buildAuthorizationURL(region, port, challenge, state),
		redirectURI: fmt.Sprintf("http://127.0.0.1:%d%s", port, webflowCallbackPath),
		verifier:    verifier,
		region:      region,
	}, nil
}

// readStdinAuthCode spawns a goroutine that reads a single line from
// webflowStdinReader and delivers either a code or an error via channels.
//
// A clean EOF (scanner.Scan returns false with no underlying error) is NOT
// reported on errCh: the goroutine exits silently so the enclosing select
// can continue waiting for the OAuth callback or context cancellation.
// Treating EOF as a fatal error would incorrectly abort a valid callback
// flow in any environment where stdin is closed/piped at the moment the
// reader is started — which is the common case for this non-interactive
// fallback (CI, screenshot capture, `cmd < /dev/null`).
//
// Only three outcomes surface to the caller:
//   - codeCh: a non-empty line was read (user pasted a code)
//   - errCh(ErrWebflowCodeRequired): the user pressed enter without typing
//   - errCh(ErrWebflowReadAuthCodeFailed): a non-EOF read error
func readStdinAuthCode() (<-chan string, <-chan error) {
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		// Route the prompt through the UI layer so it is subject to the
		// same stream abstraction (masking, TTY handling, test capture) as
		// the rest of atmos's stderr output. See CLAUDE.md §"I/O and UI
		// Usage": never use fmt.Fprintf(os.Stderr, ...).
		ui.Write("Authorization code: ")
		scanner := bufio.NewScanner(webflowStdinReader)
		if scanner.Scan() {
			code := strings.TrimSpace(scanner.Text())
			if code == "" {
				errCh <- errUtils.ErrWebflowCodeRequired
				return
			}
			codeCh <- code
			return
		}
		if err := scanner.Err(); err != nil {
			errCh <- wrapWebflowErr(errUtils.ErrWebflowReadAuthCodeFailed, err)
			return
		}
		// Clean EOF — silently exit, leaving the enclosing select to wait
		// for the callback server or context cancellation.
	}()
	return codeCh, errCh
}

// awaitNonInteractiveAuthCode waits for whichever source (callback or stdin)
// produces an authorization code first, then exchanges it for credentials.
func (i *userIdentity) awaitNonInteractiveAuthCode(ctx context.Context, setup *webflowSessionSetup, codeCh <-chan string, errCh <-chan error) (*types.AWSCredentials, error) {
	select {
	case result := <-setup.resultCh:
		if result.err != nil {
			return nil, fmt.Errorf("%w: callback error: %w", errUtils.ErrWebflowAuthFailed, result.err)
		}
		return i.exchangeAndProcess(ctx, setup, result.code)
	case code := <-codeCh:
		return i.exchangeAndProcess(ctx, setup, code)
	case readErr := <-errCh:
		return nil, wrapWebflowErr(errUtils.ErrWebflowAuthFailed, readErr)
	case <-ctx.Done():
		return nil, wrapWebflowErr(errUtils.ErrWebflowTimeout, ctx.Err())
	}
}

// exchangeAndProcess runs the token exchange for a given authorization code
// and converts the response into AWS credentials. Small helper used by the
// non-interactive select branches.
func (i *userIdentity) exchangeAndProcess(ctx context.Context, setup *webflowSessionSetup, code string) (*types.AWSCredentials, error) {
	tokenResp, err := exchangeCodeForCredentials(ctx, defaultHTTPClient, exchangeCodeParams{
		region: setup.region, code: code, codeVerifier: setup.verifier, redirectURI: setup.redirectURI,
	})
	if err != nil {
		return nil, err
	}
	return i.processTokenResponse(tokenResp, setup.region), nil
}

// waitForCallbackWithSpinner waits for the OAuth2 callback with a spinner UI.
func (i *userIdentity) waitForCallbackWithSpinner(ctx context.Context, resultCh <-chan webflowResult, region, verifier, redirectURI string) (*webflowTokenResponse, error) {
	// Run the spinner only when we have an interactive terminal (or
	// --force-tty) AND we are NOT in CI. CI suppression is preserved even
	// when a real TTY is attached, because spinner escape sequences pollute
	// CI logs.
	if !webflowIsInteractive() || telemetry.IsCI() {
		return i.waitForCallbackSimple(ctx, resultCh, region, verifier, redirectURI)
	}

	ctx, cancel := context.WithTimeout(ctx, webflowCallbackTimeout)
	defer cancel()

	tokenCh := startSpinnerExchangeGoroutine(ctx, resultCh, region, verifier, redirectURI)

	finalModel, err := runSpinnerProgramFunc(newWebflowSpinnerModel(tokenCh, cancel))
	if err != nil {
		return i.handleSpinnerFallback(&spinnerFallbackParams{
			cancel: cancel, tokenCh: tokenCh, resultCh: resultCh,
			region: region, verifier: verifier, redirectURI: redirectURI, runErr: err,
		})
	}

	final := finalModel.(webflowSpinnerModel)
	if final.result == nil {
		return nil, errUtils.Build(errUtils.ErrWebflowAuthFailed).
			WithExplanation("Browser authentication did not complete").
			WithHint("Try running the authentication again").
			WithContext("identity", i.name).
			WithExitCode(1).
			Err()
	}
	return final.result.resp, final.result.err
}

// startSpinnerExchangeGoroutine launches a goroutine that races the OAuth
// callback against the context deadline and exchanges the authorization code
// for tokens when the callback arrives. The returned channel delivers either
// a successful response or a wrapped error.
func startSpinnerExchangeGoroutine(ctx context.Context, resultCh <-chan webflowResult, region, verifier, redirectURI string) chan webflowSpinnerTokenResult {
	tokenCh := make(chan webflowSpinnerTokenResult, 1)
	go func() {
		defer close(tokenCh)
		select {
		case result := <-resultCh:
			if result.err != nil {
				tokenCh <- webflowSpinnerTokenResult{err: wrapWebflowErr(errUtils.ErrWebflowAuthFailed, result.err)}
				return
			}
			resp, err := exchangeCodeForCredentials(ctx, defaultHTTPClient, exchangeCodeParams{
				region: region, code: result.code, codeVerifier: verifier, redirectURI: redirectURI,
			})
			tokenCh <- webflowSpinnerTokenResult{resp: resp, err: err}
		case <-ctx.Done():
			tokenCh <- webflowSpinnerTokenResult{err: wrapWebflowErr(errUtils.ErrWebflowTimeout, ctx.Err())}
		}
	}()
	return tokenCh
}

// spinnerFallbackParams groups the arguments needed to drain and fall back
// from a failed bubbletea spinner run. Grouped to keep the method under the
// revive argument-limit.
type spinnerFallbackParams struct {
	cancel      context.CancelFunc
	tokenCh     chan webflowSpinnerTokenResult
	resultCh    <-chan webflowResult
	region      string
	verifier    string
	redirectURI string
	runErr      error
}

// handleSpinnerFallback handles the case where tea.NewProgram.Run returns an
// error (e.g. when stderr is not a real TTY in tests). It drains the exchange
// goroutine to avoid losing a callback that arrived just before cancellation,
// and otherwise falls back to a blocking simple wait with a fresh timeout.
func (i *userIdentity) handleSpinnerFallback(p *spinnerFallbackParams) (*webflowTokenResponse, error) {
	log.Debug("Bubbletea spinner failed, falling back to simple wait", "error", p.runErr)
	p.cancel()
	// Drain the goroutine result. If the callback arrived just before cancel,
	// the goroutine may have captured the real result instead of a timeout.
	res := <-p.tokenCh
	if !errors.Is(res.err, errUtils.ErrWebflowTimeout) {
		return res.resp, res.err
	}
	// Goroutine got the cancellation — callback hasn't arrived yet.
	// Fall back to a blocking wait with a fresh timeout.
	ctx2, cancel2 := context.WithTimeout(context.Background(), webflowCallbackTimeout)
	defer cancel2()
	return i.waitForCallbackSimple(ctx2, p.resultCh, p.region, p.verifier, p.redirectURI)
}

// waitForCallbackSimple waits for callback without spinner (non-TTY environments).
func (i *userIdentity) waitForCallbackSimple(ctx context.Context, resultCh <-chan webflowResult, region, verifier, redirectURI string) (*webflowTokenResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, webflowCallbackTimeout)
	defer cancel()

	select {
	case result := <-resultCh:
		if result.err != nil {
			return nil, wrapWebflowErr(errUtils.ErrWebflowAuthFailed, result.err)
		}
		return exchangeCodeForCredentials(ctx, defaultHTTPClient, exchangeCodeParams{
			region: region, code: result.code, codeVerifier: verifier, redirectURI: redirectURI,
		})
	case <-ctx.Done():
		return nil, wrapWebflowErr(errUtils.ErrWebflowTimeout, ctx.Err())
	}
}

// processTokenResponse converts a token response to AWS credentials and caches the refresh token.
func (i *userIdentity) processTokenResponse(tokenResp *webflowTokenResponse, region string) *types.AWSCredentials {
	creds := tokenResponseToCredentials(tokenResp, region)

	// Cache refresh token for future use.
	if tokenResp.RefreshToken != "" {
		i.saveRefreshCache(&webflowRefreshCache{
			RefreshToken: tokenResp.RefreshToken,
			Region:       region,
			ExpiresAt:    time.Now().Add(webflowSessionDuration),
		})
	}

	return creds
}
