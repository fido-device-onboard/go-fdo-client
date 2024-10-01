// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

//go:build !windows

// Package tpm implements device credentials using the
// [TPM Draft Spec](https://fidoalliance.org/specs/FDO/securing-fdo-in-tpm-v1.0-rd-20231010/securing-fdo-in-tpm-v1.0-rd-20231010.html).
package tpm

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/google/go-tpm/tpm2/transport/linuxtpm"
)

// TPM represents a logical connection to a TPM.
type TPM interface {
	Send(input []byte) ([]byte, error)
}

// Closer represents a logical connection to a TPM and you can close it.
type Closer interface {
	TPM
	io.Closer
}

// Open a TPM device at the given path.
//
// Clients should use /dev/tpmrm0 because using /dev/tpm0 requires more
// extensive resource management that the kernel already handles for us
// when using the kernel resource manager.
func Open(path string) (Closer, error) {
	switch path {
	case "/dev/tpmrm0":
		return linuxtpm.Open(path)
	case "/dev/tpm0":
		slog.Warn("direct use of the TPM can lead to resource exhaustion, use a TPM resource manager instead")
		return linuxtpm.Open(path)
	default:
		return nil, fmt.Errorf("unsupported TPM device path: %s", path)
	}
}
