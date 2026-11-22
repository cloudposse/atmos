package aws

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_user_legacy_keyring_test.go -package=$GOPACKAGE

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	atmosCredentials "github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/interactive"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/ui"
)

// legacyKeyringAdoption describes an ambiguous previous-layout keyring entry that may hold the
// long-lived keys of an aws/user identity. It never carries the secret itself.
type legacyKeyringAdoption struct {
	// Identity is the aws/user identity name (the keyring alias).
	Identity string
	// Realm is the credential realm of the identity.
	Realm string
	// Key is the previous-layout keyring key, for example "atmos_acme_prod_admin".
	Key string
	// OtherOwners are the other realm and identity pairs that older versions stored under Key.
	OtherOwners []atmosCredentials.LegacyRealmOwner
}

// legacyKeyringPrompter asks the user whether an ambiguous legacy keyring entry belongs to an
// identity. It is an interface so tests never need a terminal.
type legacyKeyringPrompter interface {
	// Available reports whether an interactive prompt can be shown in this process.
	Available() bool
	// ConfirmAdoption asks whether to use the entry for the identity. It returns true only when
	// the user explicitly answers Yes; the default answer is No.
	ConfirmAdoption(req *legacyKeyringAdoption) (bool, error)
}

// huhLegacyKeyringPrompter is the production prompter: a huh confirmation drawn on stderr.
type huhLegacyKeyringPrompter struct{}

// Available reports whether stdin and stderr are terminals outside CI with interactive mode on.
func (huhLegacyKeyringPrompter) Available() bool {
	return interactive.Available()
}

// ConfirmAdoption shows the confirmation. Cancelling (ctrl+c, esc) returns ErrUserAborted.
func (huhLegacyKeyringPrompter) ConfirmAdoption(req *legacyKeyringAdoption) (bool, error) {
	if !interactive.Available() {
		return false, errUtils.ErrAuthPromptUnavailable
	}
	adopt := false // Default answer: No.
	confirm := uiutils.NewAtmosConfirm().
		Title(legacyKeyringPromptTitle(req)).
		Description(legacyKeyringPromptDescription(req)).
		Affirmative("Yes, use it").
		Negative("No").
		Value(&adopt)
	form := huh.NewForm(huh.NewGroup(confirm)).
		WithTheme(uiutils.NewAtmosHuhTheme()).
		WithKeyMap(uiutils.NewAtmosKeyMap())
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, errUtils.ErrUserAborted
		}
		return false, fmt.Errorf("%w: %w", errUtils.ErrPromptFailed, err)
	}
	return adopt, nil
}

// legacyKeyringPromptTitle is the question shown to the user.
func legacyKeyringPromptTitle(req *legacyKeyringAdoption) string {
	return fmt.Sprintf("Use the older keyring entry for identity %q?", req.Identity)
}

// legacyKeyringPromptDescription explains where the entry comes from and why it is not used
// automatically.
func legacyKeyringPromptDescription(req *legacyKeyringAdoption) string {
	return fmt.Sprintf("No AWS credentials are stored for identity %q in realm %q, but keyring key %q "+
		"was written by an older Atmos version and may hold them. The same key was also used for %s, "+
		"so the entry could belong to another realm. Choose Yes only if these are the credentials of %q "+
		"in realm %q; the entry is then moved to the new key.",
		req.Identity, req.Realm, req.Key, describeLegacyOwners(req.OtherOwners, false), req.Identity, req.Realm)
}

// describeLegacyOwners lists the other realm and identity pairs of a legacy key in prose. Names
// are quoted for the terminal prompt, or set in code spans for Markdown error text when code is true.
func describeLegacyOwners(owners []atmosCredentials.LegacyRealmOwner, code bool) string {
	if len(owners) == 0 {
		return "other realm and identity names that contain underscores"
	}
	format := "identity %q in realm %q"
	if code {
		format = "identity `%s` in realm `%s`"
	}
	parts := make([]string, 0, len(owners))
	for _, owner := range owners {
		parts = append(parts, fmt.Sprintf(format, owner.Alias, owner.Realm))
	}
	return strings.Join(parts, " or ")
}

var (
	// legacyKeyringAdoptionMu serializes the lookup, prompt, and adoption, so concurrent
	// authentications of the same identity (for example bulk Terraform runs) prompt at most once.
	legacyKeyringAdoptionMu sync.Mutex
	// declinedLegacyKeyringAdoptions records the realm and identity pairs whose legacy entry the
	// user declined in this process, so the question is not asked again during this invocation.
	declinedLegacyKeyringAdoptions sync.Map
)

// legacyKeyringDeclineKey identifies a realm and identity pair in declinedLegacyKeyringAdoptions.
func legacyKeyringDeclineKey(realm, identity string) string {
	return realm + "\x00" + identity
}

// legacyPrompterOrDefault returns the injected prompter or the production huh prompter.
func (i *userIdentity) legacyPrompterOrDefault() legacyKeyringPrompter {
	if i.legacyPrompter != nil {
		return i.legacyPrompter
	}
	return huhLegacyKeyringPrompter{}
}

