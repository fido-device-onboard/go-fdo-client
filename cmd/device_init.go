// SPDX-FileCopyrightText: (C) 2025 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"slices"
	"strings"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo-client/internal/tls"
	"github.com/fido-device-onboard/go-fdo-client/internal/tpm_utils"
	"github.com/fido-device-onboard/go-fdo/blob"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/custom"
	"github.com/fido-device-onboard/go-fdo/protocol"
	"github.com/fido-device-onboard/go-fdo/tpm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var diConf DeviceInitClientConfig

var validDiKeyEncs = []string{"x509", "x5chain", "cose"}

var deviceInitCmd = &cobra.Command{
	Use:   "device-init [server-url]",
	Short: "Run device initialization (DI)",
	Long: `
Run device initialization (DI) to register the device with a manufacturer server.
The server URL can be provided as a positional argument, flag or via config file.
At least one of --blob or --tpm is required to store device credentials.`,
	Example: `
  # Using CLI arguments:
  go-fdo-client device-init http://127.0.0.1:8038 --key ec256 --blob cred.bin

  # Using config file:
  go-fdo-client device-init --config config.yaml

  # Mix CLI and config (CLI takes precedence):
  go-fdo-client device-init http://127.0.0.1:8038 --config config.yaml --key ec384`,
	Args: cobra.MaximumNArgs(1),
	PreRunE: func(cmd *cobra.Command, args []string) error {
		err := bindFlags(cmd, "device-init")
		if err != nil {
			return err
		}

		if len(args) > 0 {
			viper.Set("device-init.server-url", args[0])
		}

		if err := viper.Unmarshal(&diConf); err != nil {
			return fmt.Errorf("failed to unmarshal device-init config: %w", err)
		}

		return diConf.validate()
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

		if deviceStatus == FDO_STATE_PRE_DI {
			return doDI()
		} else if deviceStatus == FDO_STATE_PRE_TO1 {
			fmt.Println("Device already initialized, ready to onboard")
		} else if deviceStatus == FDO_STATE_IDLE {
			return fmt.Errorf("Device has already completed onboarding")
		} else {
			return fmt.Errorf("Device state is invalid: %v", deviceStatus)
		}

		return nil
	},
}

func deviceInitCmdInit() {
	rootCmd.AddCommand(deviceInitCmd)
	deviceInitCmd.Flags().String("key-enc", "x509", "Public key encoding to use for manufacturer key [x509,x5chain,cose]")
	deviceInitCmd.Flags().String("device-info", "", "Device information for device credentials, if not specified, it'll be gathered from the system")
	deviceInitCmd.Flags().String("device-info-mac", "", "Mac-address's iface e.g. eth0 for device credentials")
	deviceInitCmd.Flags().Bool("insecure-tls", false, "Skip TLS certificate verification")
	deviceInitCmd.Flags().String("serial-number", "", "Serial number for device credentials, if not specified, it'll be gathered from the system")
}

func init() {
	deviceInitCmdInit()
}

