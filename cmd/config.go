package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func loadStringFromConfig(cmd *cobra.Command, flagName, viperKey string, target *string) {
	if !cmd.Flags().Changed(flagName) && viper.IsSet(viperKey) {
		*target = viper.GetString(viperKey)
	}
}

func loadBoolFromConfig(cmd *cobra.Command, flagName, viperKey string, target *bool) {
	if !cmd.Flags().Changed(flagName) && viper.IsSet(viperKey) {
		*target = viper.GetBool(viperKey)
	}
}

func loadIntFromConfig(cmd *cobra.Command, flagName, viperKey string, target *int) {
	if !cmd.Flags().Changed(flagName) && viper.IsSet(viperKey) {
		*target = viper.GetInt(viperKey)
	}
}

func loadDurationFromConfig(cmd *cobra.Command, flagName, viperKey string, target *time.Duration) {
	if !cmd.Flags().Changed(flagName) && viper.IsSet(viperKey) {
		*target = viper.GetDuration(viperKey)
	}
}

func bindFlags(cmd *cobra.Command, prefix string) error {
	var bindErr error
	cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		viperKey := fmt.Sprintf("%s.%s", prefix, flag.Name)
		if err := viper.BindPFlag(viperKey, flag); err != nil {
			bindErr = err
			return
		}
	})
	return bindErr
}
