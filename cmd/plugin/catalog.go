package plugin

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/flags"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/ui"
)

const catalogFileMode = 0o644

const catalogOutputFlag = "output"

func catalogCommand() *cobra.Command {
	command := &cobra.Command{Use: "catalog", Short: "Build and maintain OCI plugin discovery catalogs"}
	command.AddCommand(catalogBuildCommand(), catalogPublishCommand(), catalogAddCommand(), catalogRemoveCommand())
	return command
}

func catalogBuildCommand() *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithStringFlag("name", "", "plugins", "Catalog name"),
		flags.WithStringFlag("source", "", "", "Release OCI repository"),
		flags.WithStringFlag(catalogOutputFlag, "o", "", "Catalog manifest output file"),
	)
	command := &cobra.Command{Use: "build <layout> --source <repository> --output <file>", Short: "Build a catalog from a verified release layout", Args: cobra.ExactArgs(1)}
	parser.RegisterFlags(command)
	_ = command.MarkFlagRequired("source")
	_ = command.MarkFlagRequired(catalogOutputFlag)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values := viper.New()
		if err := parser.BindFlagsToViper(cmd, values); err != nil {
			return err
		}
		manager, err := newManager(cmd)
		if err != nil {
			return err
		}
		index, err := atmosoci.LoadIndex(args[0], "")
		if err != nil {
			return err
		}
		release, err := pluginoci.Inspect(index, "")
		if err != nil {
			return err
		}
		file := catalog.New(values.GetString("name"))
		if err := catalog.Add(file, values.GetString("source"), release, manager.Installer.Descriptors); err != nil {
			return err
		}
		raw, err := yaml.Marshal(file)
		if err != nil {
			return err
		}
		if _, err := catalog.Parse(raw); err != nil {
			return err
		}
		if err := filesystem.NewOSFileSystem().WriteFileAtomic(values.GetString(catalogOutputFlag), raw, catalogFileMode); err != nil {
			return err
		}
		ui.Successf("Built catalog **%s** with %d package(s): `%s`.", file.Metadata.Name, len(file.Spec.Entries), values.GetString(catalogOutputFlag))
		return nil
	}
	return command
}

func catalogPublishCommand() *cobra.Command {
	parser := flags.NewStandardParser(flags.WithStringFlag("to", "", "", "Configured catalog alias or OCI reference"))
	command := &cobra.Command{Use: "publish <file> --to <catalog>", Short: "Publish catalog metadata with revision conflict checks", Args: cobra.ExactArgs(1)}
	parser.RegisterFlags(command)
	_ = command.MarkFlagRequired("to")
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values := viper.New()
		if err := parser.BindFlagsToViper(cmd, values); err != nil {
			return err
		}
		manager, err := newManager(cmd)
		if err != nil {
			return err
		}
		reference, err := catalogReference(values.GetString("to"))
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		file, err := catalog.Parse(raw)
		if err != nil {
			return err
		}
		_, digest, err := catalog.Read(cmd.Context(), manager.Installer.Registry, reference)
		if err != nil && !atmosoci.IsNotFound(err) {
			return err
		}
		if err := catalog.Publish(cmd.Context(), manager.Installer.Registry, reference, file, digest); err != nil {
			return err
		}
		ui.Successf("Published catalog **%s** to `%s`.", file.Metadata.Name, reference)
		return nil
	}
	return command
}

func catalogAddCommand() *cobra.Command {
	return &cobra.Command{Use: "add <catalog> <release-reference>", Short: "Add a verified release to a discovery catalog", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := newManager(cmd)
		if err != nil {
			return err
		}
		// Validate both arguments locally so a mistyped catalog or a missing tag or digest never
		// reaches the registry, where it would surface as an implicit "latest" lookup.
		target, err := catalogReference(args[0])
		if err != nil {
			return err
		}
		reference, err := catalog.ParseReleaseReference(args[1])
		if err != nil {
			return err
		}
		descriptor, err := manager.Installer.Registry.Get(cmd.Context(), reference)
		if err != nil {
			return err
		}
		index, err := descriptor.ImageIndex()
		if err != nil {
			return err
		}
		release, err := pluginoci.Inspect(index, descriptor.Digest.String())
		if err != nil {
			return err
		}
		return addToCatalog(cmd, manager, args[0], catalogSource{reference: target, source: "oci://" + reference.Context().Name()}, release)
	}}
}

func catalogRemoveCommand() *cobra.Command {
	parser := flags.NewStandardParser(flags.WithStringFlag("version", "", "", "Unlist only this version"))
	command := &cobra.Command{Use: "remove <catalog> <package>", Short: "Unlist a package or version without deleting artifacts", Args: cobra.ExactArgs(2)}
	parser.RegisterFlags(command)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values := viper.New()
		if err := parser.BindFlagsToViper(cmd, values); err != nil {
			return err
		}
		manager, err := newManager(cmd)
		if err != nil {
			return err
		}
		reference, err := catalogReference(args[0])
		if err != nil {
			return err
		}
		file, digest, err := catalog.Read(cmd.Context(), manager.Installer.Registry, reference)
		if err != nil {
			return err
		}
		if err := catalog.Remove(file, args[1], values.GetString("version")); err != nil {
			return err
		}
		if err := catalog.Publish(cmd.Context(), manager.Installer.Registry, reference, file, digest); err != nil {
			return err
		}
		if version := values.GetString("version"); version != "" {
			ui.Successf("Unlisted **%s** version `%s` from catalog `%s`.", args[1], catalog.NormalizeVersion(version), reference)
		} else {
			ui.Successf("Unlisted **%s** from catalog `%s`.", args[1], reference)
		}
		return nil
	}
	return command
}

func catalogReference(value string) (string, error) {
	return manage.ResolveCatalogReference(atmosConfig, value)
}

type catalogSource struct{ reference, source string }

func addToCatalog(cmd *cobra.Command, manager *manage.Manager, name string, location catalogSource, release *pluginoci.Release) error {
	reference, source := location.reference, location.source
	file, digest, err := catalog.Read(cmd.Context(), manager.Installer.Registry, reference)
	if atmosoci.IsNotFound(err) {
		file = catalog.New(name)
	} else if err != nil {
		return err
	}
	if err := catalog.Add(file, source, release, manager.Installer.Descriptors); err != nil {
		return err
	}
	if err := catalog.Publish(cmd.Context(), manager.Installer.Registry, reference, file, digest); err != nil {
		return err
	}
	ui.Successf("Catalog `%s` lists **%s** version `%s`.", reference, release.Name, catalog.NormalizeVersion(release.Version))
	return nil
}
