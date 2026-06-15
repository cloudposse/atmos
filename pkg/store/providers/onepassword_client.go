package providers

import (
	"context"
	"fmt"
	"strings"
	"sync"

	connect "github.com/1Password/connect-sdk-go/connect"
	connectop "github.com/1Password/connect-sdk-go/onepassword"
	onepassword "github.com/1password/onepassword-sdk-go"

	"github.com/cloudposse/atmos/pkg/store"
)

// opNotFoundMarkers are substrings the native SDK includes in resolve errors for missing
// vaults/items/fields. The SDK surfaces these as plain messages (no typed error), so a
// substring match is the only way to distinguish "not found" from auth/transport failures.
var opNotFoundMarkers = []string{
	"not found",
	"isn't a field",
	"no item",
	"couldn't find",
	"doesn't exist",
	"no matching",
}

// onePasswordClient abstracts the operations the store needs against 1Password, keyed by a
// secret reference ("op://vault/item[/section]/field"). Both the native SDK and 1Password
// Connect satisfy it, and tests inject an in-memory fake. Created items use the API Credential
// category and store the value in a Concealed field named after the reference's field segment.
type onePasswordClient interface {
	// Resolve returns the value the reference points to.
	Resolve(ctx context.Context, reference string) (string, error)
	// Set creates or updates the field the reference points to (creating the item if needed).
	Set(ctx context.Context, reference, value string) error
	// Delete removes the field the reference points to, deleting the item if it becomes empty.
	// It is idempotent: a missing vault/item/field is not an error.
	Delete(ctx context.Context, reference string) error
}

// opSDKAPI is the narrow subset of the native 1Password SDK that sdkClient depends on. It exists
// so sdkClient operates against an interface, letting tests inject an in-memory fake instead of
// the WASM-backed *onepassword.Client. The production adapter (sdkAPIAdapter) delegates to the
// real client's nested API (Secrets/Vaults/Items).
type opSDKAPI interface {
	Resolve(ctx context.Context, reference string) (string, error)
	ListVaults(ctx context.Context) ([]onepassword.VaultOverview, error)
	ListItems(ctx context.Context, vaultID string) ([]onepassword.ItemOverview, error)
	GetItem(ctx context.Context, vaultID, itemID string) (onepassword.Item, error)
	PutItem(ctx context.Context, item *onepassword.Item) error
	CreateItem(ctx context.Context, params *onepassword.ItemCreateParams) error
	DeleteItem(ctx context.Context, vaultID, itemID string) error
}

// sdkAPIAdapter adapts the native *onepassword.Client to opSDKAPI by flattening its nested API.
type sdkAPIAdapter struct {
	client *onepassword.Client
}

func (a sdkAPIAdapter) Resolve(ctx context.Context, reference string) (string, error) {
	return a.client.Secrets().Resolve(ctx, reference)
}

func (a sdkAPIAdapter) ListVaults(ctx context.Context) ([]onepassword.VaultOverview, error) {
	return a.client.Vaults().List(ctx)
}

func (a sdkAPIAdapter) ListItems(ctx context.Context, vaultID string) ([]onepassword.ItemOverview, error) {
	return a.client.Items().List(ctx, vaultID)
}

func (a sdkAPIAdapter) GetItem(ctx context.Context, vaultID, itemID string) (onepassword.Item, error) {
	return a.client.Items().Get(ctx, vaultID, itemID)
}

func (a sdkAPIAdapter) PutItem(ctx context.Context, item *onepassword.Item) error {
	_, err := a.client.Items().Put(ctx, *item)
	return err
}

func (a sdkAPIAdapter) CreateItem(ctx context.Context, params *onepassword.ItemCreateParams) error {
	_, err := a.client.Items().Create(ctx, *params)
	return err
}

func (a sdkAPIAdapter) DeleteItem(ctx context.Context, vaultID, itemID string) error {
	return a.client.Items().Delete(ctx, vaultID, itemID)
}

// sdkClient resolves references via the native 1Password SDK using a service-account token.
// The underlying client embeds a WASM core that is expensive to initialize, so it is built
// lazily on first use. Tests pre-seed api to bypass the WASM build entirely.
type sdkClient struct {
	token string

	once    sync.Once
	api     opSDKAPI
	initErr error
}

func newSDKClient(token string) *sdkClient {
	return &sdkClient{token: token}
}

// get lazily builds the underlying SDK API. When api is already set (tests), the build is skipped.
func (c *sdkClient) get(ctx context.Context) (opSDKAPI, error) {
	c.once.Do(func() {
		if c.api != nil {
			return
		}
		client, err := onepassword.NewClient(
			ctx,
			onepassword.WithServiceAccountToken(c.token),
			onepassword.WithIntegrationInfo(opIntegrationName, opIntegrationVersion),
		)
		if err != nil {
			c.initErr = err
			return
		}
		c.api = sdkAPIAdapter{client: client}
	})
	if c.initErr != nil {
		return nil, fmt.Errorf("%w: %w", store.ErrOnePasswordClientInit, c.initErr)
	}
	return c.api, nil
}

