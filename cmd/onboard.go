// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"math/rand/v2"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-client/internal/tls"
	"github.com/fido-device-onboard/go-fdo-client/internal/tpm_utils"
	"github.com/fido-device-onboard/go-fdo/cose"
	"github.com/fido-device-onboard/go-fdo/fsim"
	"github.com/fido-device-onboard/go-fdo/kex"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/serviceinfo"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type slogErrorWriter struct{}

func (e slogErrorWriter) Write(p []byte) (int, error) {
	w := bytes.TrimSpace(p)
	slog.Error(string(w))
	return len(w), nil
}

var onboardConfig OnboardClientConfig

var validCipherSuites = []string{
	"A128GCM", "A192GCM", "A256GCM",
	"AES-CCM-64-128-128", "AES-CCM-64-128-256",
	"COSEAES128CBC", "COSEAES128CTR",
	"COSEAES256CBC", "COSEAES256CTR",
}
var validKexSuites = []string{
	"DHKEXid14", "DHKEXid15", "ASYMKEX2048", "ASYMKEX3072", "ECDH256", "ECDH384",
}

var onboardCmd = &cobra.Command{
	Use:   "onboard",
	Short: "Run FDO TO1 and TO2 onboarding",
	Long: `
Run FDO TO1 and TO2 onboarding to transfer device ownership to the owner server.
The device must have been initialized (device-init) before running onboard.
At least one of --blob or --tpm is required to access device credentials.`,
	Example: `
  # Using CLI arguments:
  go-fdo-client onboard --key ec256 --kex ECDH256 --blob cred.bin

  # Using config file:
  go-fdo-client onboard --config config.yaml

  # Mix CLI and config (CLI takes precedence):
  go-fdo-client onboard --config config.yaml --cipher A256GCM`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		err := bindFlags(cmd, "onboard")
		if err != nil {
			return err
		}

		if err := viper.Unmarshal(&onboardConfig); err != nil {
			return fmt.Errorf("failed to unmarshal onboard config: %w", err)
		}

		return onboardConfig.validate()
	},
	RunE: func(cmd *cobra.Command, args []string) error {

		if rootConfig.Debug {
			level.Set(slog.LevelDebug)
		}

		if rootConfig.TPM != "" {
			var err error
			tpmc, err = tpm_utils.TpmOpen(rootConfig.TPM)
			if err != nil {
				return err
			}
			defer tpmc.Close()
		}

		deviceStatus, err := loadDeviceStatus()
		if err != nil {
			return fmt.Errorf("load device status failed: %w", err)
		}

		printDeviceStatus(deviceStatus)

		if deviceStatus == FDO_STATE_PRE_TO1 || (deviceStatus == FDO_STATE_IDLE && onboardConfig.Onboard.Resale) {
			return doOnboard()
		} else if deviceStatus == FDO_STATE_IDLE {
			slog.Info("FDO in Idle State. Device Onboarding already completed")
		} else if deviceStatus == FDO_STATE_PRE_DI {
			return fmt.Errorf("device has not been properly initialized: run device-init first")
		} else {
			return fmt.Errorf("device state is invalid: %v", deviceStatus)
		}

		return nil
	},
}

func onboardCmdInit() {
	rootCmd.AddCommand(onboardCmd)

	// Get current working directory for default values
	currentDir, err := os.Getwd()
	if err != nil {
		// If we can't get working directory, leave as empty string
		// (validation will require user to specify an absolute path)
		currentDir = ""
	}

	onboardCmd.Flags().Bool("allow-credential-reuse", false, "Allow credential reuse protocol during onboarding")
	onboardCmd.Flags().String("cipher", "A128GCM", "Name of cipher suite to use for encryption (see usage)")
	onboardCmd.Flags().String("download", "", "fdo.download: override destination directory set by Owner server")
	onboardCmd.Flags().Bool("echo-commands", false, "Echo all commands received to stdout (FSIM disabled if false)")
	onboardCmd.Flags().Bool("enable-interop-test", false, "Enable FIDO Alliance interop test module (fsim.Interop)")
	onboardCmd.Flags().String("kex", "", "Name of cipher suite to use for key exchange (see usage)")
	onboardCmd.Flags().Bool("insecure-tls", false, "Skip TLS certificate verification")
	onboardCmd.Flags().Int("max-serviceinfo-size", serviceinfo.DefaultMTU, "Maximum service info size to receive")
	onboardCmd.Flags().Bool("resale", false, "Perform resale")
	onboardCmd.Flags().Duration("to2-retry-delay", 0, "Delay between failed TO2 attempts when trying multiple Owner URLs from same RV directive (0=disabled)")
	onboardCmd.Flags().String("default-working-dir", currentDir, "Default working directory for all FSIMs (fdo.command, fdo.download, fdo.upload, fdo.wget)")
	onboardCmd.Flags().String("wget-dir", "", "fdo.wget: override destination directory set by Owner server")
}

