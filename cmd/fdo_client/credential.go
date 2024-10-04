// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package main

import (
	"bytes"
	"crypto"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"flag"
	"fmt"
	"hash"
	"os"
	"path/filepath"

	"github.com/fido-device-onboard/go-fdo"
	tpmnv "github.com/fido-device-onboard/go-fdo-client/internal/tpm_utils"
	"github.com/fido-device-onboard/go-fdo/blob"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/tpm"
	"github.com/google/go-tpm/tpm2"
)

const FDO_CRED_NV_IDX = 0x01D10001

func tpmCred() (hash.Hash, hash.Hash, crypto.Signer, func() error, error) {
	var diKeyFlagSet bool
	clientFlags.Visit(func(flag *flag.Flag) {
		diKeyFlagSet = diKeyFlagSet || flag.Name == "di-key"
	})
	if !diKeyFlagSet {
		return nil, nil, nil, nil, fmt.Errorf("-di-key must be set explicitly when using a TPM")
	}

	// Use TPM keys for HMAC and Device Key
	h256, err := tpm.NewHmac(tpmc, crypto.SHA256)
	if err != nil {
		_ = tpmc.Close()
		return nil, nil, nil, nil, err
	}
	h384, err := tpm.NewHmac(tpmc, crypto.SHA384)
	if err != nil {
		_ = tpmc.Close()
		return nil, nil, nil, nil, err
	}
	var key tpm.Key
	switch diKey {
	case "ec256":
		key, err = tpm.GenerateECKey(tpmc, elliptic.P256())
	case "ec384":
		key, err = tpm.GenerateECKey(tpmc, elliptic.P384())
	case "rsa2048":
		key, err = tpm.GenerateRSAKey(tpmc, 2048)
	case "rsa3072":
		if tpmPath == "simulator" {
			err = fmt.Errorf("TPM simulator does not support RSA3072")
		} else {
			key, err = tpm.GenerateRSAKey(tpmc, 3072)
		}
	default:
		err = fmt.Errorf("unsupported key type: %s", diKey)
	}
	if err != nil {
		_ = tpmc.Close()
		return nil, nil, nil, nil, err
	}

	return h256, h384, key, func() error {
		_ = h256.Close()
		_ = h384.Close()
		_ = key.Close()
		return nil
	}, nil
}

func readCred() (_ *fdo.DeviceCredential, hmacSha256, hmacSha384 hash.Hash, key crypto.Signer, cleanup func() error, _ error) {
	if tpmPath != "" {
		// DeviceCredential requires integrity, so it is stored as a file and
		// expected to be protected. In the future, it should be stored in the
		// TPM and access-protected with a policy.
		var dc tpm.DeviceCredential
		if err := readTpmCred(&dc); err != nil {
			return nil, nil, nil, nil, nil, err
		}

		hmacSha256, hmacSha384, key, cleanup, err := tpmCred()
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		return &dc.DeviceCredential, hmacSha256, hmacSha384, key, cleanup, nil
	}

	var dc blob.DeviceCredential
	if err := readCredFile(&dc); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	return &dc.DeviceCredential,
		hmac.New(sha256.New, dc.HmacSecret),
		hmac.New(sha512.New384, dc.HmacSecret),
		dc.PrivateKey,
		nil,
		nil
}

func readCredFile(v any) error {
	blobData, err := os.ReadFile(filepath.Clean(blobPath))
	if err != nil {
		return fmt.Errorf("error reading blob credential %q: %w", blobPath, err)
	}
	if err := cbor.Unmarshal(blobData, v); err != nil {
		return fmt.Errorf("error parsing blob credential %q: %w", blobPath, err)
	}
	if printDevice {
		fmt.Printf("%+v\n", v)
	}
	return nil
}

func updateCred(newDC fdo.DeviceCredential) error {
	if tpmPath != "" {
		var dc tpm.DeviceCredential
		if err := readTpmCred(&dc); err != nil {
			return err
		}
		dc.DeviceCredential = newDC
		return saveTpmCred(dc)
	}

	var dc blob.DeviceCredential
	if err := readCredFile(&dc); err != nil {
		return err
	}
	dc.DeviceCredential = newDC
	return saveCred(dc)
}

func saveCred(dc any) error {
	// Encode device credential to temp file
	tmp, err := os.CreateTemp(".", "fdo_cred_*")
	if err != nil {
		return fmt.Errorf("error creating temp file for device credential: %w", err)
	}
	defer func() { _ = tmp.Close() }()

	if err := cbor.NewEncoder(tmp).Encode(dc); err != nil {
		return err
	}

	// Rename temp file to given blob path
	_ = tmp.Close()
	if err := os.Rename(tmp.Name(), blobPath); err != nil {
		return fmt.Errorf("error renaming temp blob credential to %q: %w", blobPath, err)
	}

	return nil
}

// readTpmCred reads the stored credential from TPM NV memory.
func readTpmCred(v any) error {
	nv := tpm2.TPMHandle(FDO_CRED_NV_IDX)
	// Read data from NV
	data, err := tpmnv.TpmNVRead(tpmc, nv)
	if err != nil {
		return fmt.Errorf("failed to read from NV: %w", err)
	}

	// Decode CBOR data
	if err := cbor.Unmarshal(data, v); err != nil {
		return fmt.Errorf("error parsing credential: %w", err)
	}

	if printDevice {
		fmt.Printf("%+v\n", v)
	}
	return nil
}

// saveTpmCred encodes the device credential to CBOR and writes it to TPM NV memory.
func saveTpmCred(dc any) error {
	nv := tpm2.TPMHandle(FDO_CRED_NV_IDX)
	// Encode device credential to CBOR
	var buf bytes.Buffer
	if err := cbor.NewEncoder(&buf).Encode(dc); err != nil {
		return fmt.Errorf("error encoding device credential to CBOR: %w", err)
	}
	data := buf.Bytes()

	tpmHashAlg, err := getTPMAlgorithm(diKey)
	if err != nil {
		return err
	}

	// Write CBOR-encoded data to NV
	if err := tpmnv.TpmNVWrite(tpmc, data, nv, tpmHashAlg); err != nil {
		return fmt.Errorf("failed to write to NV: %w", err)
	}

	return nil
}

func getTPMAlgorithm(diKey string) (tpm2.TPMAlgID, error) {
	switch diKey {
	case "ec256":
		return tpm2.TPMAlgSHA256, nil
	case "ec384":
		return tpm2.TPMAlgSHA384, nil
	case "rsa2048":
		return tpm2.TPMAlgSHA256, nil
	case "rsa3072":
		return tpm2.TPMAlgSHA384, nil
	default:
		return 0, fmt.Errorf("unsupported key type: %s", diKey)
	}
}
