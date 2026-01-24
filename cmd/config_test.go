package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type TestFullConfig struct {
	FDOClientConfig  `mapstructure:",squash"`
	DeviceInitConfig `mapstructure:"device-init"`
	OnboardConfig    `mapstructure:"onboard"`
}

var capturedConfig *TestFullConfig

func resetState(t *testing.T) {
	t.Helper()
	viper.Reset()

	for _, cmd := range []*cobra.Command{rootCmd, onboardCmd, deviceInitCmd} {
		cmd.ResetFlags()
		cmd.ResetCommands()
		cmd.SetArgs(nil)
	}

	configFile = ""
	rootConfig = FDOClientConfig{}
	diConf = DeviceInitClientConfig{}
	onboardConfig = OnboardClientConfig{}

	rootCmdInit()
	onboardCmdInit()
	deviceInitCmdInit()
	capturedConfig = nil
}

func stubRunE(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	orig := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		var fdoConfig TestFullConfig

		if err := viper.Unmarshal(&fdoConfig); err != nil {
			return err
		}

		capturedConfig = &fdoConfig

		return fdoConfig.validate()
	}
	t.Cleanup(func() { cmd.RunE = orig })
}

func writeConfig(t *testing.T, contents, ext string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config."+ext)
	if err := os.WriteFile(p, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func runTest(t *testing.T, cmd *cobra.Command, config, format string, args ...string) error {
	t.Helper()
	resetState(t)

	stubRunE(t, cmd)
	path := writeConfig(t, config, format)

	cmdArgs := append([]string{cmd.Name(), "--config", path}, args...)

	rootCmd.SetArgs(cmdArgs)
	return rootCmd.Execute()
}

func runCLI(t *testing.T, cmd *cobra.Command, args ...string) error {
	t.Helper()
	resetState(t)
	stubRunE(t, cmd)
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

func runTestBothFormats(t *testing.T, name string, cmd *cobra.Command, toml, yaml string, expectErr bool, args ...string) {
	t.Helper()
	for _, tc := range []struct{ name, config, ext string }{
		{name + "/TOML", toml, "toml"},
		{name + "/YAML", yaml, "yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := runTest(t, cmd, tc.config, tc.ext, args...)
			if expectErr && err == nil {
				t.Fatal("expected error")
			}
			if !expectErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidation_RequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		command *cobra.Command
		toml    string
		yaml    string
	}{
		{"DI: missing blob/tpm", deviceInitCmd,
			`key = "ec384"` + "\n[device-init]\nserver-url = \"https://127.0.0.1:8080\"",
			"key: ec384\ndevice-init:\n  server-url: https://127.0.0.1:8080"},
		{"DI: missing key", deviceInitCmd,
			`blob = "cred.bin"` + "\n[device-init]\nserver-url = \"https://127.0.0.1:8080\"",
			"blob: cred.bin\ndevice-init:\n  server-url: https://127.0.0.1:8080"},
		{"DI: missing server-url", deviceInitCmd,
			`blob = "cred.bin"` + "\nkey = \"ec384\"\n[device-init]",
			"blob: cred.bin\nkey: ec384\ndevice-init: {}"},
		{"OB: missing blob/tpm", onboardCmd,
			`key = "ec384"` + "\n[onboard]\nkex = \"ECDH256\"\ncipher = \"A128GCM\"",
			"key: ec384\nonboard:\n  kex: ECDH256\n  cipher: A128GCM"},
		{"OB: missing key", onboardCmd,
			`blob = "cred.bin"` + "\n[onboard]\nkex = \"ECDH256\"\ncipher = \"A128GCM\"",
			"blob: cred.bin\nonboard:\n  kex: ECDH256\n  cipher: A128GCM"},
		{"OB: missing kex", onboardCmd,
			`blob = "cred.bin"` + "\nkey = \"ec384\"\n[onboard]\ncipher = \"A128GCM\"",
			"blob: cred.bin\nkey: ec384\nonboard:\n  cipher: A128GCM"},
	}
	for _, tt := range tests {
		runTestBothFormats(t, tt.name, tt.command, tt.toml, tt.yaml, true)
	}
}

func TestValidation_InvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		command *cobra.Command
		toml    string
		yaml    string
	}{
		{"invalid key type", deviceInitCmd,
			`blob = "cred.bin"` + "\nkey = \"invalid\"\n[device-init]\nserver-url = \"https://127.0.0.1:8080\"",
			"blob: cred.bin\nkey: invalid\ndevice-init:\n  server-url: https://127.0.0.1:8080"},
		{"invalid key-enc", deviceInitCmd,
			`blob = "cred.bin"` + "\nkey = \"ec384\"\n[device-init]\nserver-url = \"https://127.0.0.1:8080\"\nkey-enc = \"bad\"",
			"blob: cred.bin\nkey: ec384\ndevice-init:\n  server-url: https://127.0.0.1:8080\n  key-enc: bad"},
		{"invalid URL", deviceInitCmd,
			`blob = "cred.bin"` + "\nkey = \"ec384\"\n[device-init]\nserver-url = \"not-a-url\"",
			"blob: cred.bin\nkey: ec384\ndevice-init:\n  server-url: not-a-url"},
		{"invalid kex", onboardCmd,
			`blob = "cred.bin"` + "\nkey = \"ec384\"\n[onboard]\nkex = \"BAD_KEX\"\ncipher = \"A128GCM\"",
			"blob: cred.bin\nkey: ec384\nonboard:\n  kex: BAD_KEX\n  cipher: A128GCM"},
		{"invalid cipher", onboardCmd,
			`blob = "cred.bin"` + "\nkey = \"ec384\"\n[onboard]\nkex = \"ECDH256\"\ncipher = \"BAD\"",
			"blob: cred.bin\nkey: ec384\nonboard:\n  kex: ECDH256\n  cipher: BAD"},
		{"invalid max-serviceinfo-size", onboardCmd,
			`blob = "cred.bin"` + "\nkey = \"ec384\"\n[onboard]\nkex = \"ECDH256\"\ncipher = \"A128GCM\"\nmax-serviceinfo-size = 99999",
			"blob: cred.bin\nkey: ec384\nonboard:\n  kex: ECDH256\n  cipher: A128GCM\n  max-serviceinfo-size: 99999"},
	}
	for _, tt := range tests {
		runTestBothFormats(t, tt.name, tt.command, tt.toml, tt.yaml, true)
	}
}

func TestDeviceInit_MutuallyExclusiveDeviceInfo(t *testing.T) {
	t.Run("config file: both device-info and device-info-mac specified", func(t *testing.T) {
		toml := `blob = "cred.bin"
key = "ec384"

[device-init]
server-url = "https://127.0.0.1:8080"
device-info = "custom-device"
device-info-mac = "eth0"`

		yaml := `blob: cred.bin
key: ec384
device-init:
  server-url: https://127.0.0.1:8080
  device-info: custom-device
  device-info-mac: eth0`

		runTestBothFormats(t, "both specified", deviceInitCmd, toml, yaml, true)
	})

	t.Run("config file: only device-info specified", func(t *testing.T) {
		yaml := `blob: cred.bin
key: ec384
device-init:
  server-url: https://127.0.0.1:8080
  device-info: custom-device`

		if err := runTest(t, deviceInitCmd, yaml, "yaml"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got, want := capturedConfig.DeviceInitConfig.DeviceInfo, "custom-device"; got != want {
			t.Errorf("DeviceInfo = %q, want %q", got, want)
		}
		if got := capturedConfig.DeviceInitConfig.DeviceInfoMac; got != "" {
			t.Errorf("DeviceInfoMac = %q, want empty", got)
		}
	})

	t.Run("config file: only device-info-mac specified", func(t *testing.T) {
		yaml := `blob: cred.bin
key: ec384
device-init:
  server-url: https://127.0.0.1:8080
  device-info-mac: eth0`

		if err := runTest(t, deviceInitCmd, yaml, "yaml"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got := capturedConfig.DeviceInitConfig.DeviceInfo; got != "" {
			t.Errorf("DeviceInfo = %q, want empty", got)
		}
		if got, want := capturedConfig.DeviceInitConfig.DeviceInfoMac, "eth0"; got != want {
			t.Errorf("DeviceInfoMac = %q, want %q", got, want)
		}
	})

	t.Run("CLI: both device-info and device-info-mac specified", func(t *testing.T) {
		err := runCLI(t, deviceInitCmd, "device-init", "https://127.0.0.1:8080",
			"--blob", "cred.bin", "--key", "ec384",
			"--device-info", "custom-device", "--device-info-mac", "eth0")

		if err == nil {
			t.Fatal("expected error when both device-info and device-info-mac are specified")
		}

		expectedErr := "can't specify both --device-info and --device-info-mac"
		if !strings.Contains(err.Error(), expectedErr) {
			t.Errorf("error = %q, want error containing %q", err.Error(), expectedErr)
		}
	})

	t.Run("CLI: only device-info specified", func(t *testing.T) {
		if err := runCLI(t, deviceInitCmd, "device-init", "https://127.0.0.1:8080",
			"--blob", "cred.bin", "--key", "ec384", "--device-info", "cli-device"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got, want := capturedConfig.DeviceInitConfig.DeviceInfo, "cli-device"; got != want {
			t.Errorf("DeviceInfo = %q, want %q", got, want)
		}
	})

	t.Run("CLI: only device-info-mac specified", func(t *testing.T) {
		if err := runCLI(t, deviceInitCmd, "device-init", "https://127.0.0.1:8080",
			"--blob", "cred.bin", "--key", "ec384", "--device-info-mac", "eth0"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got, want := capturedConfig.DeviceInitConfig.DeviceInfoMac, "eth0"; got != want {
			t.Errorf("DeviceInfoMac = %q, want %q", got, want)
		}
	})

	t.Run("CLI overrides config device-info", func(t *testing.T) {
		yaml := `blob: cred.bin
key: ec384
device-init:
  server-url: https://127.0.0.1:8080
  device-info: config-device`

		resetState(t)
		stubRunE(t, deviceInitCmd)
		path := writeConfig(t, yaml, "yaml")

		rootCmd.SetArgs([]string{"device-init", "--config", path, "--device-info", "cli-device"})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got, want := capturedConfig.DeviceInitConfig.DeviceInfo, "cli-device"; got != want {
			t.Errorf("DeviceInfo = %q, want %q (CLI should override config)", got, want)
		}
	})

	t.Run("config device-info + CLI device-info-mac", func(t *testing.T) {
		yaml := `blob: cred.bin
key: ec384
device-init:
  server-url: https://127.0.0.1:8080
  device-info: config-device`

		resetState(t)
		stubRunE(t, deviceInitCmd)
		path := writeConfig(t, yaml, "yaml")

		rootCmd.SetArgs([]string{"device-init", "--config", path, "--device-info-mac", "eth0"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error when device-info in config and device-info-mac in CLI")
		}

		expectedErr := "can't specify both --device-info and --device-info-mac"
		if !strings.Contains(err.Error(), expectedErr) {
			t.Errorf("error = %q, want error containing %q", err.Error(), expectedErr)
		}
	})

	t.Run("config device-info-mac + CLI device-info", func(t *testing.T) {
		yaml := `blob: cred.bin
key: ec384
device-init:
  server-url: https://127.0.0.1:8080
  device-info-mac: eth0`

		resetState(t)
		stubRunE(t, deviceInitCmd)
		path := writeConfig(t, yaml, "yaml")

		rootCmd.SetArgs([]string{"device-init", "--config", path, "--device-info", "cli-device"})

		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("expected error when device-info-mac in config and device-info in CLI")
		}

		expectedErr := "can't specify both --device-info and --device-info-mac"
		if !strings.Contains(err.Error(), expectedErr) {
			t.Errorf("error = %q, want error containing %q", err.Error(), expectedErr)
		}
	})
}

func TestValidation_InvalidConfigFile(t *testing.T) {
	for _, cmd := range []*cobra.Command{deviceInitCmd, onboardCmd} {
		t.Run(cmd.Name(), func(t *testing.T) {
			resetState(t)
			stubRunE(t, cmd)

			rootCmd.SetArgs([]string{cmd.Name(), "--config", "/no/such/file.toml"})

			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("expected error for missing config file, got nil")
			}
		})
	}
}

func TestValidation_MalformedConfig(t *testing.T) {
	t.Run("TOML", func(t *testing.T) {
		err := runTest(t, deviceInitCmd, "this is [[[ not valid", "toml")
		if err == nil {
			t.Fatalf("expected error for malformed TOML, got nil")
		}
	})
	t.Run("YAML", func(t *testing.T) {
		err := runTest(t, onboardCmd, "bad:\n  indent\n    broken:", "yaml")
		if err == nil {
			t.Fatalf("expected error for malformed YAML, got nil")
		}
	})
}

func TestDeviceInit_ConfigFileLoading(t *testing.T) {
	toml := `debug = true
blob = "cred.bin"
key = "ec384"

[device-init]
server-url = "https://example.com:8080"
key-enc = "cose"
device-info = "dev-1"
insecure-tls = true
serial-number = "serial123"`

	yaml := `debug: true
blob: cred.bin
key: ec384
device-init:
  server-url: https://example.com:8080
  key-enc: cose
  device-info: dev-1
  insecure-tls: true
  serial-number: serial123`

	runTestBothFormats(t, "", deviceInitCmd, toml, yaml, false)

	if got, want := capturedConfig.Debug, true; got != want {
		t.Errorf("Debug = %v, want %v", got, want)
	}
	if got, want := capturedConfig.DeviceInitConfig.KeyEnc, "cose"; got != want {
		t.Errorf("KeyEnc = %q, want %q", got, want)
	}
	if got, want := capturedConfig.DeviceInitConfig.DeviceInfo, "dev-1"; got != want {
		t.Errorf("DeviceInfo = %q, want %q", got, want)
	}
	if got, want := capturedConfig.DeviceInitConfig.InsecureTLS, true; got != want {
		t.Errorf("InsecureTLS = %v, want %v", got, want)
	}
	if got, want := capturedConfig.DeviceInitConfig.SerialNumber, "serial123"; got != want {
		t.Errorf("SerialNumber = %v, want %v", got, want)
	}
}

func TestOnboard_ConfigFileLoading(t *testing.T) {
	toml := `debug = true
blob = "cred.bin"
key = "ec384"

[onboard]
kex = "ECDH384"
cipher = "A256GCM"
echo-commands = true
insecure-tls = true
max-serviceinfo-size = 2000
allow-credential-reuse = true
resale = true`

	yaml := `debug: true
blob: cred.bin
key: ec384
onboard:
  kex: ECDH384
  cipher: A256GCM
  echo-commands: true
  insecure-tls: true
  max-serviceinfo-size: 2000
  allow-credential-reuse: true
  resale: true`

	runTestBothFormats(t, "all options", onboardCmd, toml, yaml, false)

	o := capturedConfig.OnboardConfig
	if got, want := o.Kex, "ECDH384"; got != want {
		t.Errorf("Kex = %q, want %q", got, want)
	}
	if got, want := o.Cipher, "A256GCM"; got != want {
		t.Errorf("Cipher = %q, want %q", got, want)
	}
	if got, want := o.EchoCommands, true; got != want {
		t.Errorf("EchoCommands = %v, want %v", got, want)
	}
	if got, want := o.MaxServiceInfoSize, 2000; got != want {
		t.Errorf("MaxServiceInfoSize = %d, want %d", got, want)
	}

	toml = `blob = "cred.bin"
key = "ec384"

[onboard]
kex = "ECDH256"
cipher = "A128GCM"
to2-retry-delay = "30s"`

	yaml = `blob: cred.bin
key: ec384
onboard:
  kex: ECDH256
  cipher: A128GCM
  to2-retry-delay: 30s`

	runTestBothFormats(t, "TO2RetryDelay", onboardCmd, toml, yaml, false)

	if got, want := capturedConfig.OnboardConfig.TO2RetryDelay, 30*time.Second; got != want {
		t.Errorf("TO2RetryDelay = %v, want %v", got, want)
	}
}

func TestDeviceInit_CLIOnly(t *testing.T) {
	if err := runCLI(t, deviceInitCmd, "device-init", "https://127.0.0.1:8080", "--blob", "cred.bin", "--key",
		"ec384", "--key-enc", "cose", "--serial-number", "serial456"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := capturedConfig.Blob, "cred.bin"; got != want {
		t.Errorf("Blob = %q, want %q", got, want)
	}
	if got, want := capturedConfig.DeviceInitConfig.KeyEnc, "cose"; got != want {
		t.Errorf("KeyEnc = %q, want %q", got, want)
	}
	if got, want := capturedConfig.DeviceInitConfig.SerialNumber, "serial456"; got != want {
		t.Errorf("SerialNumber = %q, want %q", got, want)
	}
}

func TestOnboard_CLIOnly(t *testing.T) {
	if err := runCLI(t, onboardCmd, "onboard", "--blob", "cred.bin", "--key", "ec384", "--kex", "ECDH256", "--cipher", "A256GCM"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := capturedConfig.OnboardConfig.Kex, "ECDH256"; got != want {
		t.Errorf("Kex = %q, want %q", got, want)
	}
	if got, want := capturedConfig.OnboardConfig.Cipher, "A256GCM"; got != want {
		t.Errorf("Cipher = %q, want %q", got, want)
	}
}

func TestDeviceInit_CLIOverridesConfig(t *testing.T) {
	toml := `blob = "config.bin"
key = "ec256"
debug = false

[device-init]
server-url = "https://config.com:8080"
key-enc = "x509"
serial-number = "notthisserial1"`

	yaml := `blob: config.bin
key: ec256
debug: false
device-init:
  server-url: https://config.com:8080
  key-enc: x509
  serial-number: notthisserial1`

	runTestBothFormats(t, "", deviceInitCmd, toml, yaml, false,
		"https://cli.com:9090", "--blob", "cli.bin", "--key", "ec384", "--debug", "--key-enc", "cose",
		"--insecure-tls", "--serial-number", "serial100")

	checks := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"Blob", capturedConfig.Blob, "cli.bin"},
		{"Key", capturedConfig.Key, "ec384"},
		{"Debug", capturedConfig.Debug, true},
		{"ServerURL", capturedConfig.DeviceInitConfig.ServerURL, "https://cli.com:9090"},
		{"KeyEnc", capturedConfig.DeviceInitConfig.KeyEnc, "cose"},
		{"InsecureTLS", capturedConfig.DeviceInitConfig.InsecureTLS, true},
		{"SerialNumber", capturedConfig.DeviceInitConfig.SerialNumber, "serial100"},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestOnboard_CLIOverridesConfig(t *testing.T) {
	toml := `blob = "config.bin"
key = "ec256"

[onboard]
kex = "ECDH256"
cipher = "A128GCM"
max-serviceinfo-size = 1300`

	yaml := `blob: config.bin
key: ec256
onboard:
  kex: ECDH256
  cipher: A128GCM
  max-serviceinfo-size: 1300`

	runTestBothFormats(t, "", onboardCmd, toml, yaml, false,
		"--blob", "cli.bin", "--key", "ec384", "--kex", "ECDH384", "--cipher", "A256GCM",
		"--max-serviceinfo-size", "2000", "--echo-commands", "--resale")

	o := capturedConfig.OnboardConfig
	checks := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"Blob", capturedConfig.Blob, "cli.bin"},
		{"Kex", o.Kex, "ECDH384"},
		{"Cipher", o.Cipher, "A256GCM"},
		{"MaxServiceInfoSize", o.MaxServiceInfoSize, 2000},
		{"EchoCommands", o.EchoCommands, true},
		{"Resale", o.Resale, true},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestDeviceInit_CLIAndConfigWithDefaults(t *testing.T) {
	yaml := `blob: cred.bin
key: ec384
device-init:
  server-url: https://example.com:8080`

	resetState(t)
	stubRunE(t, deviceInitCmd)
	path := writeConfig(t, yaml, "yaml")

	// CLI provides debug and device-info, but NOT key-enc or insecure-tls
	rootCmd.SetArgs([]string{"device-init", "--config", path, "--debug", "--device-info", "test-device"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify CLI values
	if got, want := capturedConfig.Debug, true; got != want {
		t.Errorf("Debug = %v, want %v (from CLI)", got, want)
	}
	if got, want := capturedConfig.DeviceInitConfig.DeviceInfo, "test-device"; got != want {
		t.Errorf("DeviceInfo = %q, want %q (from CLI)", got, want)
	}

	// Verify config values
	if got, want := capturedConfig.Blob, "cred.bin"; got != want {
		t.Errorf("Blob = %q, want %q (from config)", got, want)
	}
	if got, want := capturedConfig.DeviceInitConfig.ServerURL, "https://example.com:8080"; got != want {
		t.Errorf("ServerURL = %q, want %q (from config)", got, want)
	}

	// Verify defaults (not in config or CLI)
	if got, want := capturedConfig.DeviceInitConfig.KeyEnc, "x509"; got != want {
		t.Errorf("KeyEnc = %q, want %q (DEFAULT)", got, want)
	}
	if got, want := capturedConfig.DeviceInitConfig.InsecureTLS, false; got != want {
		t.Errorf("InsecureTLS = %v, want %v (DEFAULT)", got, want)
	}
	if got, want := capturedConfig.DeviceInitConfig.DeviceInfoMac, ""; got != want {
		t.Errorf("DeviceInfoMac = %q, want %q (DEFAULT)", got, want)
	}
}

func TestOnboard_CLIAndConfigWithDefaults(t *testing.T) {
	yaml := `blob: cred.bin
key: ec384
onboard:
  kex: ECDH256`

	resetState(t)
	stubRunE(t, onboardCmd)
	path := writeConfig(t, yaml, "yaml")

	// CLI provides echo-commands and max-serviceinfo-size, but NOT cipher, insecure-tls, resale
	rootCmd.SetArgs([]string{"onboard", "--config", path, "--echo-commands", "--max-serviceinfo-size", "2000"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	o := capturedConfig.OnboardConfig

	// Verify CLI values
	if got, want := o.EchoCommands, true; got != want {
		t.Errorf("EchoCommands = %v, want %v (from CLI)", got, want)
	}
	if got, want := o.MaxServiceInfoSize, 2000; got != want {
		t.Errorf("MaxServiceInfoSize = %d, want %d (from CLI)", got, want)
	}

	// Verify config values
	if got, want := capturedConfig.Blob, "cred.bin"; got != want {
		t.Errorf("Blob = %q, want %q (from config)", got, want)
	}
	if got, want := o.Kex, "ECDH256"; got != want {
		t.Errorf("Kex = %q, want %q (from config)", got, want)
	}

	// Verify defaults (not in config or CLI)
	if got, want := o.Cipher, "A128GCM"; got != want {
		t.Errorf("Cipher = %q, want %q (DEFAULT)", got, want)
	}
	if got, want := o.InsecureTLS, false; got != want {
		t.Errorf("InsecureTLS = %v, want %v (DEFAULT)", got, want)
	}
	if got, want := o.Resale, false; got != want {
		t.Errorf("Resale = %v, want %v (DEFAULT)", got, want)
	}
	if got, want := o.AllowCredentialReuse, false; got != want {
		t.Errorf("AllowCredentialReuse = %v, want %v (DEFAULT)", got, want)
	}
	if got, want := o.EnableInteropTest, false; got != want {
		t.Errorf("EnableInteropTest = %v, want %v (DEFAULT)", got, want)
	}
}

func TestDeviceInit_PositionalArgOverridesServerURL(t *testing.T) {
	toml := `blob = "cred.bin"
key = "ec384"

[device-init]
server-url = "https://config.com:8080"`

	yaml := `blob: cred.bin
key: ec384
device-init:
  server-url: https://config.com:8080`

	runTestBothFormats(t, "", deviceInitCmd, toml, yaml, false, "https://positional.com:9090")

	if got, want := capturedConfig.DeviceInitConfig.ServerURL, "https://positional.com:9090"; got != want {
		t.Errorf("ServerURL = %q, want %q", got, want)
	}
}

func TestDeviceInit_Defaults(t *testing.T) {
	toml := `blob = "cred.bin"
key = "ec384"

[device-init]
server-url = "https://127.0.0.1:8080"`

	yaml := `blob: cred.bin
key: ec384
device-init:
  server-url: https://127.0.0.1:8080`

	runTestBothFormats(t, "", deviceInitCmd, toml, yaml, false)

	if got, want := capturedConfig.Debug, false; got != want {
		t.Errorf("Debug = %v, want %v", got, want)
	}
	if got, want := capturedConfig.DeviceInitConfig.KeyEnc, "x509"; got != want {
		t.Errorf("KeyEnc = %q, want %q", got, want)
	}
	if got, want := capturedConfig.DeviceInitConfig.InsecureTLS, false; got != want {
		t.Errorf("InsecureTLS = %v, want %v", got, want)
	}
}

func TestOnboard_Defaults(t *testing.T) {
	toml := `blob = "cred.bin"
key = "ec384"

[onboard]
kex = "ECDH256"`

	yaml := `blob: cred.bin
key: ec384
onboard:
  kex: ECDH256`

	runTestBothFormats(t, "", onboardCmd, toml, yaml, false)

	o := capturedConfig.OnboardConfig
	if got, want := o.Cipher, "A128GCM"; got != want {
		t.Errorf("Cipher = %q, want %q", got, want)
	}
	if got, want := o.MaxServiceInfoSize, 1300; got != want {
		t.Errorf("MaxServiceInfoSize = %d, want %d", got, want)
	}
	if got, want := o.InsecureTLS, false; got != want {
		t.Errorf("InsecureTLS = %v, want %v", got, want)
	}
	if got, want := o.EchoCommands, false; got != want {
		t.Errorf("EchoCommands = %v, want %v", got, want)
	}
	if got, want := o.Resale, false; got != want {
		t.Errorf("Resale = %v, want %v", got, want)
	}
}

func TestDeviceInit_URLEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"hostname too long", "https://" + strings.Repeat("a", 256) + ":8080"},
		{"invalid hostname", "https://inval!d:8080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := fmt.Sprintf("blob: cred.bin\nkey: ec384\ndevice-init:\n  server-url: %s", tt.url)
			if err := runTest(t, deviceInitCmd, yaml, "yaml"); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestOnboard_MaxServiceInfoSizeBoundaries(t *testing.T) {
	tests := []struct {
		value   string
		wantErr bool
	}{
		{"0", false},
		{"65535", false},
		{"-1", true},
		{"65536", true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			yaml := "blob: cred.bin\nkey: ec384\nonboard:\n  kex: ECDH256\n  cipher: A128GCM\n  max-serviceinfo-size: " + tt.value
			err := runTest(t, onboardCmd, yaml, "yaml")
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestOnboard_DirectoryValidation(t *testing.T) {
	validDir := t.TempDir()
	tests := []struct {
		name    string
		field   string
		value   string
		wantErr bool
	}{
		{"valid download", "download", validDir, false},
		{"invalid download", "download", "/nonexistent", true},
		{"valid wget-dir", "wget-dir", validDir, false},
		{"invalid wget-dir", "wget-dir", "/nonexistent", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := fmt.Sprintf("blob: cred.bin\nkey: ec384\nonboard:\n  kex: ECDH256\n  cipher: A128GCM\n  %s: %s", tt.field, tt.value)
			err := runTest(t, onboardCmd, yaml, "yaml")
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestOnboard_UploadFlag(t *testing.T) {
	t.Run("CLI with valid upload path", func(t *testing.T) {
		validFile := filepath.Join(t.TempDir(), "upload.txt")
		if err := os.WriteFile(validFile, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}

		if err := runCLI(t, onboardCmd, "onboard", "--blob", "cred.bin", "--key", "ec384",
			"--kex", "ECDH256", "--cipher", "A128GCM", "--upload", validFile); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got := len(onboardConfig.Onboard.Upload); got != 1 {
			t.Errorf("Upload count = %d, want 1", got)
		}
	})

	t.Run("CLI with invalid upload path", func(t *testing.T) {
		err := runCLI(t, onboardCmd, "onboard", "--blob", "cred.bin", "--key", "ec384",
			"--kex", "ECDH256", "--cipher", "A128GCM", "--upload", "/nonexistent/file.txt")
		if err == nil {
			t.Fatalf("expected error for invalid upload path, got nil")
		}
	})

	t.Run("CLI with multiple upload paths", func(t *testing.T) {
		dir := t.TempDir()
		file1 := filepath.Join(dir, "file1.txt")
		file2 := filepath.Join(dir, "file2.txt")
		if err := os.WriteFile(file1, []byte("test1"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file2, []byte("test2"), 0o600); err != nil {
			t.Fatal(err)
		}

		if err := runCLI(t, onboardCmd, "onboard", "--blob", "cred.bin", "--key", "ec384",
			"--kex", "ECDH256", "--cipher", "A128GCM", "--upload", file1, "--upload", file2); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got := len(onboardConfig.Onboard.Upload); got != 2 {
			t.Errorf("Upload count = %d, want 2", got)
		}
	})
}

func TestOnboard_UploadViaConfig(t *testing.T) {
	t.Run("single file via YAML", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "upload.txt")
		if err := os.WriteFile(file, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}

		yaml := fmt.Sprintf(`blob: cred.bin
key: ec384
onboard:
  kex: ECDH256
  cipher: A128GCM
  upload:
    - %s`, file)

		if err := runTest(t, onboardCmd, yaml, "yaml"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got := len(capturedConfig.OnboardConfig.Upload); got != 1 {
			t.Errorf("Upload count = %d, want 1", got)
		}
	})

	t.Run("multiple files via YAML", func(t *testing.T) {
		dir := t.TempDir()
		file1 := filepath.Join(dir, "file1.txt")
		file2 := filepath.Join(dir, "file2.txt")
		if err := os.WriteFile(file1, []byte("test1"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file2, []byte("test2"), 0o600); err != nil {
			t.Fatal(err)
		}

		yaml := fmt.Sprintf(`blob: cred.bin
key: ec384
onboard:
  kex: ECDH256
  cipher: A128GCM
  upload:
    - %s
    - %s`, file1, file2)

		if err := runTest(t, onboardCmd, yaml, "yaml"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got := len(capturedConfig.OnboardConfig.Upload); got != 2 {
			t.Errorf("Upload count = %d, want 2", got)
		}
	})

	t.Run("invalid path via YAML", func(t *testing.T) {
		yaml := `blob: cred.bin
key: ec384
onboard:
  kex: ECDH256
  cipher: A128GCM
  upload:
    - /nonexistent/file.txt`

		err := runTest(t, onboardCmd, yaml, "yaml")
		if err == nil {
			t.Fatal("expected error for nonexistent upload file")
		}
	})

	t.Run("CLI overrides config", func(t *testing.T) {
		dir := t.TempDir()
		configFile := filepath.Join(dir, "config-upload.txt")
		cliFile := filepath.Join(dir, "cli-upload.txt")
		if err := os.WriteFile(configFile, []byte("from config"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cliFile, []byte("from cli"), 0o600); err != nil {
			t.Fatal(err)
		}

		yaml := fmt.Sprintf(`blob: cred.bin
key: ec384
onboard:
  kex: ECDH256
  cipher: A128GCM
  upload:
    - %s`, configFile)

		resetState(t)
		stubRunE(t, onboardCmd)
		path := writeConfig(t, yaml, "yaml")

		rootCmd.SetArgs([]string{"onboard", "--config", path, "--upload", cliFile})

		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// CLI should override config (only CLI file present)
		if got := len(onboardConfig.Onboard.Upload); got != 1 {
			t.Errorf("Upload count = %d, want 1 (CLI overrides config)", got)
		}

		// Verify it's the CLI file, not the config file
		found := false
		for _, path := range onboardConfig.Onboard.Upload {
			if strings.Contains(path, "cli-upload.txt") {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected CLI upload file, but it's not present")
		}
	})
}