func init() {
	onboardCmdInit()
}

func doOnboard() error {
	// Read device credential blob to configure client for TO1/TO2
	dc, hmacSha256, hmacSha384, privateKey, cleanup, err := readCred()
	if err == nil && cleanup != nil {
		defer func() { _ = cleanup() }()
	}
	if err != nil {
		return err
	}

	// Try TO1+TO2
	kexCipherSuiteID, ok := kex.CipherSuiteByName(onboardConfig.Onboard.Cipher)
	if !ok {
		return fmt.Errorf("invalid key exchange cipher suite: %s", onboardConfig.Onboard.Cipher)
	}

	osVersion, err := getOSVersion()
	if err != nil {
		osVersion = "unknown"
		slog.Warn("Setting serviceinfo.Devmod.Version", "error", err, "default", osVersion)
	}

	deviceName, err := getDeviceName()
	if err != nil {
		deviceName = "unknown"
		slog.Warn("Setting serviceinfo.Devmod.Device", "error", err, "default", deviceName)
	}

	newDC, err := transferOwnership(clientContext, dc.RvInfo, fdo.TO2Config{
		Cred:       *dc,
		HmacSha256: hmacSha256,
		HmacSha384: hmacSha384,
		Key:        privateKey,
		Devmod: serviceinfo.Devmod{
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
			Version: osVersion,
			Device:  deviceName,
			FileSep: ";",
			Bin:     runtime.GOARCH,
		},
		KeyExchange:               kex.Suite(onboardConfig.Onboard.Kex),
		CipherSuite:               kexCipherSuiteID,
		AllowCredentialReuse:      onboardConfig.Onboard.AllowCredentialReuse,
		MaxServiceInfoSizeReceive: uint16(onboardConfig.Onboard.MaxServiceInfoSize),
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			slog.Info("Onboarding canceled by user")
		}
		return err
	}
	if newDC == nil {
		slog.Info("Credential not updated (Credential Reuse Protocol)")
		return nil
	}

	// Store new credential
	slog.Info("FIDO Device Onboard Complete")
	return updateCred(*newDC, FDO_STATE_IDLE)
}

// addJitter adds ±25% randomization to a delay duration as per FDO spec v1.1 section 3.7.
func addJitter(delay time.Duration) time.Duration {
	jitterPercent := 0.25 * (2*rand.Float64() - 1) // Random from -0.25 to +0.25 (±25%)
	jitter := float64(delay) * jitterPercent
	return delay + time.Duration(jitter)
}