// adoptAmbiguousLegacyKeyringEntry handles the case where no long-lived keys are stored under
// the identity's current keyring key but an older Atmos version may have stored them under an
// ambiguous previous-layout key (see credentials.AmbiguousLegacyRealmStore). Only aws/user
// long-lived keys get this treatment, because a login cannot recreate them; other identities
// simply log in again.
//
// It returns (nil, nil) when there is no such entry, so the caller keeps its usual not-found
// handling. Otherwise it asks the user, when a prompt can be shown, whether the entry is theirs:
// on Yes it moves the entry to the current key and returns the keys; on No, or when no prompt
// can be shown, it returns ErrAwsUserNotConfigured marked with ErrAwsUserLegacyKeyringEntryNotAdopted
// and hints that name the legacy key.
func (i *userIdentity) adoptAmbiguousLegacyKeyringEntry(ctx context.Context, store types.CredentialStore) (*types.AWSCredentials, error) {
	legacyStore, ok := store.(atmosCredentials.AmbiguousLegacyRealmStore)
	if !ok || i.realm == "" {
		return nil, nil
	}

	legacyKeyringAdoptionMu.Lock()
	defer legacyKeyringAdoptionMu.Unlock()

	entry, err := legacyStore.FindAmbiguousLegacyRealmEntry(i.name, i.realm)
	if err != nil {
		// Best effort: a failed lookup keeps the existing not-found behavior.
		log.Debug("Failed to look up legacy keyring entry", logKeyIdentity, i.name, "error", err)
		return nil, nil
	}
	if entry == nil || !isLongLivedAWSCredentials(entry.Credentials) {
		return nil, nil
	}
	req := &legacyKeyringAdoption{Identity: i.name, Realm: i.realm, Key: entry.Key, OtherOwners: entry.OtherOwners}

	declineKey := legacyKeyringDeclineKey(i.realm, i.name)
	if _, declined := declinedLegacyKeyringAdoptions.Load(declineKey); declined {
		return nil, i.legacyKeyringNotAdoptedError(req, false)
	}
	prompter := i.legacyPrompterOrDefault()
	if !types.AllowPrompts(ctx) || !prompter.Available() {
		return nil, i.legacyKeyringNotAdoptedError(req, true)
	}
	adopt, err := prompter.ConfirmAdoption(req)
	if errors.Is(err, errUtils.ErrAuthPromptUnavailable) {
		return nil, i.legacyKeyringNotAdoptedError(req, true)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: confirming the older keyring entry %q for identity %q: %w", errUtils.ErrAwsUserNotConfigured, req.Key, i.name, err)
	}
	if !adopt {
		declinedLegacyKeyringAdoptions.Store(declineKey, struct{}{})
		return nil, i.legacyKeyringNotAdoptedError(req, false)
	}

	if err := legacyStore.AdoptAmbiguousLegacyRealmEntry(i.name, i.realm); err != nil {
		return nil, fmt.Errorf("%w: identity %q: moving keyring key %q to the new layout: %w", errUtils.ErrAwsUserKeyringReadFailed, i.name, req.Key, err)
	}
	ui.Successf("Moved the AWS credentials of identity %q from keyring key %q to the new key layout", i.name, req.Key)
	// Read back through the current key: it holds the adopted entry, or newer credentials that
	// another process stored meanwhile.
	return i.credentialsFrom(store)
}

// isLongLivedAWSCredentials reports whether creds are complete long-lived AWS keys (not a
// session), the only kind of entry worth adopting for an aws/user identity.
func isLongLivedAWSCredentials(creds types.ICredentials) bool {
	awsCreds, ok := creds.(*types.AWSCredentials)
	return ok && awsCreds.AccessKeyID != "" && awsCreds.SecretAccessKey != "" && awsCreds.SessionToken == ""
}

// legacyKeyringNotAdoptedError builds the not-configured error returned when an ambiguous legacy
// entry exists but was not adopted. noPrompt is true when the entry could not be offered because
// no interactive prompt can be shown, and adds a hint to re-run interactively.
func (i *userIdentity) legacyKeyringNotAdoptedError(req *legacyKeyringAdoption, noPrompt bool) error {
	builder := errUtils.Build(errUtils.ErrAwsUserNotConfigured).
		WithCause(errUtils.ErrAwsUserLegacyKeyringEntryNotAdopted).
		WithExplanationf("AWS User credentials not found for identity `%s` in realm `%s`. Keyring key `%s` was written by an older Atmos version and may hold them, but the same key was also used for %s, so it is not used without confirmation",
			i.name, i.realm, req.Key, describeLegacyOwners(req.OtherOwners, true)).
		WithHintf("Run `atmos auth user configure --identity=%s` to store the credentials again", i.name)
	if noPrompt {
		builder = builder.WithHintf("Or re-run this command in an interactive terminal and confirm that keyring key `%s` belongs to identity `%s`", req.Key, i.name)
	}
	return builder.
		WithContext("identity", i.name).
		WithContext("realm", i.realm).
		WithContext("legacy_keyring_key", req.Key).
		Err()
}
