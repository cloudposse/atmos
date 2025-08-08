# ADR: Approach for AWS SAML Login Implementation

**Date:** 2025-08-08

**Status:** Accepted

## Context

We need to implement an AWS SAML login function in Atmos that:

- Supports multiple identity providers (IDPs) without writing custom integrations for each (Okta, Ping, ADFS, AzureAD, OneLogin, etc.).
- Allows the user to authenticate interactively in a **non-headless browser** window.
- Retrieves a SAML assertion and exchanges it for AWS STS credentials.
- Minimizes the need to maintain complex scraping or protocol-handling code ourselves.

Two implementation paths were considered:

1. **Use `saml2aws` library directly**
   Leverage [`github.com/Versent/saml2aws`](https://github.com/Versent/saml2aws), which already:

   - Supports dozens of IDPs.
   - Handles SAML form scraping, role parsing, and STS credential exchange.
   - Provides a `Browser` provider that launches a Playwright-controlled Chromium window for interactive login.
   - Parses AWS roles from the assertion, eliminating the need to reimplement this logic.
   - Has been battle-tested in production by many users.

2. **Implement our own "open default browser + wait for SAML response" flow**

   - Open the system’s default browser (`open` on macOS, `xdg-open` on Linux, `start` on Windows).
   - Run a small local HTTP server to capture the SAML POST back from the IDP.
   - Manually extract the `SAMLResponse` and feed it into our AWS role parsing and STS exchange.
   - Optionally use bookmarklets or browser extensions to capture the assertion from the AWS SAML redirect page.

## Decision

We will **use the `saml2aws` library** for the initial implementation.

The login function will:

- Use the `Browser` provider from `saml2aws` to launch a Playwright-controlled Chromium window in non-headless mode.
- Allow the user to authenticate directly in the browser UI without passing credentials on the CLI or in code.
- Capture the SAML assertion via the built-in provider flow.
- Reuse saml2aws’ role parsing and STS AssumeRoleWithSAML logic.

We will not implement the “open default browser + wait for response” approach at this stage.

## Rationale

- **Time savings & reduced complexity:** `saml2aws` already solves the hardest part—navigating IDP-specific login flows and scraping the SAML assertion. Implementing this ourselves would mean re-creating a large portion of that complexity.
- **Broad IDP support out-of-the-box:** All IDPs supported by `saml2aws` will work with minimal additional code.
- **Less maintenance risk:** Keeping up with IDP login flow changes is delegated to the upstream `saml2aws` project.
- **Quick path to working solution:** This lets us ship a functional login feature quickly, with the option to optimize UX later.
- **Future flexibility:** If we later want to:

  - Use the OS default browser instead of Playwright.
  - Integrate with OS-level keychains for caching assertions.
  - Replace the login mechanism entirely.
    …we can switch to the alternative approach or a hybrid without throwing away the role parsing and STS logic from saml2aws.

## Consequences

- **Not using the system’s default browser initially:** The Playwright Chromium instance is separate from the user’s daily browser, so existing sessions and cookies are not reused.
- **Requires Playwright browser binaries:** First-time logins will download a Chromium driver if not already installed.
- **Local browser state is file-based:** Stored in `~/.aws/saml2aws/storageState.json` (or equivalent path on other OSes), not in the OS keychain. We’ll ensure the directory exists automatically.
- **Dependency on upstream project:** Changes or deprecations in `saml2aws` could require adjustments in our integration.

## Alternatives Considered

1. **Open default browser + wait for SAML**

   - Pros: Uses the user’s preferred browser with existing sessions; no extra binary downloads.
   - Cons: Would require either manual copy-pasting of assertions, bookmarklets, or a custom browser extension; loses automatic scraping capabilities from `saml2aws` without forking it.

## Future Considerations

- Add **keyring support** to store short-lived assertions and STS credentials in an OS-agnostic secure store.
- Explore **Exec provider** or custom flow to open the system default browser and still capture SAML without Playwright.
- Monitor `saml2aws` upstream changes for improved browser flexibility.
