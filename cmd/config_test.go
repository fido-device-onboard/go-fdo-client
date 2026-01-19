package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// resetState reinitializes the CLI/Config logic for testing
func resetState(t *testing.T) {
	t.Helper()

	// Reset viper
	viper.Reset()

	// Reset root command flags
	rootCmd.ResetFlags()
	rootCmd.ResetCommands()
	rootCmd.SetArgs(nil)

	// Reset device init command flags
	deviceInitCmd.ResetFlags()
	deviceInitCmd.ResetCommands()
	deviceInitCmd.SetArgs(nil)

	// Reset onboard command flags
	onboardCmd.ResetFlags()
	onboardCmd.ResetCommands()
	onboardCmd.SetArgs(nil)

	// Reinitialize flags
	rootCmdInit()
	deviceInitCmdInit()
	onboardCmdInit()

	// Reset global variables
	debug = false
	blobPath = ""
	tpmPath = ""
	configFile = ""
	diURL = ""
	diDeviceInfo = ""
	diDeviceInfoMac = ""
	diKey = ""
	diKeyEnc = "x509"
	insecureTLS = false
	allowCredentialReuse = false
	cipherSuite = "A128GCM"
	dlDir = ""
	echoCmds = false
	kexSuite = ""
	maxServiceInfoSize = 1300
	resale = false
	to2RetryDelay = 0
	uploads = make(fsVar)
	wgetDir = ""
}

// stubDeviceInitRunE stubs out the device-init command execution but keeps config loading
func stubDeviceInitRunE(t *testing.T) {
	t.Helper()
	orig := deviceInitCmd.RunE
	deviceInitCmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Positional argument takes highest precedence
		if len(args) > 0 {
			diURL = args[0]
		} else if cmd.Flags().Changed("server-url") {
			diURL = viper.GetString("device-init.server-url")
		} else if viper.IsSet("device-init.server-url") {
			diURL = viper.GetString("device-init.server-url")
		}

		// Load other config values from viper if not set via CLI
		loadStringFromConfig(cmd, "key", "device-init.key", &diKey)
		loadStringFromConfig(cmd, "key-enc", "device-init.key-enc", &diKeyEnc)
		loadStringFromConfig(cmd, "device-info", "device-init.device-info", &diDeviceInfo)
		loadStringFromConfig(cmd, "device-info-mac", "device-init.device-info-mac", &diDeviceInfoMac)
		loadBoolFromConfig(cmd, "insecure-tls", "device-init.insecure-tls", &insecureTLS)
		return nil
	}
	t.Cleanup(func() { deviceInitCmd.RunE = orig })
}

// stubOnboardRunE stubs out the onboard command execution but keeps config loading
func stubOnboardRunE(t *testing.T) {
	t.Helper()
	orig := onboardCmd.RunE
	onboardCmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Load config values from viper if not set via CLI
		loadStringFromConfig(cmd, "key", "onboard.key", &diKey)
		loadStringFromConfig(cmd, "kex", "onboard.kex", &kexSuite)
		loadStringFromConfig(cmd, "cipher", "onboard.cipher", &cipherSuite)
		loadStringFromConfig(cmd, "download", "onboard.download", &dlDir)
		loadBoolFromConfig(cmd, "echo-commands", "onboard.echo-commands", &echoCmds)
		loadBoolFromConfig(cmd, "insecure-tls", "onboard.insecure-tls", &insecureTLS)
		loadIntFromConfig(cmd, "max-serviceinfo-size", "onboard.max-serviceinfo-size", &maxServiceInfoSize)
		loadBoolFromConfig(cmd, "allow-credential-reuse", "onboard.allow-credential-reuse", &allowCredentialReuse)
		loadBoolFromConfig(cmd, "resale", "onboard.resale", &resale)
		loadDurationFromConfig(cmd, "to2-retry-delay", "onboard.to2-retry-delay", &to2RetryDelay)
		loadStringFromConfig(cmd, "wget-dir", "onboard.wget-dir", &wgetDir)
		return nil
	}
	t.Cleanup(func() { onboardCmd.RunE = orig })
}

func writeYAMLConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeTOMLConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(p, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestConfigLoadYAML(t *testing.T) {
	resetState(t)
	stubDeviceInitRunE(t)

	config := `
debug: true
blob: "test-cred.bin"

device-init:
  server-url: "https://example.com:8080"
  key: "ec384"
  key-enc: "x5chain"
  device-info: "test-device"
  insecure-tls: true
`
	path := writeYAMLConfig(t, config)
	rootCmd.SetArgs([]string{"device-init", "--config", path})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify global config loaded
	if !debug {
		t.Error("expected debug to be true")
	}
	if blobPath != "test-cred.bin" {
		t.Errorf("expected blob to be 'test-cred.bin', got %q", blobPath)
	}

	// Verify device-init config loaded
	if diURL != "https://example.com:8080" {
		t.Errorf("expected server-url to be 'https://example.com:8080', got %q", diURL)
	}
	if diKey != "ec384" {
		t.Errorf("expected key to be 'ec384', got %q", diKey)
	}
	if diKeyEnc != "x5chain" {
		t.Errorf("expected key-enc to be 'x5chain', got %q", diKeyEnc)
	}
	if diDeviceInfo != "test-device" {
		t.Errorf("expected device-info to be 'test-device', got %q", diDeviceInfo)
	}
	if !insecureTLS {
		t.Error("expected insecure-tls to be true")
	}
}

func TestConfigLoadTOML(t *testing.T) {
	resetState(t)
	stubDeviceInitRunE(t)

	config := `
debug = true
blob = "test-cred.bin"

[device-init]
server-url = "https://example.com:8080"
key = "ec256"
key-enc = "cose"
`
	path := writeTOMLConfig(t, config)
	rootCmd.SetArgs([]string{"device-init", "--config", path})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diKey != "ec256" {
		t.Errorf("expected key to be 'ec256', got %q", diKey)
	}
	if diKeyEnc != "cose" {
		t.Errorf("expected key-enc to be 'cose', got %q", diKeyEnc)
	}
}

func TestCLIOverridesConfig(t *testing.T) {
	resetState(t)
	stubDeviceInitRunE(t)

	config := `
debug: false
blob: "config-cred.bin"

device-init:
  server-url: "https://config.example.com:8080"
  key: "ec384"
  key-enc: "x509"
`
	path := writeYAMLConfig(t, config)

	// CLI flags should override config values
	rootCmd.SetArgs([]string{
		"device-init",
		"--config", path,
		"--debug",
		"--blob", "cli-cred.bin",
		"--key", "ec256",
		"https://cli.example.com:9090", // positional arg
	})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// CLI flags should take precedence
	if !debug {
		t.Error("expected debug to be true (from CLI)")
	}
	if blobPath != "cli-cred.bin" {
		t.Errorf("expected blob to be 'cli-cred.bin', got %q", blobPath)
	}
	if diURL != "https://cli.example.com:9090" {
		t.Errorf("expected server-url to be 'https://cli.example.com:9090', got %q", diURL)
	}
	if diKey != "ec256" {
		t.Errorf("expected key to be 'ec256' (from CLI), got %q", diKey)
	}
}

func TestPositionalArgOverridesConfigServerURL(t *testing.T) {
	resetState(t)
	stubDeviceInitRunE(t)

	config := `
blob: "cred.bin"

device-init:
  server-url: "https://config.example.com:8080"
  key: "ec384"
`
	path := writeYAMLConfig(t, config)

	// Positional arg should override config
	rootCmd.SetArgs([]string{
		"device-init",
		"--config", path,
		"https://positional.example.com:9999",
	})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diURL != "https://positional.example.com:9999" {
		t.Errorf("expected server-url to be 'https://positional.example.com:9999', got %q", diURL)
	}
}

func TestOnboardConfigLoad(t *testing.T) {
	resetState(t)
	stubOnboardRunE(t)

	config := `
debug: true
blob: "cred.bin"

onboard:
  key: "ec384"
  kex: "ECDH384"
  cipher: "A256GCM"
  download: "/tmp/downloads"
  echo-commands: true
  insecure-tls: true
  max-serviceinfo-size: 2000
  allow-credential-reuse: true
  resale: true
  to2-retry-delay: "10s"
  wget-dir: "/tmp/wget"
  upload:
    - "/tmp/file1"
    - "/tmp/file2"
`
	path := writeYAMLConfig(t, config)
	rootCmd.SetArgs([]string{"onboard", "--config", path})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diKey != "ec384" {
		t.Errorf("expected key to be 'ec384', got %q", diKey)
	}
	if kexSuite != "ECDH384" {
		t.Errorf("expected kex to be 'ECDH384', got %q", kexSuite)
	}
	if cipherSuite != "A256GCM" {
		t.Errorf("expected cipher to be 'A256GCM', got %q", cipherSuite)
	}
	if !echoCmds {
		t.Error("expected echo-commands to be true")
	}
	if !insecureTLS {
		t.Error("expected insecure-tls to be true")
	}
	if maxServiceInfoSize != 2000 {
		t.Errorf("expected max-serviceinfo-size to be 2000, got %d", maxServiceInfoSize)
	}
	if !allowCredentialReuse {
		t.Error("expected allow-credential-reuse to be true")
	}
	if !resale {
		t.Error("expected resale to be true")
	}
	if to2RetryDelay != 10*time.Second {
		t.Errorf("expected to2-retry-delay to be 10s, got %v", to2RetryDelay)
	}
}

func TestOnboardConfigLoadTOML(t *testing.T) {
	resetState(t)
	stubOnboardRunE(t)

	config := `
debug = true
blob = "cred.bin"

[onboard]
key = "ec384"
kex = "ECDH384"
cipher = "A256GCM"
download = "/tmp/downloads"
echo-commands = true
insecure-tls = true
max-serviceinfo-size = 2000
allow-credential-reuse = true
resale = true
to2-retry-delay = "10s"
wget-dir = "/tmp/wget"
`
	path := writeTOMLConfig(t, config)
	rootCmd.SetArgs([]string{"onboard", "--config", path})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diKey != "ec384" {
		t.Errorf("expected key to be 'ec384', got %q", diKey)
	}
	if kexSuite != "ECDH384" {
		t.Errorf("expected kex to be 'ECDH384', got %q", kexSuite)
	}
	if cipherSuite != "A256GCM" {
		t.Errorf("expected cipher to be 'A256GCM', got %q", cipherSuite)
	}
	if !echoCmds {
		t.Error("expected echo-commands to be true")
	}
	if !insecureTLS {
		t.Error("expected insecure-tls to be true")
	}
	if maxServiceInfoSize != 2000 {
		t.Errorf("expected max-serviceinfo-size to be 2000, got %d", maxServiceInfoSize)
	}
	if !allowCredentialReuse {
		t.Error("expected allow-credential-reuse to be true")
	}
	if !resale {
		t.Error("expected resale to be true")
	}
	if to2RetryDelay != 10*time.Second {
		t.Errorf("expected to2-retry-delay to be 10s, got %v", to2RetryDelay)
	}
}

func TestNoConfigFileRequired(t *testing.T) {
	resetState(t)
	stubDeviceInitRunE(t)

	// Should work without config file, using only CLI flags
	rootCmd.SetArgs([]string{
		"device-init",
		"--blob", "cred.bin",
		"--key", "ec384",
		"https://example.com:8080",
	})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if blobPath != "cred.bin" {
		t.Errorf("expected blob to be 'cred.bin', got %q", blobPath)
	}
	if diKey != "ec384" {
		t.Errorf("expected key to be 'ec384', got %q", diKey)
	}
	if diURL != "https://example.com:8080" {
		t.Errorf("expected server-url to be 'https://example.com:8080', got %q", diURL)
	}
}

func TestConfigFileNotFoundError(t *testing.T) {
	resetState(t)

	rootCmd.SetArgs([]string{
		"device-init",
		"--config", "/nonexistent/config.yaml",
		"--blob", "cred.bin",
		"--key", "ec384",
		"https://example.com:8080",
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent config file")
	}
	if !strings.Contains(err.Error(), "failed to read config file") {
		t.Errorf("expected error to contain 'failed to read config file', got: %v", err)
	}
}

func TestMissingBlobOrTPM(t *testing.T) {
	resetState(t)

	config := `
debug: true

device-init:
  server-url: "https://example.com:8080"
  key: "ec384"
`
	path := writeYAMLConfig(t, config)

	// Neither blob nor tpm specified
	rootCmd.SetArgs([]string{
		"device-init",
		"--config", path,
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when neither blob nor tpm is specified")
	}
	if !strings.Contains(err.Error(), "either --blob or --tpm must be specified") {
		t.Errorf("expected error to contain 'either --blob or --tpm must be specified', got: %v", err)
	}
}

func TestMissingRequiredServerURL(t *testing.T) {
	resetState(t)

	config := `
blob: "cred.bin"

device-init:
  key: "ec384"
`
	path := writeYAMLConfig(t, config)

	rootCmd.SetArgs([]string{
		"device-init",
		"--config", path,
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when server-url is missing")
	}
	if !strings.Contains(err.Error(), "server-url is required") {
		t.Errorf("expected error to contain 'server-url is required', got: %v", err)
	}
}

func TestMissingRequiredKey(t *testing.T) {
	resetState(t)

	config := `
blob: "cred.bin"

device-init:
  server-url: "https://example.com:8080"
`
	path := writeYAMLConfig(t, config)

	rootCmd.SetArgs([]string{
		"device-init",
		"--config", path,
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when key is missing")
	}
	if !strings.Contains(err.Error(), "--key is required") {
		t.Errorf("expected error to contain '--key is required', got: %v", err)
	}
}

func TestMissingRequiredKeyCLI(t *testing.T) {
	resetState(t)

	// Test missing key via CLI (no config file)
	rootCmd.SetArgs([]string{
		"device-init",
		"--blob", "cred.bin",
		"https://example.com:8080",
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --key is not provided via CLI")
	}
	if !strings.Contains(err.Error(), "--key is required") {
		t.Errorf("expected error to contain '--key is required', got: %v", err)
	}
}

func TestTPMFromConfig(t *testing.T) {
	resetState(t)
	stubDeviceInitRunE(t)

	config := `
debug: true
tpm: "/dev/tpm0"

device-init:
  server-url: "https://example.com:8080"
  key: "ec384"
`
	path := writeYAMLConfig(t, config)
	rootCmd.SetArgs([]string{"device-init", "--config", path})

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tpmPath != "/dev/tpm0" {
		t.Errorf("expected tpm to be '/dev/tpm0', got %q", tpmPath)
	}
	if blobPath != "" {
		t.Errorf("expected blob to be empty, got %q", blobPath)
	}
}

func TestMutuallyExclusiveDeviceInfoFlags(t *testing.T) {
	resetState(t)

	config := `
blob: "cred.bin"

device-init:
  server-url: "https://example.com:8080"
  key: "ec384"
  device-info: "custom-device-info"
  device-info-mac: "eth0"
`
	path := writeYAMLConfig(t, config)
	rootCmd.SetArgs([]string{"device-init", "--config", path})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when both device-info and device-info-mac are specified")
	}
	if !strings.Contains(err.Error(), "device-info") || !strings.Contains(err.Error(), "device-info-mac") {
		t.Errorf("expected error to mention both device-info and device-info-mac, got: %v", err)
	}
}

func TestMutuallyExclusiveDeviceInfoFlagsCLI(t *testing.T) {
	resetState(t)

	// Test via CLI flags
	rootCmd.SetArgs([]string{
		"device-init",
		"--blob", "cred.bin",
		"--key", "ec384",
		"--device-info", "custom-info",
		"--device-info-mac", "eth0",
		"https://example.com:8080",
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when both --device-info and --device-info-mac CLI flags are specified")
	}
	if !strings.Contains(err.Error(), "device-info") || !strings.Contains(err.Error(), "device-info-mac") {
		t.Errorf("expected error to mention both device-info and device-info-mac, got: %v", err)
	}
}
