// Package github is the authority that registers a key on a GitHub user account, as a signing
// key, an authentication key, or both.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
)

// Name is the registry key of the GitHub authority.
const Name = "github"

// Factory builds the GitHub authority for a registry.
var Factory = authority.Factory{
	Name: Name,
	New:  func(o authority.Options) (authority.Authority, error) { return New(o) },
}

// Scopes GitHub requires for key management.
const (
	ScopeSigningRead   = "read:ssh_signing_key"
	ScopeSigningWrite  = "write:ssh_signing_key"
	ScopeAuthRead      = "read:public_key"
	ScopeAuthWrite     = "write:public_key"
	defaultKeyTitle    = "atmos-ssh"
	operationLogin     = "get user"
	operationList      = "list keys"
	operationCreate    = "add key"
	operationDelete    = "remove key"
	maxPagesExceededBy = "too many pages of keys"
)

type endpoint struct {
	path       string
	readScope  string
	writeScope string
}

var endpoints = map[authority.Use]endpoint{
	authority.UseSigning:        {path: "/user/ssh_signing_keys", readScope: ScopeSigningRead, writeScope: ScopeSigningWrite},
	authority.UseAuthentication: {path: "/user/keys", readScope: ScopeAuthRead, writeScope: ScopeAuthWrite},
}

// Authority registers keys on the account the token belongs to.
type Authority struct {
	httpClient *http.Client
	base       *url.URL
	agent      string
}

// New builds the authority. Options.HTTPClient and Options.BaseURL are injectable; the defaults
// are a plain client and https://api.github.com.
func New(options authority.Options) (*Authority, error) {
	raw := options.BaseURL
	if raw == "" {
		raw = DefaultBaseURL
	}
	base, err := url.Parse(raw)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, fmt.Errorf("%w: base URL %q", authority.ErrInvalidRequest, raw)
	}
	agent := options.UserAgent
	if agent == "" {
		agent = DefaultUserAgent
	}
	return &Authority{httpClient: noWriteRedirects(options.HTTPClient), base: base, agent: agent}, nil
}

// noWriteRedirects returns a copy of hc that never follows a redirect for a request that is not a
// GET, so a token and a request body cannot be replayed at an address GitHub did not intend. It does
// not rely on the caller's client for this.
func noWriteRedirects(hc *http.Client) *http.Client {
	if hc == nil {
		hc = &http.Client{}
	}
	cp := *hc
	inner := hc.CheckRedirect
	cp.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.Method != http.MethodGet {
			return http.ErrUseLastResponse
		}
		if inner != nil {
			return inner(req, via)
		}
		if len(via) >= 10 {
			return http.ErrUseLastResponse
		}
		return nil
	}
	return &cp
}

// Name implements authority.Authority.
func (*Authority) Name() string { return Name }

// Supports implements authority.Authority.
func (*Authority) Supports() []authority.Use {
	return []authority.Use{authority.UseAuthentication, authority.UseSigning}
}

func (a *Authority) newClient(operation string, credentials map[string]string) (*client, error) {
	token := strings.TrimSpace(credentials[authority.CredentialToken])
	if token == "" {
		return nil, &authority.Error{Authority: Name, Operation: operation, Err: authority.ErrUnauthorized, Detail: "no token supplied"}
	}
	return &client{http: a.httpClient, base: a.base, token: token, agent: a.agent, timeout: requestTimeout}, nil
}

type remoteKey struct {
	ID    int64  `json:"id"`
	Key   string `json:"key"`
	Title string `json:"title"`
}

// Ensure implements authority.Authority. A key already on the account is adopted. When it fails
// part way it returns the receipt of the uses that were enrolled.
func (a *Authority) Ensure(ctx context.Context, credentials map[string]string, request authority.Request) (authority.Receipt, error) {
	receipt := authority.NewReceipt(Name, request)
	if err := authority.ValidateRequest(a, request); err != nil {
		return receipt, err
	}
	c, err := a.newClient("ensure", credentials)
	if err != nil {
		return receipt, err
	}
	login, err := c.login(ctx)
	if err != nil {
		return receipt, err
	}
	receipt.Subject = login
	title := request.Title
	if title == "" {
		title = defaultKeyTitle
	}
	for _, use := range authority.NormalizeUses(request.Uses) {
		id, err := c.ensureKey(ctx, use, authority.AuthorizedKey(request.Key), title)
		if err != nil {
			return receipt, err
		}
		receipt.References[use] = id
		receipt.Uses = append(receipt.Uses, use)
	}
	return receipt, nil
}

func (c *client) login(ctx context.Context) (string, error) {
	resp, err := c.do(ctx, call{operation: operationLogin}, http.MethodGet, "/user", nil, nil)
	if err != nil {
		return "", err
	}
	var user struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(resp.body, &user); err != nil || user.Login == "" {
		return "", &authority.Error{Authority: Name, Operation: operationLogin, Status: resp.status, Err: authority.ErrUnexpectedStatus, Detail: "response has no login"}
	}
	return user.Login, nil
}

