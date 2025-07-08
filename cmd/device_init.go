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
	"math"
	"math/big"
	"net"
	"net/url"
	"strconv"
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
)

var (
	diURL           string
	diDeviceInfo    string
	diDeviceInfoMac string
	diKey           string
	diKeyEnc        string
	insecureTLS     bool
)

var validDiKeys = []string{"ec256", "ec384", "rsa2048", "rsa3072"}
var validDiKeyEncs = []string{"x509", "x5chain", "cose"}

var deviceInitCmd = &cobra.Command{
	Use:   "device-init <server-url>",
	Short: "Run device initialization (DI)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Set the server URL from the positional argument
		diURL = args[0]

		if err := validateDIFlags(); err != nil {
			return fmt.Errorf("Validation error: %v", err)
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

func init() {
	rootCmd.AddCommand(deviceInitCmd)
	deviceInitCmd.Flags().StringVar(&diKey, "key", "", "Key type for device credential [options: ec256, ec384, rsa2048, rsa3072]")
	deviceInitCmd.Flags().StringVar(&diKeyEnc, "key-enc", "x509", "Public key encoding to use for manufacturer key [x509,x5chain,cose]")
	deviceInitCmd.Flags().StringVar(&diDeviceInfo, "device-info", "", "Device information for device credentials, if not specified, it'll be gathered from the system")
	deviceInitCmd.Flags().StringVar(&diDeviceInfoMac, "device-info-mac", "", "Mac-address's iface e.g. eth0 for device credentials")
	deviceInitCmd.Flags().BoolVar(&insecureTLS, "insecure-tls", false, "Skip TLS certificate verification")

	// User must explicitly select the key type for the device credentials since the TPM resources are limited
	deviceInitCmd.MarkFlagRequired("key")
	deviceInitCmd.MarkFlagsMutuallyExclusive("device-info", "device-info-mac")
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
	switch diKey {
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
		return fmt.Errorf("unsupported key type: %s", diKey)
	}
	if err != nil {
		return fmt.Errorf("error generating device key: %w", err)
	}

	// If using a TPM, swap key/hmac for that
	if tpmPath != "" {
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

	// Call the DI server
	sn, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return fmt.Errorf("error generating random serial number: %w", err)
	}
	var keyEncoding protocol.KeyEncoding
	switch {
	case strings.EqualFold(diKeyEnc, "x509"):
		keyEncoding = protocol.X509KeyEnc
	case strings.EqualFold(diKeyEnc, "x5chain"):
		keyEncoding = protocol.X5ChainKeyEnc
	case strings.EqualFold(diKeyEnc, "cose"):
		keyEncoding = protocol.CoseKeyEnc
	default:
		return fmt.Errorf("unsupported key encoding: %s", diKeyEnc)
	}

	var deviceInfo string
	switch {
	case diDeviceInfo != "" && diDeviceInfoMac != "":
		return fmt.Errorf("can't specify both --device-info and --device-info-mac")
	case diDeviceInfo != "":
		deviceInfo = diDeviceInfo
	case diDeviceInfoMac != "":
		deviceInfo, err = getMac(diDeviceInfoMac)
		if err != nil {
			return fmt.Errorf("error getting device information from iface %s: %w", diDeviceInfoMac, err)
		}
	default:
		deviceInfo, err = getSerial()
		if err != nil {
			return fmt.Errorf("error getting device information from the system: %w", err)
		}
	}

	cred, err := fdo.DI(context.TODO(), tls.TlsTransport(diURL, nil, insecureTLS), custom.DeviceMfgInfo{
		KeyType:      keyType,
		KeyEncoding:  keyEncoding,
		SerialNumber: strconv.FormatInt(sn.Int64(), 10),
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

	if tpmPath != "" {
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

func validateDiKey() error {
	if !contains(validDiKeys, diKey) {
		return fmt.Errorf("invalid --key type: '%s' [options: %s]", diKey, strings.Join(validDiKeys, ", "))
	}
	return nil
}

func validateDIFlags() error {
	// idURL
	parsedURL, err := url.ParseRequestURI(diURL)
	if err != nil {
		return fmt.Errorf("invalid DI URL: %s", diURL)
	}
	host, port, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		return fmt.Errorf("invalid DI URL: %s", diURL)
	}
	if net.ParseIP(host) == nil && !isValidHostname(host) {
		return fmt.Errorf("invalid hostname: %s", host)
	}
	if port != "" && !isValidPort(port) {
		return fmt.Errorf("invalid port: %s", port)
	}

	if err = validateDiKey(); err != nil {
		return err
	}

	if !contains(validDiKeyEncs, diKeyEnc) {
		return fmt.Errorf("invalid DI key encoding: %s", diKeyEnc)
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
