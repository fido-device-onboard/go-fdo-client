// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package cmd

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
	switch rootConfig.Key {
	case "ec256":
		key, err = tpm.GenerateECKey(tpmc, elliptic.P256())
	case "ec384":
		key, err = tpm.GenerateECKey(tpmc, elliptic.P384())
	case "rsa2048":
		key, err = tpm.GenerateRSAKey(tpmc, 2048)
	case "rsa3072":
		key, err = tpm.GenerateRSAKey(tpmc, 3072)
	default:
		err = fmt.Errorf("unsupported key type: %s", rootConfig.Key)
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
	if rootConfig.TPM != "" {
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

func loadDeviceStatus() (FdoDeviceState, error) {
	var dataSize int
	if rootConfig.TPM != "" {
		nv := tpm2.TPMHandle(FDO_CRED_NV_IDX)
		dataSize = (int)(tpmnv.TpmNVGetSize(tpmc, nv))
		if dataSize != 0 {
			var dc fdoTpmDeviceCredential
			if err := readTpmCred(&dc); err != nil {
				return FDO_STATE_PC, err
			}
			return dc.State, nil
		}
	} else {
		blobData, err := os.ReadFile(filepath.Clean(rootConfig.Blob))
		if err != nil {
			if os.IsNotExist(err) {
				slog.Debug("DeviceCredential file does not exist. Set state to run DI")
				return FDO_STATE_PRE_DI, nil
			}
			return FDO_STATE_PC, fmt.Errorf("error reading blob credential %q: %v", rootConfig.Blob, err)
		}
		if len(blobData) > 0 {
			var dc fdoDeviceCredential
			if err := readCredFile(&dc); err != nil {
				return FDO_STATE_PC, err
			}
			return dc.State, nil
		}
	}

	slog.Debug("DeviceCredential is empty. Set state to run DI")
	return FDO_STATE_PRE_DI, nil
}

func readCredFile(v any) error {
	blobData, err := os.ReadFile(filepath.Clean(rootConfig.Blob))
	if err != nil {
		return fmt.Errorf("error reading blob credential %q: %w", rootConfig.Blob, err)
	}
	if err := cbor.Unmarshal(blobData, v); err != nil {
		return fmt.Errorf("error parsing blob credential %q: %w", rootConfig.Blob, err)
	}
	return nil
}

func updateCred(newDC fdo.DeviceCredential, state FdoDeviceState) error {
	if rootConfig.TPM != "" {
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
	tmpbase := filepath.Dir(rootConfig.Blob)
	tmp, err := os.CreateTemp(tmpbase, "fdo_cred_*")
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

	// Move temp file to given blob path (supports cross-filesystem moves)
	if err := moveFile(tmp.Name(), rootConfig.Blob); err != nil {
		return fmt.Errorf("error moving temp blob credential to %q: %w", rootConfig.Blob, err)
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

	tpmHashAlg, err := getTPMAlgorithm(rootConfig.Key)
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