func (c *sdkClient) Resolve(ctx context.Context, reference string) (string, error) {
	api, err := c.get(ctx)
	if err != nil {
		return "", err
	}
	value, err := api.Resolve(ctx, reference)
	if err != nil {
		if isOPNotFound(err.Error()) {
			return "", store.ErrOnePasswordNotFound
		}
		return "", err
	}
	return value, nil
}

func (c *sdkClient) Set(ctx context.Context, reference, value string) error {
	api, err := c.get(ctx)
	if err != nil {
		return err
	}
	ref, err := parseOPReference(reference)
	if err != nil {
		return err
	}

	vaultID, found, err := c.resolveVaultID(ctx, api, ref.vault)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: vault %q", store.ErrOnePasswordNotFound, ref.vault)
	}

	itemID, found, err := c.resolveItemID(ctx, api, vaultID, ref.item)
	if err != nil {
		return err
	}
	if !found {
		return c.createItem(ctx, api, vaultID, ref, value)
	}
	return c.updateField(ctx, api, opItemLoc{vaultID: vaultID, itemID: itemID}, ref.field, value)
}

func (c *sdkClient) createItem(ctx context.Context, api opSDKAPI, vaultID string, ref opReference, value string) error {
	params := onepassword.ItemCreateParams{
		VaultID:  vaultID,
		Title:    ref.item,
		Category: onepassword.ItemCategoryAPICredentials,
		Fields: []onepassword.ItemField{{
			Title:     ref.field,
			FieldType: onepassword.ItemFieldTypeConcealed,
			Value:     value,
		}},
	}
	return api.CreateItem(ctx, &params)
}

func (c *sdkClient) updateField(ctx context.Context, api opSDKAPI, loc opItemLoc, field, value string) error {
	item, err := api.GetItem(ctx, loc.vaultID, loc.itemID)
	if err != nil {
		return err
	}
	if idx := indexOfSDKField(item.Fields, field); idx >= 0 {
		item.Fields[idx].Value = value
	} else {
		item.Fields = append(item.Fields, onepassword.ItemField{
			Title:     field,
			FieldType: onepassword.ItemFieldTypeConcealed,
			Value:     value,
		})
	}
	return api.PutItem(ctx, &item)
}

func (c *sdkClient) Delete(ctx context.Context, reference string) error {
	api, err := c.get(ctx)
	if err != nil {
		return err
	}
	ref, err := parseOPReference(reference)
	if err != nil {
		return err
	}
	// A missing vault or item means there is nothing to delete (idempotent).
	vaultID, found, err := c.resolveVaultID(ctx, api, ref.vault)
	if err != nil || !found {
		return err
	}
	itemID, found, err := c.resolveItemID(ctx, api, vaultID, ref.item)
	if err != nil || !found {
		return err
	}
	return c.deleteItemField(ctx, api, opItemLoc{vaultID: vaultID, itemID: itemID}, ref.field)
}

func (c *sdkClient) deleteItemField(ctx context.Context, api opSDKAPI, loc opItemLoc, field string) error {
	item, err := api.GetItem(ctx, loc.vaultID, loc.itemID)
	if err != nil {
		return err
	}
	idx := indexOfSDKField(item.Fields, field)
	if idx < 0 {
		return nil // field absent: idempotent.
	}
	item.Fields = append(item.Fields[:idx], item.Fields[idx+1:]...)
	if len(item.Fields) == 0 {
		return api.DeleteItem(ctx, loc.vaultID, loc.itemID)
	}
	return api.PutItem(ctx, &item)
}

// resolveVaultID finds a vault's ID by title (case-insensitive) or ID.
func (c *sdkClient) resolveVaultID(ctx context.Context, api opSDKAPI, vaultQuery string) (string, bool, error) {
	vaults, err := api.ListVaults(ctx)
	if err != nil {
		return "", false, err
	}
	for i := range vaults {
		if strings.EqualFold(vaults[i].Title, vaultQuery) || vaults[i].ID == vaultQuery {
			return vaults[i].ID, true, nil
		}
	}
	return "", false, nil
}

// resolveItemID finds an item's ID within a vault by title (case-insensitive) or ID.
func (c *sdkClient) resolveItemID(ctx context.Context, api opSDKAPI, vaultID, itemQuery string) (string, bool, error) {
	items, err := api.ListItems(ctx, vaultID)
	if err != nil {
		return "", false, err
	}
	for i := range items {
		if strings.EqualFold(items[i].Title, itemQuery) || items[i].ID == itemQuery {
			return items[i].ID, true, nil
		}
	}
	return "", false, nil
}

// opItemLoc identifies a resolved 1Password item by vault and item ID.
type opItemLoc struct {
	vaultID string
	itemID  string
}

// indexOfSDKField returns the index of the native-SDK field whose title or ID matches, or -1.
func indexOfSDKField(fields []onepassword.ItemField, field string) int {
	for i := range fields {
		if fieldMatches(fields[i].Title, fields[i].ID, field) {
			return i
		}
	}
	return -1
}

