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

// FDOClientConfig contains global configuration options
type FDOClientConfig struct {
	Debug bool   `mapstructure:"debug"`
	Blob  string `mapstructure:"blob"`
	TPM   string `mapstructure:"tpm"`
}

// DeviceInitConfig contains device initialization specific configuration
type DeviceInitConfig struct {
	ServerURL     string `mapstructure:"server-url"`
	Key           string `mapstructure:"key"`
	KeyEnc        string `mapstructure:"key-enc"`
	DeviceInfo    string `mapstructure:"device-info"`
	DeviceInfoMac string `mapstructure:"device-info-mac"`
	InsecureTLS   bool   `mapstructure:"insecure-tls"`
}

// OnboardConfig contains onboarding specific configuration
type OnboardConfig struct {
	Key                  string        `mapstructure:"key"`
	Kex                  string        `mapstructure:"kex"`
	Cipher               string        `mapstructure:"cipher"`
	Download             string        `mapstructure:"download"`
	EchoCommands         bool          `mapstructure:"echo-commands"`
	InsecureTLS          bool          `mapstructure:"insecure-tls"`
	MaxServiceInfoSize   int           `mapstructure:"max-serviceinfo-size"`
	AllowCredentialReuse bool          `mapstructure:"allow-credential-reuse"`
	Resale               bool          `mapstructure:"resale"`
	TO2RetryDelay        time.Duration `mapstructure:"to2-retry-delay"`
	Upload               []string      `mapstructure:"upload"`
	WgetDir              string        `mapstructure:"wget-dir"`
}

// DeviceInitClientConfig is the full configuration file structure for device-init
type DeviceInitClientConfig struct {
	FDOClientConfig `mapstructure:",squash"`
	DeviceInit      DeviceInitConfig `mapstructure:"device-init"`
}

// OnboardClientConfig is the full configuration file structure for onboard
type OnboardClientConfig struct {
	FDOClientConfig `mapstructure:",squash"`
	Onboard         OnboardConfig `mapstructure:"onboard"`
}
