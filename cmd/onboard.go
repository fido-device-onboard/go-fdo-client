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
)

type fsVar map[string]string

type slogErrorWriter struct{}

func (e slogErrorWriter) Write(p []byte) (int, error) {
	w := bytes.TrimSpace(p)
	slog.Error(string(w))
	return len(w), nil
}

var (
	allowCredentialReuse bool
	cipherSuite          string
	dlDir                string
	enableInteropTest    bool
	kexSuite             string
	maxServiceInfoSize   int
	resale               bool
	to2RetryDelay        time.Duration
	uploads              = make(fsVar)
	wgetDir              string
)
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
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateOnboardFlags(); err != nil {
			return fmt.Errorf("validation error: %v", err)
		}
		if debug {
			level.Set(slog.LevelDebug)
		}

		if tpmPath != "" {
			var err error
			tpmc, err = tpm_utils.TpmOpen(tpmPath)
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

		if deviceStatus == FDO_STATE_PRE_TO1 || (deviceStatus == FDO_STATE_IDLE && resale) {
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

func init() {
	rootCmd.AddCommand(onboardCmd)
	onboardCmd.Flags().BoolVar(&allowCredentialReuse, "allow-credential-reuse", false, "Allow credential reuse protocol during onboarding")
	onboardCmd.Flags().StringVar(&cipherSuite, "cipher", "A128GCM", "Name of cipher suite to use for encryption (see usage)")
	onboardCmd.Flags().StringVar(&dlDir, "download", "", "fdo.download: override destination directory set by Owner server")
	onboardCmd.Flags().StringVar(&diKey, "key", "", "Key type for device credential [options: ec256, ec384, rsa2048, rsa3072]")
	onboardCmd.Flags().BoolVar(&enableInteropTest, "enable-interop-test", false, "Enable FIDO Alliance interop test module (fsim.Interop)")
	onboardCmd.Flags().StringVar(&kexSuite, "kex", "", "Name of cipher suite to use for key exchange (see usage)")
	onboardCmd.Flags().BoolVar(&insecureTLS, "insecure-tls", false, "Skip TLS certificate verification")
	onboardCmd.Flags().IntVar(&maxServiceInfoSize, "max-serviceinfo-size", serviceinfo.DefaultMTU, "Maximum service info size to receive")
	onboardCmd.Flags().BoolVar(&resale, "resale", false, "Perform resale")
	onboardCmd.Flags().DurationVar(&to2RetryDelay, "to2-retry-delay", 0, "Delay between failed TO2 attempts when trying multiple Owner URLs from same RV directive (0=disabled)")
	onboardCmd.Flags().Var(&uploads, "upload", "fdo.upload: restrict Owner server upload access to specific dirs and files, comma-separated and/or flag provided multiple times")
	onboardCmd.Flags().StringVar(&wgetDir, "wget-dir", "", "fdo.wget: override destination directory set by Owner server")

	onboardCmd.MarkFlagRequired("key")
	onboardCmd.MarkFlagRequired("kex")
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
	kexCipherSuiteID, ok := kex.CipherSuiteByName(cipherSuite)
	if !ok {
		return fmt.Errorf("invalid key exchange cipher suite: %s", cipherSuite)
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
		KeyExchange:               kex.Suite(kexSuite),
		CipherSuite:               kexCipherSuiteID,
		AllowCredentialReuse:      allowCredentialReuse,
		MaxServiceInfoSizeReceive: uint16(maxServiceInfoSize),
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
		to1d, err = fdo.TO1(ctx, tls.TlsTransport(url.String(), nil, insecureTLS), conf.Cred, conf.Key, nil)
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
				newDC, err := transferOwnership2(ctx, tls.TlsTransport(baseURL, nil, insecureTLS), to1d, conf)
				if newDC != nil {
					slog.Info("TO2 succeeded", "base URL", baseURL)
					return newDC, nil
				}
				slog.Error("TO2 failed", "base URL", baseURL, "error", err)

				// Apply configurable delay between Owner URLs within a directive
				// (not spec-compliant, but prevents hammering the same server via different URLs)
				if !isLastURL && to2RetryDelay > 0 {
					slog.Info("Applying TO2 retry delay", "delay", to2RetryDelay)
					if err := applyDelay(ctx, to2RetryDelay); err != nil {
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

func transferOwnership2(ctx context.Context, transport fdo.Transport, to1d *cose.Sign1[protocol.To1d, []byte], conf fdo.TO2Config) (*fdo.DeviceCredential, error) {
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

	// fdo.upload: by default allow owner access to any file it requests (assume read
	// access permissions are correct).  Use --upload to restrict which files/directories the
	// owner may access
	if len(uploads) == 0 {
		uploads["/"] = "" // allow filesystem access, see fsVar.Open
	}
	fsims["fdo.upload"] = &fsim.Upload{
		FS: uploads,
	}

	// fdo.wget: use --wget-dir to force downloaded files into a specific local directory,
	// otherwise allow the owner server to control where the file is stored on the device
	wgetFSIM := &fsim.Wget{
		// bug: https://github.com/fido-device-onboard/go-fdo/issues/205
		Timeout: time.Hour,
	}
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
	conf.DeviceModules = fsims

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

func (files fsVar) String() string {
	if len(files) == 0 {
		return "[]"
	}
	paths := "["
	for path := range files {
		paths += path + ","
	}
	return paths[:len(paths)-1] + "]"
}

func (files fsVar) Set(paths string) error {
	for _, path := range strings.Split(paths, ",") {
		abs, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("[%q]: %w", path, err)
		}
		files[pathToName(path, abs)] = abs
	}
	return nil
}

func (files fsVar) Type() string {
	return "fsVar"
}

// Open implements fs.FS
func (files fsVar) Open(path string) (fs.File, error) {
	if !fs.ValidPath(path) {
		return nil, &fs.PathError{
			Op:   "open",
			Path: path,
			Err:  fs.ErrInvalid,
		}
	}

	// TODO: Enforce chroot-like security
	if _, rootAccess := files["/"]; rootAccess {
		return os.Open(filepath.Clean(path))
	}

	name := pathToName(path, "")
	if abs, ok := files[name]; ok {
		return os.Open(filepath.Clean(abs))
	}
	for dir := filepath.Dir(name); dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		if abs, ok := files[dir]; ok {
			return os.Open(filepath.Clean(abs))
		}
	}
	return nil, &fs.PathError{
		Op:   "open",
		Path: path,
		Err:  fs.ErrNotExist,
	}
}

// The name of the directory or file is its cleaned path, if absolute. If the
// path given is relative, then remove all ".." and "." at the start. If the
// path given is only 1 or more ".." or ".", then use the name of the absolute
// path.
func pathToName(path, abs string) string {
	cleaned := filepath.Clean(path)
	if rooted := path[:1] == "/"; rooted {
		return cleaned
	}
	pathparts := strings.Split(cleaned, string(filepath.Separator))
	for len(pathparts) > 0 && (pathparts[0] == ".." || pathparts[0] == ".") {
		pathparts = pathparts[1:]
	}
	if len(pathparts) == 0 && abs != "" {
		pathparts = []string{filepath.Base(abs)}
	}
	return filepath.Join(pathparts...)
}

func validateOnboardFlags() error {
	if !slices.Contains(validCipherSuites, cipherSuite) {
		return fmt.Errorf("invalid cipher suite: %s", cipherSuite)
	}

	if dlDir != "" && (!isValidPath(dlDir) || !fileExists(dlDir)) {
		return fmt.Errorf("invalid download directory: %s", dlDir)
	}

	if err := validateDiKey(); err != nil {
		return err
	}

	if !slices.Contains(validKexSuites, kexSuite) {
		return fmt.Errorf("invalid key exchange suite: '%s', options [%s]",
			kexSuite, strings.Join(validKexSuites, ", "))
	}

	if maxServiceInfoSize < 0 || maxServiceInfoSize > math.MaxUint16 {
		return fmt.Errorf("max-serviceinfo-size must be between 0 and %d", math.MaxUint16)
	}

	for path := range uploads {
		if !isValidPath(path) {
			return fmt.Errorf("invalid upload path: %s", path)
		}

		if !fileExists(path) {
			return fmt.Errorf("file doesn't exist: %s", path)
		}
	}

	if wgetDir != "" && (!isValidPath(wgetDir) || !fileExists(wgetDir)) {
		return fmt.Errorf("invalid wget directory: %s", wgetDir)
	}

	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
}