func doDI() (err error) { //nolint:gocyclo
	// Generate new key and secret
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("error generating device secret: %w", err)
	}
	hmacSha256, hmacSha384 := hmac.New(sha256.New, secret), hmac.New(sha512.New384, secret)

	var sigAlg x509.SignatureAlgorithm
	var keyType protocol.KeyType
	var key crypto.Signer
	switch rootConfig.Key {
	case "ec256":
		keyType = protocol.Secp256r1KeyType
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "ec384":
		keyType = protocol.Secp384r1KeyType
		key, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case "rsa2048":
		keyType = protocol.Rsa2048RestrKeyType
		key, err = rsa.GenerateKey(rand.Reader, 2048)
	case "rsa3072":
		sigAlg = x509.SHA384WithRSA
		keyType = protocol.RsaPkcsKeyType
		key, err = rsa.GenerateKey(rand.Reader, 3072)
	default:
		return fmt.Errorf("unsupported key type: %s", rootConfig.Key)
	}
	if err != nil {
		return fmt.Errorf("error generating device key: %w", err)
	}

	// If using a TPM, swap key/hmac for that
	if rootConfig.TPM != "" {
		var cleanup func() error
		hmacSha256, hmacSha384, key, cleanup, err = tpmCred()
		if err != nil {
			return err
		}
		defer func() { _ = cleanup() }()
	}

	// Generate Java implementation-compatible mfg string
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject:            pkix.Name{CommonName: "device.go-fdo"},
		SignatureAlgorithm: sigAlg,
	}, key)
	if err != nil {
		return fmt.Errorf("error creating CSR for device certificate chain: %w", err)
	}
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		return fmt.Errorf("error parsing CSR for device certificate chain: %w", err)
	}

	// If serial # is not provided, it will be gathered from system
	var serialNumber string
	if diConf.DeviceInit.SerialNumber == "" {
		serialNumber, err = getSerial()
		if err != nil {
			slog.Warn("error getting device serial number", "error", err)
		}
		diConf.DeviceInit.SerialNumber = serialNumber
	}

	var keyEncoding protocol.KeyEncoding
	switch {
	case strings.EqualFold(diConf.DeviceInit.KeyEnc, "x509"):
		keyEncoding = protocol.X509KeyEnc
	case strings.EqualFold(diConf.DeviceInit.KeyEnc, "x5chain"):
		keyEncoding = protocol.X5ChainKeyEnc
	case strings.EqualFold(diConf.DeviceInit.KeyEnc, "cose"):
		keyEncoding = protocol.CoseKeyEnc
	default:
		return fmt.Errorf("unsupported key encoding: %s", diConf.DeviceInit.KeyEnc)
	}

	var deviceInfo string
	switch {
	case diConf.DeviceInit.DeviceInfo != "":
		deviceInfo = diConf.DeviceInit.DeviceInfo
	case diConf.DeviceInit.DeviceInfoMac != "":
		deviceInfo, err = getMac(diConf.DeviceInit.DeviceInfoMac)
		if err != nil {
			return fmt.Errorf("error getting device information from iface %s: %w", diConf.DeviceInit.DeviceInfoMac, err)
		}
	default:
		deviceInfo = diConf.DeviceInit.SerialNumber
		if deviceInfo == "" {
			return fmt.Errorf("device info cannot be determined automatically. " +
				"Please specify either:\n" +
				"  --serial-number <value>  (to set device serial number)\n" +
				"  --device-info <value>    (to set device info directly)\n" +
				"  or both flags")
		}
	}
	slog.Debug("Starting Device Initialization", "Serial Number", diConf.DeviceInit.SerialNumber, "Device Info", deviceInfo)

	cred, err := fdo.DI(context.TODO(), tls.TlsTransport(diConf.DeviceInit.ServerURL, nil, diConf.DeviceInit.InsecureTLS), custom.DeviceMfgInfo{
		KeyType:      keyType,
		KeyEncoding:  keyEncoding,
		SerialNumber: diConf.DeviceInit.SerialNumber,
		DeviceInfo:   deviceInfo,
		CertInfo:     cbor.X509CertificateRequest(*csr),
	}, fdo.DIConfig{
		HmacSha256: hmacSha256,
		HmacSha384: hmacSha384,
		Key:        key,
	})
	if err != nil {
		return err
	}

	if rootConfig.TPM != "" {
		return saveTpmCred(fdoTpmDeviceCredential{
			tpm.DeviceCredential{
				DeviceCredential: *cred,
				DeviceKey:        tpm.FdoDeviceKey,
			},
			FDO_STATE_PRE_TO1,
		})
	}
	err = saveCred(fdoDeviceCredential{
		blob.DeviceCredential{
			Active:           true,
			DeviceCredential: *cred,
			HmacSecret:       secret,
			PrivateKey:       blob.Pkcs8Key{Signer: key},
		},
		FDO_STATE_PRE_TO1,
	})

	// Securely erase the secret and HMAC objects from memory
	for i := range secret {
		secret[i] = 0
	}
	hmacSha256.Reset()
	hmacSha384.Reset()

	return err
}

func (d *DeviceInitClientConfig) validate() error {
	if d.DeviceInit.ServerURL == "" {
		return fmt.Errorf("server-url is required (via positional argument, or config file)")
	}

	if d.Key == "" {
		return fmt.Errorf("--key is required (via CLI flag or config file)")
	}
	if err := validateKey(d.Key); err != nil {
		return err
	}

	if d.DeviceInit.DeviceInfo != "" && d.DeviceInit.DeviceInfoMac != "" {
		return fmt.Errorf("can't specify both --device-info and --device-info-mac")
	}

	parsedURL, err := url.ParseRequestURI(d.DeviceInit.ServerURL)
	if err != nil {
		return fmt.Errorf("invalid DI URL: %s", d.DeviceInit.ServerURL)
	}
	host, port, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		return fmt.Errorf("invalid DI URL: %s", d.DeviceInit.ServerURL)
	}
	if net.ParseIP(host) == nil && !isValidHostname(host) {
		return fmt.Errorf("invalid hostname: %s", host)
	}
	if port != "" && !isValidPort(port) {
		return fmt.Errorf("invalid port: %s", port)
	}

	if !slices.Contains(validDiKeyEncs, d.DeviceInit.KeyEnc) {
		return fmt.Errorf("invalid DI key encoding: %s", d.DeviceInit.KeyEnc)
	}

	return nil
}

func isValidHostname(hostname string) bool {
	if len(hostname) > 255 {
		return false
	}
	for _, part := range strings.Split(hostname, ".") {
		if len(part) == 0 || len(part) > 63 {
			return false
		}
		for _, char := range part {
			if !((char >= 'a' && char <= 'z') ||
				(char >= 'A' && char <= 'Z') ||
				(char >= '0' && char <= '9') ||
				char == '-') {
				return false
			}
		}
		if strings.HasPrefix(part, "-") || strings.HasSuffix(part, "-") {
			return false
		}
	}
	return true
}

func isValidPort(port string) bool {
	for _, char := range port {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
