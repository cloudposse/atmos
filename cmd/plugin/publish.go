package plugin

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/flags"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	publishDestinationFlag = "to"
	signFlag               = "sign"
	yesFlag                = "yes"
	verifyPublicFlag       = "verify-public"
)

func publishCommand() *cobra.Command {
	options := publicationFlags()
	parser := newOutputParser(options...)
	command := &cobra.Command{Use: "publish <layout> --to <repository>", Short: "Publish a verified plugin release", Args: cobra.ExactArgs(1)}
	registerOutputFlags(command, parser)
	_ = command.MarkFlagRequired(publishDestinationFlag)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values, err := parseOutputFlags(cmd, parser)
		if err != nil {
			return err
		}
		// Pre-flight everything that can be checked locally before the first registry write.
		if err := rejectOrphanSigningFlags(cmd, values); err != nil {
			return err
		}
		if _, err := pluginoci.ParseDestination(values.GetString(publishDestinationFlag)); err != nil {
			return err
		}
		manager, err := newManager(cmd)
		if err != nil {
			return err
		}
		release, err := publishLayout(cmd.Context(), manager, args[0], values)
		if err != nil {
			return err
		}
		return presentPublishedRelease(release, values)
	}
	return command
}

// publishLayout shares signing and public-access checks between both release entry points.
func publishLayout(ctx context.Context, manager *manage.Manager, layout string, values *viper.Viper) (*pluginoci.Release, error) {
	repository := values.GetString(publishDestinationFlag)
	access := newPublicVerifier()
	if values.GetBool(verifyPublicFlag) {
		if err := access.RequirePublic(ctx, repository); err != nil {
			return nil, err
		}
	}
	release, err := pluginoci.Publish(ctx, manager.Installer.Registry, layout, repository, manager.Installer.Descriptors, pluginoci.WithExecutableFormatCheck())
	if err != nil {
		return nil, err
	}
	reference := strings.TrimPrefix(repository, "oci://") + "@" + release.Digest
	if values.GetBool(signFlag) {
		if _, err := signRelease(ctx, manager, reference, values); err != nil {
			return nil, fmt.Errorf("release %s was published but signing failed; retry with atmos plugin sign: %w", reference, err)
		}
	}
	if values.GetBool(verifyPublicFlag) {
		if _, err := access.Verify(ctx, repository, release.Digest, release.Digest); err != nil {
			return nil, fmt.Errorf("release %s was published but public access verification failed: %w", reference, err)
		}
	}
	return release, nil
}

// rejectOrphanSigningFlags fails when a signing-only flag is set without --sign,
// instead of silently ignoring it.
func rejectOrphanSigningFlags(cmd *cobra.Command, values *viper.Viper) error {
	if values.GetBool(signFlag) {
		return nil
	}
	for _, flag := range []string{cosignVersionFlag, yesFlag} {
		if cmd.Flags().Changed(flag) {
			return errUtils.Build(fmt.Errorf("%w: --%s only applies when signing", errUtils.ErrPluginFlagConflict, flag)).
				WithHintf("Add `--sign` to sign the release, or remove `--%s`.", flag).
				Err()
		}
	}
	return nil
}

// deletePrompts are the seams that decide whether and how deletion is confirmed.
type deletePrompts struct {
	interactive func() bool
	confirm     func(title, description string) (bool, error)
}

func defaultDeletePrompts() deletePrompts {
	return deletePrompts{interactive: flags.IsInteractive, confirm: confirmWithPrompt}
}

func confirmWithPrompt(title, description string) (bool, error) {
	var confirmed bool
	prompt := uiutils.NewAtmosConfirm().
		Title(title).
		Description(description).
		Affirmative("Yes, delete").
		Negative("No, keep it").
		Value(&confirmed).
		WithTheme(uiutils.NewAtmosHuhTheme())
	if err := prompt.Run(); err != nil {
		return false, fmt.Errorf("confirmation prompt failed: %w", err)
	}
	return confirmed, nil
}

func deleteCommand() *cobra.Command {
	parser := flags.NewStandardParser(flags.WithBoolFlag(yesFlag, "y", false, "Delete without asking for confirmation"))
	command := &cobra.Command{Use: "delete <repository@sha256:digest>", Short: "Delete a selected remote release digest", Args: cobra.ExactArgs(1)}
	parser.RegisterFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values := viper.New()
		if err := parser.BindFlagsToViper(cmd, values); err != nil {
			return err
		}
		reference, err := pluginoci.ParseDigestReference(args[0])
		if err != nil {
			return err
		}
		manager, err := newManager(cmd)
		if err != nil {
			return err
		}
		return deleteRelease(cmd.Context(), manager.Installer.Registry, reference, values.GetBool(yesFlag), defaultDeletePrompts())
	}
	return command
}

func confirmationRequired(reference string) error {
	return errUtils.Build(fmt.Errorf("%w: deleting %s is irreversible", errUtils.ErrPluginConfirmationRequired, reference)).
		WithHint("Pass `--yes` to delete without a prompt, or run the command in an interactive terminal to confirm.").
		Err()
}

// deleteRelease removes one digest after confirming, unless yes skips the prompt.
func deleteRelease(ctx context.Context, client atmosoci.Registry, reference name.Digest, yes bool, prompts deletePrompts) error {
	if !yes && !prompts.interactive() {
		return confirmationRequired(reference.Name())
	}
	target := pluginoci.DescribeDeletion(ctx, client, reference)
	if !yes {
		confirmed, err := prompts.confirm("Delete this release from the registry?", describeDeletion(&target))
		if err != nil {
			return err
		}
		if !confirmed {
			ui.Info("Deletion cancelled; the release was not changed.")
			return nil
		}
	}
	if err := client.Delete(ctx, reference); err != nil {
		return err
	}
	ui.Successf("Deleted **%s**%s.", reference.Name(), describeTags(target.Tags))
	return nil
}

func describeDeletion(target *pluginoci.DeletionTarget) string {
	tags := "none found"
	if len(target.Tags) != 0 {
		tags = strings.Join(target.Tags, ", ") + " (these tags stop resolving)"
	}
	return fmt.Sprintf("Repository: %s\nDigest: %s\nTags: %s", target.Repository, target.Digest, tags)
}

func describeTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return " (tags: " + strings.Join(tags, ", ") + ")"
}
