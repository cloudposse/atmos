package cmd

import "github.com/spf13/pflag"

func setStringFlagIfChanged(flags *pflag.FlagSet, name string, target *string) error {
	if flags.Changed(name) {
		val, err := flags.GetString(name)
		if err != nil {
			return err
		}
		*target = val
	}
	return nil
}

func setBoolFlagIfChanged(flags *pflag.FlagSet, name string, target *bool) error {
	if flags.Changed(name) {
		val, err := flags.GetBool(name)
		if err != nil {
			return err
		}
		*target = val
	}
	return nil
}

func setSliceOfStringsFlagIfChanged(flags *pflag.FlagSet, name string, target *[]string) error {
	if flags.Changed(name) {
		val, err := flags.GetStringSlice(name)
		if err != nil {
			return err
		}
		*target = val
	}
	return nil
}
