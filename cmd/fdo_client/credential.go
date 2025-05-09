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
	"fmt"
	"hash"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/pflag"

	"github.com/fido-device-onboard/go-fdo"
	tpmnv "github.com/fido-device-onboard/go-fdo-client/internal/tpm_utils"
	"github.com/fido-device-onboard/go-fdo/blob"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/tpm"
	"github.com/google/go-tpm/tpm2"
)

const FDO_CRED_NV_IDX = 0x01D10001

// FDO Device State
type FdoDeviceState int

const (
	FDO_STATE_PC FdoDeviceState = iota
	FDO_STATE_PRE_DI
	FDO_STATE_PRE_TO1
	FDO_STATE_IDLE
	FDO_STATE_RESALE
	FDO_STATE_ERROR
)

type fdoDeviceCredential struct {
	DC    blob.DeviceCredential
	State FdoDeviceState
}

type fdoTpmDeviceCredential struct {
	DC    tpm.DeviceCredential
	State FdoDeviceState
}

func tpmCred() (hash.Hash, hash.Hash, crypto.Signer, func() error, error) {
	var diKeyFlagSet bool
	clientFlags.Flags().Visit(func(flag *pflag.Flag) {
		if flag == nil {
			slog.Error("Unexpected nil flag encountered")
			return
		}
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
		key, err = tpm.GenerateRSAKey(tpmc, 3072)
	default:
		err = fmt.Errorf("unsupported key type")
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
		var dc fdoTpmDeviceCredential
		if err := readTpmCred(&dc); err != nil {
			return nil, nil, nil, nil, nil, err
		}

		hmacSha256, hmacSha384, key, cleanup, err := tpmCred()
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		return &dc.DC.DeviceCredential, hmacSha256, hmacSha384, key, cleanup, nil
	}

	var dc fdoDeviceCredential
	if err := readCredFile(&dc); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	return &dc.DC.DeviceCredential,
		hmac.New(sha256.New, dc.DC.HmacSecret),
		hmac.New(sha512.New384, dc.DC.HmacSecret),
		&dc.DC.PrivateKey,
		nil,
		nil
}
func loadDeviceStatus(state *FdoDeviceState) bool {
	if state == nil {
		return false
	}
	var dataSize int
	if tpmPath != "" {
		nv := tpm2.TPMHandle(FDO_CRED_NV_IDX)
		dataSize = (int)(tpmnv.TpmNVGetSize(tpmc, nv))
	} else {
		blobData, err := os.ReadFile(filepath.Clean(blobPath))
		if err != nil {
			if os.IsNotExist(err) {
				slog.Debug("DeviceCredential file does not exist. Set state to run DI")
				*state = FDO_STATE_PRE_DI
				return true // Return true if the file does not exist
			}
			slog.Error("error reading blob credential %q: %v", blobPath, err)
			return false
		}
		dataSize = len(blobData)
	}

	if dataSize == 0 {
		slog.Debug("DeviceCredential is empty. Set state to run DI")
		*state = FDO_STATE_PRE_DI
	} else if resale {
		*state = FDO_STATE_RESALE
		slog.Debug("DeviceCredential is non-empty. Set state to resale")
	} else {
		slog.Debug("DeviceCredential is non-empty. Set state to run TO1/TO2")
		// No Device state is being set currently
	}
	return true
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

func updateCred(newDC fdo.DeviceCredential, state FdoDeviceState) error {
	if tpmPath != "" {
		var dc fdoTpmDeviceCredential
		if err := readTpmCred(&dc); err != nil {
			return err
		}
		dc.DC.DeviceCredential = newDC
		dc.State = state
		return saveTpmCred(dc)
	}

	var dc fdoDeviceCredential
	if err := readCredFile(&dc); err != nil {
		return err
	}
	dc.DC.DeviceCredential = newDC
	dc.State = state
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

	// Ensure the temp file is closed before renaming
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("error closing temp file: %w", err)
	}

	// Rename temp file to given blob path
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
