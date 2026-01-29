package cmd

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

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

var validKeys = []string{"ec256", "ec384", "rsa2048", "rsa3072"}

func validateKey(key string) error {
	if !slices.Contains(validKeys, key) {
		return fmt.Errorf("invalid --key type: '%s' [options: %s]", key, strings.Join(validKeys, ", "))
	}
	return nil
}

// FDOClientConfig contains global configuration options
type FDOClientConfig struct {
	Debug bool   `mapstructure:"debug"`
	Blob  string `mapstructure:"blob"`
	TPM   string `mapstructure:"tpm"`
	Key   string `mapstructure:"key"`
}

type DeviceInitConfig struct {
	ServerURL     string `mapstructure:"server-url"`
	KeyEnc        string `mapstructure:"key-enc"`
	DeviceInfo    string `mapstructure:"device-info"`
	DeviceInfoMac string `mapstructure:"device-info-mac"`
	InsecureTLS   bool   `mapstructure:"insecure-tls"`
	SerialNumber  string `mapstructure:"serial-number"`
}

type OnboardConfig struct {
	Kex                  string        `mapstructure:"kex"`
	Cipher               string        `mapstructure:"cipher"`
	Download             string        `mapstructure:"download"`
	EchoCommands         bool          `mapstructure:"echo-commands"`
	EnableInteropTest    bool          `mapstructure:"enable-interop-test"`
	InsecureTLS          bool          `mapstructure:"insecure-tls"`
	MaxServiceInfoSize   int           `mapstructure:"max-serviceinfo-size"`
	AllowCredentialReuse bool          `mapstructure:"allow-credential-reuse"`
	Resale               bool          `mapstructure:"resale"`
	TO2RetryDelay        time.Duration `mapstructure:"to2-retry-delay"`
	Upload               []string      `mapstructure:"upload"`
	WgetDir              string        `mapstructure:"wget-dir"`
}

type DeviceInitClientConfig struct {
	FDOClientConfig `mapstructure:",squash"`
	DeviceInit      DeviceInitConfig `mapstructure:"device-init"`
}

type OnboardClientConfig struct {
	FDOClientConfig `mapstructure:",squash"`
	Onboard         OnboardConfig `mapstructure:"onboard"`
}