// applyDelay waits for the specified duration with context cancellation support.
func applyDelay(ctx context.Context, delay time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

// getOwnerURLs performs TO1 protocol to discover Owner URLs or uses RV bypass.
// Returns: owner URLs, TO1 response (needed for TO2)
func getOwnerURLs(ctx context.Context, directive *protocol.RvDirective, conf fdo.TO2Config) ([]string, *cose.Sign1[protocol.To1d, []byte]) {
	var to1d *cose.Sign1[protocol.To1d, []byte]
	var ownerURLs []string

	// RV bypass: Use Owner URLs directly from directive, skipping TO1
	if directive.Bypass {
		slog.Info("RV bypass enabled, skipping TO1 protocol")
		for _, url := range directive.URLs {
			ownerURLs = append(ownerURLs, url.String())
			slog.Info("Using Owner URL from bypass directive", "url", url.String())
		}
		return ownerURLs, nil
	}

	// Normal flow: Contact Rendezvous server via TO1 to discover Owner address
	slog.Info("Attempting TO1 protocol")
	for _, url := range directive.URLs {
		var err error
		to1d, err = fdo.TO1(ctx, tls.TlsTransport(url.String(), nil, onboardConfig.Onboard.InsecureTLS), conf.Cred, conf.Key, nil)
		if err != nil {
			slog.Error("TO1 failed", "base URL", url.String(), "error", err)
			continue
		}
		slog.Info("TO1 succeeded", "base URL", url.String())
		break
	}

	// Check if all TO1 attempts failed
	// Note: Empty URLs is valid (delay-only directive), individual failures already logged in loop
	if to1d == nil {
		slog.Info("All TO1 attempts failed for this directive")
		return nil, nil // Return empty URLs - will skip TO2
	}

	// TO1 succeeded - extract TO2 URLs from response
	for _, to2Addr := range to1d.Payload.Val.RV {
		if to2Addr.DNSAddress == nil && to2Addr.IPAddress == nil {
			slog.Error("Both IP and DNS can't be null")
			continue
		}

		var scheme, port string
		switch to2Addr.TransportProtocol {
		case protocol.HTTPTransport:
			scheme, port = "http://", "80"
		case protocol.HTTPSTransport:
			scheme, port = "https://", "443"
		default:
			slog.Error("Unsupported transport protocol", "transport protocol", to2Addr.TransportProtocol)
			continue
		}
		if to2Addr.Port != 0 {
			port = strconv.Itoa(int(to2Addr.Port))
		}

		// Check and add DNS address if valid and resolvable
		if to2Addr.DNSAddress != nil {
			if isResolvableDNS(*to2Addr.DNSAddress) {
				host := *to2Addr.DNSAddress
				ownerURLs = append(ownerURLs, scheme+net.JoinHostPort(host, port))
			} else {
				slog.Warn("DNS address is not resolvable", "dns", *to2Addr.DNSAddress)
			}
		}

		// Check and add IP address if valid
		if to2Addr.IPAddress != nil {
			if isValidIP(to2Addr.IPAddress.String()) {
				host := to2Addr.IPAddress.String()
				ownerURLs = append(ownerURLs, scheme+net.JoinHostPort(host, port))
			} else {
				slog.Warn("IP address is not valid", "ip", to2Addr.IPAddress.String())
			}
		}
	}

	// Check if TO1 succeeded but returned no valid TO2 addresses
	// This is unexpected but valid (manufacturer may have configured device oddly)
	if len(ownerURLs) == 0 {
		slog.Info("TO1 succeeded but no valid TO2 addresses found")
	}

	return ownerURLs, to1d
}

func transferOwnership(ctx context.Context, rvInfo [][]protocol.RvInstruction, conf fdo.TO2Config) (*fdo.DeviceCredential, error) { //nolint:gocyclo
	directives := protocol.ParseDeviceRvInfo(rvInfo)

	if len(directives) == 0 {
		return nil, errors.New("no rendezvous information found that's usable for the device")
	}

	// Infinite retry loop - continues until onboarding succeeds or context canceled
	for {
		for i, directive := range directives {
			isLastDirective := (i == len(directives)-1)

			// Step 1: Get Owner URLs (via TO1 or RV bypass)
			ownerURLs, to1d := getOwnerURLs(ctx, &directive, conf)

			// Step 2: Attempt TO2 with each Owner URL
			// Note: If TO1 failed, ownerURLs is empty and loop is skipped
			if len(ownerURLs) > 0 {
				slog.Info("Attempting TO2 protocol")
			}
			for j, baseURL := range ownerURLs {
				isLastURL := (j == len(ownerURLs)-1)
				newDC, err := transferOwnership2(ctx, tls.TlsTransport(baseURL, nil, onboardConfig.Onboard.InsecureTLS), to1d, conf)
				if newDC != nil {
					slog.Info("TO2 succeeded", "base URL", baseURL)
					return newDC, nil
				}
				slog.Error("TO2 failed", "base URL", baseURL, "error", err)

				// Apply configurable delay between Owner URLs within a directive
				// (not spec-compliant, but prevents hammering the same server via different URLs)
				if !isLastURL && onboardConfig.Onboard.TO2RetryDelay > 0 {
					slog.Info("Applying TO2 retry delay", "delay", onboardConfig.Onboard.TO2RetryDelay)
					if err := applyDelay(ctx, onboardConfig.Onboard.TO2RetryDelay); err != nil {
						return nil, err
					}
				}
			}

			// Step 3: Apply delay after directive attempts (TO1 failed or all TO2 URLs failed)
			// IMPORTANT: Delay applies even with zero URLs (allows RVDelaySec-only directives)
			if directive.Delay != 0 {
				// Use configured delay from directive
				delay := addJitter(directive.Delay)
				slog.Info("Applying directive delay", "delay", delay)
				if err := applyDelay(ctx, delay); err != nil {
					return nil, err
				}
			} else if isLastDirective {
				// Last directive with no configured delay - apply default
				delay := addJitter(120 * time.Second)
				slog.Info("Applying default delay for last directive", "delay", delay)
				if err := applyDelay(ctx, delay); err != nil {
					return nil, err
				}
			}
			// Non-last directive with no delay - continue to next directive
		}
	}
}

// initializeFSIMs creates and configures all FDO Service Info Modules (FSIMs).
// All standard FSIMs (fdo.command, fdo.download, fdo.upload, fdo.wget) are enabled
// by default using go-fdo library defaults. The CLI parameters allow customization
// of the default behavior. Uses a common default directory for FIDO Alliance compliance.
func initializeFSIMs(dlDir, wgetDir, defaultWorkingDir string, enableInteropTest bool) map[string]serviceinfo.DeviceModule {
	fsims := map[string]serviceinfo.DeviceModule{}
	if enableInteropTest {
		fsims["fido_alliance"] = &fsim.Interop{}
	}

	// For now enable all supported service modules. Follow up: introduce a CLI option
	// that allows the user to explicitly select which FSIMs should be
	// enabled.

	// Use service module defaults provided by the go-fdo library. Use the CLI options
	// to customize the default behavior.

	fsims["fdo.command"] = &fsim.Command{}

	// fdo.download: enable error output. Use --download to force downloaded files into a specific
	// local directory, otherwise allow the owner server to control where the file is stored on
	// the local device
	dlFSIM := &fsim.Download{
		ErrorLog: &slogErrorWriter{},
	}
	if dlDir != "" {
		dlFSIM.CreateTemp = func() (*os.File, error) {
			tmpFile, err := os.CreateTemp(dlDir, ".fdo.download_*")
			if err != nil {
				return nil, err
			}
			return tmpFile, nil
		}
		dlFSIM.NameToPath = func(name string) string {
			cleanName := filepath.Clean(name)
			return filepath.Join(dlDir, filepath.Base(cleanName))
		}
	}
	fsims["fdo.download"] = dlFSIM

	// fdo.upload:
	// - Absolute paths are always allowed (no restrictions on device side)
	// - Relative paths use the default directory
	fsims["fdo.upload"] = &fsim.Upload{
		FS: &WorkingDirFS{
			DefaultDir: defaultWorkingDir,
		},
	}

	// fdo.wget: use --wget-dir to force downloaded files into a specific local directory,
	// otherwise allow the owner server to control where the file is stored on the device
	wgetFSIM := &fsim.Wget{}
	if wgetDir != "" {
		wgetFSIM.CreateTemp = func() (*os.File, error) {
			tmpFile, err := os.CreateTemp(wgetDir, ".fdo.wget_*")
			if err != nil {
				return nil, err
			}
			return tmpFile, nil
		}
		wgetFSIM.NameToPath = func(name string) string {
			cleanName := filepath.Clean(name)
			return filepath.Join(wgetDir, filepath.Base(cleanName))
		}
	}
	fsims["fdo.wget"] = wgetFSIM

	return fsims
}

func transferOwnership2(ctx context.Context, transport fdo.Transport, to1d *cose.Sign1[protocol.To1d, []byte], conf fdo.TO2Config) (*fdo.DeviceCredential, error) {
	conf.DeviceModules = initializeFSIMs(onboardConfig.Onboard.Download, onboardConfig.Onboard.WgetDir, onboardConfig.Onboard.DefaultWorkingDir, onboardConfig.Onboard.EnableInteropTest)
	return fdo.TO2(ctx, transport, to1d, conf)
}

// Function to validate if a string is a valid IP address
func isValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

// Function to check if a DNS address is resolvable
func isResolvableDNS(dns string) bool {
	_, err := net.LookupHost(dns)
	return err == nil
}

func printDeviceStatus(status FdoDeviceState) {
	switch status {
	case FDO_STATE_PRE_DI:
		slog.Debug("Device is ready for DI")
	case FDO_STATE_PRE_TO1:
		slog.Debug("Device is ready for Ownership transfer")
	case FDO_STATE_IDLE:
		slog.Debug("Device Ownership transfer Done")
	case FDO_STATE_RESALE:
		slog.Debug("Device is ready for Ownership transfer")
	case FDO_STATE_ERROR:
		slog.Debug("Error in getting device status")
	}
}

// WorkingDirFS implements a simplified file system for uploads following FIDO Alliance spec
type WorkingDirFS struct {
	DefaultDir string // Default directory for relative paths
}

// Open implements fs.FS with simplified logic:
// - Absolute paths are always allowed
// - Relative paths are resolved from DefaultDir
// - Only basic file validation (no directories)
func (ufs *WorkingDirFS) Open(name string) (fs.File, error) {
	var targetPath string

	// Determine target path based on absolute vs relative
	if filepath.IsAbs(name) {
		// Absolute paths are always allowed
		targetPath = name
	} else {
		// Relative paths go to default directory
		targetPath = filepath.Join(ufs.DefaultDir, name)

		// Security check: ensure the resolved path is still within the default directory
		if !strings.HasPrefix(targetPath, filepath.Clean(ufs.DefaultDir)+string(filepath.Separator)) &&
			targetPath != filepath.Clean(ufs.DefaultDir) {
			return nil, &fs.PathError{
				Op:   "open",
				Path: name,
				Err:  fs.ErrPermission,
			}
		}
	}

	// Open the file
	file, err := os.Open(targetPath)
	if err != nil {
		return nil, err
	}

	// Basic validation - ensure it's a file, not a directory
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	if info.IsDir() {
		file.Close()
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}

	return file, nil
}

func (o *OnboardClientConfig) validate() error {
	// Validate default working directory is an absolute path
	if !filepath.IsAbs(o.Onboard.DefaultWorkingDir) {
		return fmt.Errorf("default-working-dir must be an absolute path, got: %s", o.Onboard.DefaultWorkingDir)
	}
	// Validate directory exists and is writable
	if !fileExists(o.Onboard.DefaultWorkingDir) {
		return fmt.Errorf("invalid default working directory: %s", o.Onboard.DefaultWorkingDir)
	}
	// Test writability using CreateTemp
	tempFile, err := os.CreateTemp(o.Onboard.DefaultWorkingDir, ".fdo.test_*")
	if err != nil {
		return fmt.Errorf("default working directory is not writable: %w", err)
	}
	tempFile.Close()
	os.Remove(tempFile.Name())

	if o.Key == "" {
		return fmt.Errorf("--key is required (via CLI flag or config file)")
	}
	if err := validateKey(o.Key); err != nil {
		return err
	}

	if o.Onboard.Kex == "" {
		return fmt.Errorf("--kex is required (via CLI flag or config file)")
	}

	if !slices.Contains(validCipherSuites, o.Onboard.Cipher) {
		return fmt.Errorf("invalid cipher suite: %s", o.Onboard.Cipher)
	}

	if o.Onboard.Download != "" && (!isValidPath(o.Onboard.Download) || !fileExists(o.Onboard.Download)) {
		return fmt.Errorf("invalid download directory: %s", o.Onboard.Download)
	}

	if !slices.Contains(validKexSuites, o.Onboard.Kex) {
		return fmt.Errorf("invalid key exchange suite: '%s', options [%s]",
			o.Onboard.Kex, strings.Join(validKexSuites, ", "))
	}

	if o.Onboard.MaxServiceInfoSize < 0 || o.Onboard.MaxServiceInfoSize > math.MaxUint16 {
		return fmt.Errorf("max-serviceinfo-size must be between 0 and %d", math.MaxUint16)
	}

	if o.Onboard.WgetDir != "" && (!isValidPath(o.Onboard.WgetDir) || !fileExists(o.Onboard.WgetDir)) {
		return fmt.Errorf("invalid wget directory: %s", o.Onboard.WgetDir)
	}

	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
}