// connectClient resolves references against a 1Password Connect server. Connect has no native
// reference resolver, so the `op://vault/item[/section]/field` reference is parsed and resolved
// via the item/field REST API.
type connectClient struct {
	client connect.Client
}

func newConnectClient(host, token string) *connectClient {
	return &connectClient{client: connect.NewClientWithUserAgent(host, token, opIntegrationName+"/"+opIntegrationVersion)}
}

func (c *connectClient) Resolve(_ context.Context, reference string) (string, error) {
	ref, err := parseOPReference(reference)
	if err != nil {
		return "", err
	}

	opItem, err := c.client.GetItem(ref.item, ref.vault)
	if err != nil {
		if isOPNotFound(err.Error()) {
			return "", store.ErrOnePasswordNotFound
		}
		return "", err
	}

	for _, f := range opItem.Fields {
		if f != nil && fieldMatches(f.Label, f.ID, ref.field) {
			return f.Value, nil
		}
	}
	return "", store.ErrOnePasswordNotFound
}

func (c *connectClient) Set(_ context.Context, reference, value string) error {
	ref, err := parseOPReference(reference)
	if err != nil {
		return err
	}

	item, err := c.client.GetItem(ref.item, ref.vault)
	if err != nil {
		if isOPNotFound(err.Error()) {
			return c.createItem(ref, value)
		}
		return err
	}

	for _, f := range item.Fields {
		if f != nil && fieldMatches(f.Label, f.ID, ref.field) {
			f.Value = value
			_, err = c.client.UpdateItem(item, ref.vault)
			return err
		}
	}
	item.Fields = append(item.Fields, &connectop.ItemField{
		Label: ref.field,
		Type:  connectop.FieldTypeConcealed,
		Value: value,
	})
	_, err = c.client.UpdateItem(item, ref.vault)
	return err
}

func (c *connectClient) createItem(ref opReference, value string) error {
	vaultID, err := c.resolveVaultID(ref.vault)
	if err != nil {
		return err
	}
	item := &connectop.Item{
		Title:    ref.item,
		Vault:    connectop.ItemVault{ID: vaultID},
		Category: connectop.ApiCredential,
		Fields: []*connectop.ItemField{{
			Label: ref.field,
			Type:  connectop.FieldTypeConcealed,
			Value: value,
		}},
	}
	_, err = c.client.CreateItem(item, ref.vault)
	return err
}

func (c *connectClient) Delete(_ context.Context, reference string) error {
	ref, err := parseOPReference(reference)
	if err != nil {
		return err
	}

	item, err := c.client.GetItem(ref.item, ref.vault)
	if err != nil {
		if isOPNotFound(err.Error()) {
			return nil // idempotent.
		}
		return err
	}

	idx := -1
	for i, f := range item.Fields {
		if f != nil && fieldMatches(f.Label, f.ID, ref.field) {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil // field absent: idempotent.
	}
	item.Fields = append(item.Fields[:idx], item.Fields[idx+1:]...)
	if len(item.Fields) == 0 {
		return c.client.DeleteItem(item, ref.vault)
	}
	_, err = c.client.UpdateItem(item, ref.vault)
	return err
}

// resolveVaultID resolves a Connect vault query (title or UUID) to its UUID, needed when
// creating an item.
func (c *connectClient) resolveVaultID(vaultQuery string) (string, error) {
	if v, err := c.client.GetVault(vaultQuery); err == nil {
		return v.ID, nil
	}
	v, err := c.client.GetVaultByTitle(vaultQuery)
	if err != nil {
		return "", fmt.Errorf("%w: vault %q: %w", store.ErrOnePasswordNotFound, vaultQuery, err)
	}
	return v.ID, nil
}

// opReference is an `op://vault/item[/section]/field` reference split into its addressing parts.
type opReference struct {
	vault string
	item  string
	field string
}

// parseOPReference splits an `op://vault/item[/section]/field` reference. The optional section
// segment is not needed for field lookup (fields are matched by label/ID) and is ignored.
func parseOPReference(reference string) (opReference, error) {
	trimmed := strings.TrimPrefix(reference, opReferenceScheme)
	parts := strings.Split(trimmed, "/")
	if len(parts) < 3 || len(parts) > 4 {
		return opReference{}, fmt.Errorf("%w: %q", store.ErrOnePasswordInvalidReference, reference)
	}
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			return opReference{}, fmt.Errorf("%w: %q", store.ErrOnePasswordInvalidReference, reference)
		}
	}
	// parts is [vault, item, field] or [vault, item, section, field]; the field is always last.
	return opReference{vault: parts[0], item: parts[1], field: parts[len(parts)-1]}, nil
}

// fieldMatches reports whether a field's label or ID matches the reference's field segment.
func fieldMatches(label, id, field string) bool {
	return strings.EqualFold(label, field) || strings.EqualFold(id, field)
}

// isOPNotFound reports whether a 1Password error message indicates a missing vault/item/field.
func isOPNotFound(message string) bool {
	lower := strings.ToLower(message)
	for _, marker := range opNotFoundMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