// ensureKey returns the ID of the key on the account for the use, creating it when absent.
func (c *client) ensureKey(ctx context.Context, use authority.Use, authorized, title string) (string, error) {
	ep := endpoints[use]
	if id, found, err := c.findKey(ctx, use, ep, authorized); err != nil || found {
		return id, err
	}
	info := call{operation: operationCreate, use: use, permission: ep.writeScope}
	resp, err := c.do(ctx, info, http.MethodPost, ep.path, nil, map[string]string{"key": authorized, "title": title})
	if err != nil {
		var ae *authority.Error
		if errors.As(err, &ae) && errors.Is(err, authority.ErrKeyInUse) {
			// The key may have appeared on this account since the listing. Adopt it if so.
			if id, found, lerr := c.findKey(ctx, use, ep, authorized); lerr == nil && found {
				return id, nil
			}
			ae.Detail = joinDetail("the key is registered on another account", ae.Detail)
		}
		return "", err
	}
	var created remoteKey
	if err := json.Unmarshal(resp.body, &created); err != nil || created.ID == 0 {
		return "", &authority.Error{Authority: Name, Operation: operationCreate, Use: use, Status: resp.status, Err: authority.ErrUnexpectedStatus, Detail: "response has no key ID"}
	}
	return strconv.FormatInt(created.ID, 10), nil
}

// findKey looks for the key among the account's keys, following pagination.
func (c *client) findKey(ctx context.Context, use authority.Use, ep endpoint, authorized string) (string, bool, error) {
	info := call{operation: operationList, use: use, permission: ep.readScope}
	page := 1
	for n := 0; n < maxPages; n++ {
		query := url.Values{"per_page": {strconv.Itoa(perPage)}, "page": {strconv.Itoa(page)}}
		resp, err := c.do(ctx, info, http.MethodGet, ep.path, query, nil)
		if err != nil {
			return "", false, err
		}
		var keys []remoteKey
		if err := json.Unmarshal(resp.body, &keys); err != nil {
			return "", false, &authority.Error{Authority: Name, Operation: operationList, Use: use, Status: resp.status, Err: authority.ErrUnexpectedStatus, Detail: "response is not a list of keys"}
		}
		for _, k := range keys {
			if sameKey(k.Key, authorized) {
				return strconv.FormatInt(k.ID, 10), true, nil
			}
		}
		if page = nextPage(resp.header, page); page == 0 {
			return "", false, nil
		}
	}
	return "", false, &authority.Error{Authority: Name, Operation: operationList, Use: use, Err: authority.ErrUnexpectedStatus, Detail: maxPagesExceededBy}
}

// sameKey compares the key type and base64 body, ignoring the comment.
func sameKey(a, b string) bool {
	fa, fb := strings.Fields(a), strings.Fields(b)
	return len(fa) >= 2 && len(fb) >= 2 && fa[0] == fb[0] && fa[1] == fb[1]
}

// Revoke implements authority.Authority. It deletes the keys named in receipt.References and treats
// an ID that is already gone as success. Every reference is attempted; failures are joined.
func (a *Authority) Revoke(ctx context.Context, credentials map[string]string, receipt authority.Receipt) error {
	if len(receipt.References) == 0 {
		return nil
	}
	c, err := a.newClient("revoke", credentials)
	if err != nil {
		return err
	}
	uses := make([]authority.Use, 0, len(receipt.References))
	for use := range receipt.References {
		uses = append(uses, use)
	}
	sort.Slice(uses, func(i, j int) bool { return uses[i] < uses[j] })
	var errs []error
	for _, use := range uses {
		if err := c.deleteKey(ctx, use, receipt.References[use]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *client) deleteKey(ctx context.Context, use authority.Use, ref string) error {
	ep, ok := endpoints[use]
	if !ok {
		return &authority.Error{Authority: Name, Operation: operationDelete, Use: use, Err: authority.ErrUnsupportedUse}
	}
	id, err := strconv.ParseInt(ref, 10, 64)
	if err != nil || id <= 0 {
		return &authority.Error{Authority: Name, Operation: operationDelete, Use: use, Err: authority.ErrInvalidRequest, Detail: "reference is not a key ID"}
	}
	info := call{operation: operationDelete, use: use, permission: ep.writeScope}
	resp, err := c.do(ctx, info, http.MethodDelete, ep.path+"/"+strconv.FormatInt(id, 10), nil, nil)
	if err == nil {
		return nil
	}
	// GitHub answers 404 both for a missing key and for a token without the scope. The scope headers
	// tell them apart when they are present.
	if resp != nil && resp.status == http.StatusNotFound && !lacksScope(resp.header) {
		return nil
	}
	return err
}
